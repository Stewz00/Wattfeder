package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/observability"
	"github.com/Stewz00/wattfeder/internal/persistence"
)

// opsConfig is what the flags decided about reporting on the run.
type opsConfig struct {
	address      string        // -ops-address
	otlpEndpoint string        // -otlp-endpoint
	interval     time.Duration // -interval, the pace readiness expects intervals to arrive at
	grace        time.Duration // -shutdown-grace
}

// opsStack is how one run reports on itself: the observers the runtime hands each interval to,
// an optional tracer provider, and an optional server for /healthz, /readyz and /metrics.
// Whatever ends the run, close shuts down what startOps opened.
type opsStack struct {
	observers      []application.Observer
	tracerProvider *sdktrace.TracerProvider
	server         *observability.Server
	serveErr       <-chan error
	grace          time.Duration
}

// startOps builds the observers and starts the ops server, if one is configured. The listener
// binds here, before the caller opens the database, so a bad -ops-address fails startup the same
// way a bad database path does, leaving nothing behind.
func startOps(ctx context.Context, cfg opsConfig, log *slog.Logger) (*opsStack, error) {
	metrics := observability.NewMetrics()
	readiness := observability.NewReadiness(cfg.interval)
	ops := &opsStack{grace: cfg.grace}

	if cfg.otlpEndpoint != "" {
		provider, err := observability.NewTracerProvider(ctx, cfg.otlpEndpoint, "wattfeder")
		if err != nil {
			return nil, fmt.Errorf("build tracer provider: %w", err)
		}
		ops.tracerProvider = provider
		// The tracer goes first so the interval span it opens is what every later observer's
		// context carries: the logger reads trace_id back out of it, and the traced repository
		// nests its commit span under it.
		ops.observers = append(ops.observers, observability.NewTracer(provider))
	}
	ops.observers = append(ops.observers, observability.NewLogger(log), metrics, readiness)

	if cfg.address != "" {
		server := observability.NewServer(cfg.address, metrics, readiness)
		listener, err := server.Listen()
		if err != nil {
			return nil, errors.Join(fmt.Errorf("listen on -ops-address: %w", err), ops.close())
		}

		serveErr := make(chan error, 1)
		go func() { serveErr <- server.Serve(listener) }()
		ops.server, ops.serveErr = server, serveErr
		log.Info("ops_server_listening", "address", listener.Addr().String())
	}

	return ops, nil
}

// observer combines every observer into the one value application.Agent holds.
func (o *opsStack) observer() application.Observer {
	return observability.NewMultiObserver(o.observers...)
}

// repository decorates repo so each commit becomes a child span of its interval span. Without
// tracing configured it returns repo unchanged.
func (o *opsStack) repository(repo persistence.Repository) persistence.Repository {
	if o.tracerProvider == nil {
		return repo
	}
	return observability.NewTracedRepository(repo, o.tracerProvider)
}

// close shuts down what startOps opened, stopping the server before flushing the spans the run
// produced.
func (o *opsStack) close() error {
	var failures []error
	if o.server != nil {
		failures = append(failures, shutdown("ops server", o.grace, o.server.Shutdown), <-o.serveErr)
	}
	if o.tracerProvider != nil {
		failures = append(failures, shutdown("tracer provider", o.grace, o.tracerProvider.Shutdown))
	}
	return errors.Join(failures...)
}

// shutdown stops one component within grace, naming it in any error. It deliberately starts from
// a fresh context: shutdown usually runs because the run's own context was cancelled, and a
// cancelled context would leave pending spans unflushed and open connections unclosed.
func shutdown(name string, grace time.Duration, stop func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()

	if err := stop(ctx); err != nil {
		return fmt.Errorf("shut down %s: %w", name, err)
	}
	return nil
}
