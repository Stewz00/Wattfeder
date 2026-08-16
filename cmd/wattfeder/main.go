package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/demo"
	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/observability"
	"github.com/Stewz00/wattfeder/internal/persistence"
	"github.com/Stewz00/wattfeder/internal/persistence/sqlite"
	"github.com/Stewz00/wattfeder/internal/simulator"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()

	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses arguments and executes the agent, writing the record stream to output and
// structured logs to os.Stderr. Logs never share output's stream: make demo, make demo-faults
// and the CLI tests all parse output as newline-delimited JSON records.
func run(ctx context.Context, args []string, output io.Writer) error {
	return runWithErrOutput(ctx, args, output, os.Stderr)
}

func runWithErrOutput(ctx context.Context, args []string, output, errOutput io.Writer) error {
	flags := flag.NewFlagSet("wattfeder", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	seed := flags.Int64("seed", 42, "seed for deterministic profile variation")
	start := flags.String("start", "2026-08-07T00:00:00Z", "simulation start time in RFC 3339")
	interval := flags.Duration("interval", time.Hour, "telemetry interval")
	deviceID := flags.String("device-id", "home-001", "household device ID")
	batteryCapacity := flags.Float64("battery-capacity-kwh", 10, "battery capacity in kWh")
	startingSOC := flags.Float64("starting-battery-soc-percent", 50, "initial battery state of charge")
	pvPeakPower := flags.Float64("pv-peak-power-kw", 6, "PV peak power in kW")
	loadBasePower := flags.Float64("load-base-power-kw", 0.4, "household base power in kW")
	priceBase := flags.Float64("price-base-eur-kwh", 0.30, "base electricity price in EUR/kWh")
	databasePath := flags.String("database", "wattfeder.db", "path to the SQLite database")
	scenarioPath := flags.String("scenario", "", "path to a deterministic demo scenario in JSON format")
	agentID := flags.String("agent-id", "agent-001", "identity of this installed agent instance")
	intervals := flags.Int("intervals", 0, "number of intervals to process; 0 runs until stopped")
	pace := flags.String("pace", "real", `"real" waits one interval between observations; "fast" does not wait`)
	shutdownGrace := flags.Duration("shutdown-grace", 5*time.Second, "how long an in-flight commit may finish after cancellation")
	logLevel := flags.String("log-level", "info", `log verbosity: "debug", "info", "warn" or "error"`)
	opsAddress := flags.String("ops-address", "", "address for /healthz, /readyz and /metrics; empty serves nothing")
	otlpEndpoint := flags.String("otlp-endpoint", "", "OTLP/HTTP collector endpoint, e.g. localhost:4318; empty disables tracing")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(output, "Usage: %s [options]\n\nOptions:\n", flags.Name())
			flags.SetOutput(output)
			flags.PrintDefaults()
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(*agentID) == "" {
		return fmt.Errorf("agent ID must not be empty")
	}
	level, err := observability.ParseLevel(*logLevel)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(errOutput, &slog.HandlerOptions{Level: level}))

	if *scenarioPath != "" {
		var conflictingFlags []string
		flags.Visit(func(option *flag.Flag) {
			if option.Name != "scenario" {
				conflictingFlags = append(conflictingFlags, "-"+option.Name)
			}
		})
		if len(conflictingFlags) != 0 {
			return fmt.Errorf("-scenario cannot be combined with configuration flags: %s", strings.Join(conflictingFlags, ", "))
		}

		scenario, err := demo.LoadScenario(*scenarioPath)
		if err != nil {
			return fmt.Errorf("load demo scenario: %w", err)
		}
		if err := demo.Run(ctx, scenario, output); err != nil {
			return fmt.Errorf("run demo: %w", err)
		}
		return nil
	}

	startTime, err := time.Parse(time.RFC3339, *start)
	if err != nil {
		return fmt.Errorf("parse -start: %w", err)
	}

	cfg := simulator.Config{
		Seed:                      *seed,
		Start:                     startTime,
		Interval:                  *interval,
		DeviceID:                  *deviceID,
		BatteryCapacityKWh:        *batteryCapacity,
		StartingBatterySOCPercent: *startingSOC,
		PVPeakPowerKW:             *pvPeakPower,
		LoadBasePowerKW:           *loadBasePower,
		PriceBaseEURPerKWh:        *priceBase,
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configure simulator: %w", err)
	}
	policy, err := household.NewPolicy(cfg.BatteryCapacityKWh, cfg.Interval)
	if err != nil {
		return fmt.Errorf("configure control policy: %w", err)
	}

	// Settle every flag before opening the database, so a rejected argument never leaves a
	// database file behind for a run that was never going to start.
	clock, err := buildClock(*pace, cfg.Start)
	if err != nil {
		return err
	}

	metrics := observability.NewMetrics()
	readiness := observability.NewReadiness(cfg.Interval)

	var tracerProvider *sdktrace.TracerProvider
	var tracer *observability.Tracer
	if *otlpEndpoint != "" {
		tracerProvider, err = observability.NewTracerProvider(ctx, *otlpEndpoint, "wattfeder")
		if err != nil {
			return fmt.Errorf("build tracer provider: %w", err)
		}
		tracer = observability.NewTracer(tracerProvider)
	}

	// The ops listener binds, if configured, before the database opens: a bad -ops-address must
	// fail startup the same way a bad database path does, leaving nothing behind.
	var opsServer *observability.Server
	var opsListener net.Listener
	if *opsAddress != "" {
		opsServer = observability.NewServer(*opsAddress, metrics, readiness)
		opsListener, err = opsServer.Listen()
		if err != nil {
			return fmt.Errorf("listen on -ops-address: %w", err)
		}
	}

	opsServeErr := make(chan error, 1)
	if opsServer != nil {
		go func() { opsServeErr <- opsServer.Serve(opsListener) }()
		log.Info("ops_server_listening", "address", opsListener.Addr().String())
	}

	log.Info("agent_starting", "agent_id", *agentID, "device_id", cfg.DeviceID, "database", *databasePath)

	repository, err := sqlite.Open(*databasePath)
	if err != nil {
		return fmt.Errorf("open persistence: %w", err)
	}
	if err := repository.Migrate(ctx); err != nil {
		return closeRepository(repository, fmt.Errorf("migrate persistence: %w", err))
	}
	log.Debug("persistence_migrated", "database", *databasePath)
	snapshot, found, err := repository.Snapshot(ctx, cfg.DeviceID)
	if err != nil {
		return closeRepository(repository, fmt.Errorf("restore device snapshot: %w", err))
	}
	if found && snapshot.State.DeviceID != "" {
		cfg.StartingBatterySOCPercent = snapshot.State.BatterySOCPercent
	}

	sim, err := simulator.New(cfg)
	if err != nil {
		return closeRepository(repository, err)
	}

	var repoForRun persistence.Repository = repository
	observers := []application.Observer{}
	if tracer != nil {
		// The tracer runs first so the interval span it opens is what every other observer's
		// context carries, letting the logger read trace_id back out and the traced repository
		// nest its commit span under the interval span.
		observers = append(observers, tracer)
		repoForRun = observability.NewTracedRepository(repository, tracerProvider)
	}
	observers = append(observers, observability.NewLogger(log), metrics, readiness)

	encoder := json.NewEncoder(output)
	runErr := application.Run(ctx, application.Agent{
		Clock:         clock,
		Source:        sim,
		Sink:          sim,
		Policy:        policy,
		Repository:    repoForRun,
		Observer:      observability.NewMultiObserver(observers...),
		ID:            *agentID,
		DeviceID:      cfg.DeviceID,
		MaxIntervals:  *intervals,
		ShutdownGrace: *shutdownGrace,
		Write: func(record application.Record) error {
			return encoder.Encode(record)
		},
	})
	if runErr != nil {
		log.Error("run_failed", "error", runErr.Error())
		runErr = fmt.Errorf("run simulation: %w", runErr)
	} else {
		log.Info("run_ended")
	}

	log.Debug("agent_shutting_down")
	if tracerProvider != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), *shutdownGrace)
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("shut down tracer provider: %w", err))
		}
		cancel()
	}
	if opsServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), *shutdownGrace)
		if err := opsServer.Shutdown(shutdownCtx); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("shut down ops server: %w", err))
		}
		cancel()
		<-opsServeErr
	}

	return closeRepository(repository, runErr)
}

// buildClock returns the Clock matching pace: "real" waits one interval between observations;
// "fast" never waits and tracks a simulated schedule starting at start instead of the wall clock.
func buildClock(pace string, start time.Time) (application.Clock, error) {
	switch pace {
	case "real":
		return application.NewRealClock(), nil
	case "fast":
		return application.NewInstantClock(start.UTC()), nil
	default:
		return nil, fmt.Errorf(`invalid -pace value %q: must be "real" or "fast"`, pace)
	}
}

func closeRepository(repository *sqlite.Repository, priorErr error) error {
	if err := repository.Close(); err != nil {
		return errors.Join(priorErr, fmt.Errorf("close persistence: %w", err))
	}
	return priorErr
}
