package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/household"
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

func run(ctx context.Context, args []string, output io.Writer) error {
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
	policy, err := household.NewPolicy(cfg.BatteryCapacityKWh, cfg.Interval)
	if err != nil {
		return fmt.Errorf("configure control policy: %w", err)
	}
	sim, err := simulator.New(cfg)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(output)
	if err := application.RunDay(ctx, sim, policy, func(record application.Record) error {
		return encoder.Encode(record)
	}); err != nil {
		return fmt.Errorf("run simulation: %w", err)
	}

	return nil
}
