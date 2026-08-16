package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/household"
)

func TestRunEmitsJSONRecords(t *testing.T) {
	var output bytes.Buffer
	if err := runIsolated(t, context.Background(), []string{"-interval", "24h", "-pace", "fast", "-intervals", "1"}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	decoder := json.NewDecoder(&output)
	var record application.Record
	if err := decoder.Decode(&record); err != nil {
		t.Fatalf("decode output: %v", err)
	}

	if record.DeviceID != "home-001" {
		t.Errorf("device ID = %q, want %q", record.DeviceID, "home-001")
	}
	if record.EventID == "" {
		t.Error("event ID is empty")
	}
	if record.Decision == "" || record.Reason == "" {
		t.Errorf("decision = %q, reason = %q, want both populated", record.Decision, record.Reason)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Errorf("decode after first record error = %v, want EOF", err)
	}
}

func TestRunRejectsInvalidStart(t *testing.T) {
	err := run(context.Background(), []string{"-start", "not-a-time"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "parse -start") {
		t.Errorf("run() error = %v, want start parsing error", err)
	}
}

func TestRunMapsConfigurationFlagsToOutput(t *testing.T) {
	var output bytes.Buffer
	args := []string{
		"-interval", "24h",
		"-start", "2026-08-08T06:30:00+02:00",
		"-device-id", "home-test",
		"-starting-battery-soc-percent", "73",
		"-pace", "fast",
		"-intervals", "1",
	}
	if err := runIsolated(t, context.Background(), args, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	var record application.Record
	if err := json.NewDecoder(&output).Decode(&record); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	wantTimestamp := time.Date(2026, 8, 8, 4, 30, 0, 0, time.UTC)
	if record.Timestamp == nil || !record.Timestamp.Equal(wantTimestamp) {
		t.Errorf("timestamp = %v, want %v", record.Timestamp, wantTimestamp)
	}
	if record.DeviceID != "home-test" {
		t.Errorf("device ID = %q, want %q", record.DeviceID, "home-test")
	}
	if record.BatterySOCPercent == nil || *record.BatterySOCPercent != 73 {
		t.Errorf("battery SOC = %v, want 73", record.BatterySOCPercent)
	}
}

func TestRunIsDeterministic(t *testing.T) {
	args := []string{"-interval", "24h", "-seed", "123", "-pace", "fast", "-intervals", "1"}
	var first bytes.Buffer
	var second bytes.Buffer

	if err := runIsolated(t, context.Background(), args, &first); err != nil {
		t.Fatalf("first run() error = %v", err)
	}
	if err := runIsolated(t, context.Background(), args, &second); err != nil {
		t.Fatalf("second run() error = %v", err)
	}
	if first.String() != second.String() {
		t.Errorf("identical runs produced different output:\nfirst:  %s\nsecond: %s", first.String(), second.String())
	}
}

func TestRunDemoScenario(t *testing.T) {
	scenarioPath := filepath.Join("..", "..", "scenarios", "demo.json")
	args := []string{"-scenario", scenarioPath}
	var first bytes.Buffer
	var second bytes.Buffer

	if err := run(context.Background(), args, &first); err != nil {
		t.Fatalf("first run() error = %v", err)
	}
	if err := run(context.Background(), args, &second); err != nil {
		t.Fatalf("second run() error = %v", err)
	}
	if first.String() != second.String() {
		t.Error("identical scenario runs produced different output")
	}

	output := first.String()
	for _, want := range []string{
		`"event":"demo_scenario_loaded"`,
		`"event":"telemetry_produced"`,
		`"event":"decision_produced"`,
		`"event":"simulation_completed"`,
		`"records":4`,
		`"charge_decisions":1`,
		`"discharge_decisions":3`,
		`"expected_result":"matched"`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("run() output does not contain %q:\n%s", want, output)
		}
	}
}

func TestRunUnreliableTelemetryScenario(t *testing.T) {
	scenarioPath := filepath.Join("..", "..", "scenarios", "unreliable-telemetry-day.json")
	args := []string{"-scenario", scenarioPath}
	var first bytes.Buffer
	var second bytes.Buffer

	if err := run(context.Background(), args, &first); err != nil {
		t.Fatalf("first run() error = %v", err)
	}
	if err := run(context.Background(), args, &second); err != nil {
		t.Fatalf("second run() error = %v", err)
	}
	if first.String() != second.String() {
		t.Error("identical scenario runs produced different output")
	}

	output := first.String()
	for _, want := range []string{
		`"disposition":"accepted"`,
		`"disposition":"rejected"`,
		`"disposition":"missing"`,
		`"disposition":"unavailable"`,
		`"disposition":"duplicate"`,
		`"disposition":"history_only"`,
		`"health_status":"online"`,
		`"health_status":"invalid"`,
		`"health_status":"offline"`,
		`"health_status":"stale"`,
		`"records":12`,
		`"expected_result":"matched"`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("run() output does not contain %q:\n%s", want, output)
		}
	}
}

func TestRunRejectsScenarioWithConfigurationFlags(t *testing.T) {
	err := run(context.Background(), []string{"-scenario", "demo.json", "-seed", "1"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("run() error = %v, want conflicting-flags error", err)
	}
}

func TestRunRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "unknown flag", args: []string{"-unknown"}, wantErr: "flag provided but not defined"},
		{name: "missing flag value", args: []string{"-interval"}, wantErr: "flag needs an argument"},
		{name: "invalid duration", args: []string{"-interval", "tomorrow"}, wantErr: "invalid value"},
		{name: "partial final interval", args: []string{"-interval", "7m"}, wantErr: "must divide"},
		{name: "blank device ID", args: []string{"-device-id", "  "}, wantErr: "device ID"},
		{name: "blank agent ID", args: []string{"-agent-id", "  "}, wantErr: "agent ID"},
		{name: "zero battery capacity", args: []string{"-battery-capacity-kwh", "0"}, wantErr: "battery capacity"},
		{name: "blank database path", args: []string{"-database", " "}, wantErr: "SQLite path"},
		{name: "invalid pace", args: []string{"-pace", "sideways", "-intervals", "1"}, wantErr: "invalid -pace value"},
		{name: "invalid log level", args: []string{"-log-level", "shout", "-intervals", "1"}, wantErr: "invalid -log-level value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The default -database path is relative, so a rejected run is checked from a
			// scratch directory rather than the package's own.
			t.Chdir(t.TempDir())

			err := run(context.Background(), tt.args, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("run() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunRejectsArgumentsWithoutCreatingADatabase(t *testing.T) {
	// Every flag is settled before persistence is opened, so an argument the agent was always
	// going to reject leaves nothing behind in the directory it was started from.
	dir := t.TempDir()
	t.Chdir(dir)

	if err := run(context.Background(), []string{"-pace", "sideways"}, &bytes.Buffer{}); err == nil {
		t.Fatal("run() error = nil, want an invalid -pace error")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("a rejected run left %v behind, want an untouched directory", names)
	}
}

func TestRunReturnsOutputError(t *testing.T) {
	wantErr := errors.New("output unavailable")
	err := runIsolated(t, context.Background(), []string{"-interval", "24h", "-pace", "fast", "-intervals", "1"}, errorWriter{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Errorf("run() error = %v, want wrapped output error %v", err, wantErr)
	}
}

func TestRunPrintsHelp(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"-help"}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	help := output.String()
	if !strings.Contains(help, "Usage: wattfeder [options]") ||
		!strings.Contains(help, "-interval") ||
		!strings.Contains(help, "-database") {
		t.Errorf("run() help = %q, want usage and available flags", help)
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	err := run(context.Background(), []string{"unexpected"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Errorf("run() error = %v, want unexpected-arguments error", err)
	}
}

func TestRunTreatsCancellationAsACleanStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var output bytes.Buffer
	// Ctrl+C and SIGTERM are how the agent is meant to stop, so run reports success and main
	// exits 0. Reporting the cancellation as an error would force main to tell it apart from a
	// real failure that arrived alongside it.
	if err := runIsolated(t, ctx, []string{"-interval", "24h"}, &output); err != nil {
		t.Errorf("run() error = %v, want nil", err)
	}
	if output.Len() != 0 {
		t.Errorf("run() wrote %q after cancellation, want no output", output.String())
	}
}

func TestRunRestoresLatestBatteryState(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "wattfeder.db")
	firstArgs := []string{
		"-database", databasePath,
		"-interval", "12h",
		"-start", "2026-08-07T00:00:00Z",
		"-starting-battery-soc-percent", "73",
		"-pace", "fast",
		"-intervals", "2",
	}
	var firstOutput bytes.Buffer
	if err := run(t.Context(), firstArgs, &firstOutput); err != nil {
		t.Fatalf("first run() error = %v", err)
	}

	firstRecords := decodeRecords(t, &firstOutput)
	if len(firstRecords) != 2 {
		t.Fatalf("first run record count = %d, want 2", len(firstRecords))
	}

	secondArgs := []string{
		"-database", databasePath,
		"-interval", "24h",
		"-start", "2026-08-08T00:00:00Z",
		"-starting-battery-soc-percent", "1",
		"-pace", "fast",
		"-intervals", "1",
	}
	var secondOutput bytes.Buffer
	if err := run(t.Context(), secondArgs, &secondOutput); err != nil {
		t.Fatalf("second run() error = %v", err)
	}

	secondRecords := decodeRecords(t, &secondOutput)
	if len(secondRecords) != 1 {
		t.Fatalf("second run record count = %d, want 1", len(secondRecords))
	}
	lastFirst := firstRecords[len(firstRecords)-1]
	if lastFirst.BatterySOCPercent == nil || secondRecords[0].BatterySOCPercent == nil {
		t.Fatalf("expected battery SOC on both runs' records, got %v and %v", lastFirst.BatterySOCPercent, secondRecords[0].BatterySOCPercent)
	}
	if *secondRecords[0].BatterySOCPercent != *lastFirst.BatterySOCPercent {
		t.Errorf(
			"restored battery SOC = %v, want latest persisted value %v",
			*secondRecords[0].BatterySOCPercent,
			*lastFirst.BatterySOCPercent,
		)
	}
}

func TestRunReportsDuplicatesWithoutRedelivering(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "wattfeder.db")
	args := []string{"-database", databasePath, "-interval", "24h", "-pace", "fast", "-intervals", "1"}
	var firstOutput bytes.Buffer
	if err := run(t.Context(), args, &firstOutput); err != nil {
		t.Fatalf("first run() error = %v", err)
	}
	firstRecords := decodeRecords(t, &firstOutput)
	if len(firstRecords) == 0 {
		t.Fatal("first run produced no records")
	}

	var secondOutput bytes.Buffer
	if err := run(t.Context(), args, &secondOutput); err != nil {
		t.Fatalf("duplicate run() error = %v", err)
	}
	secondRecords := decodeRecords(t, &secondOutput)
	if len(secondRecords) != len(firstRecords) {
		t.Fatalf("duplicate run record count = %d, want %d (every interval must still be reported)", len(secondRecords), len(firstRecords))
	}
	for i, record := range secondRecords {
		if record.Disposition != household.DispositionDuplicate {
			t.Errorf("record %d disposition = %q, want %q", i, record.Disposition, household.DispositionDuplicate)
		}
		if record.StateUpdated {
			t.Errorf("record %d state_updated = true, want false (a duplicate must not redeliver)", i)
		}
		if record.Decision != "" {
			t.Errorf("record %d decision = %q, want empty (a duplicate must not produce a command)", i, record.Decision)
		}
	}
}

func TestRunReturnsPersistenceStartupErrorBeforeOutput(t *testing.T) {
	var output bytes.Buffer
	err := run(
		t.Context(),
		[]string{"-database", t.TempDir(), "-interval", "24h"},
		&output,
	)
	if err == nil || !strings.Contains(err.Error(), "open persistence") {
		t.Errorf("run() error = %v, want persistence startup error", err)
	}
	if output.Len() != 0 {
		t.Errorf("run() output = %q, want none after persistence startup failure", output.String())
	}
}

func TestRunTwoIndependentAgentsDoNotCrossContaminate(t *testing.T) {
	dir := t.TempDir()
	databaseA := filepath.Join(dir, "agent-a.db")
	databaseB := filepath.Join(dir, "agent-b.db")

	argsFor := func(agentID, deviceID, databasePath string) []string {
		return []string{
			"-agent-id", agentID,
			"-device-id", deviceID,
			"-database", databasePath,
			"-interval", "1h",
			"-pace", "fast",
			"-intervals", "3",
		}
	}

	var outputA, outputB bytes.Buffer
	var errA, errB error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		errA = run(t.Context(), argsFor("agent-a", "home-a", databaseA), &outputA)
	}()
	go func() {
		defer wg.Done()
		errB = run(t.Context(), argsFor("agent-b", "home-b", databaseB), &outputB)
	}()
	wg.Wait()

	if errA != nil {
		t.Fatalf("agent A run() error = %v", errA)
	}
	if errB != nil {
		t.Fatalf("agent B run() error = %v", errB)
	}

	recordsA := decodeRecords(t, &outputA)
	recordsB := decodeRecords(t, &outputB)
	if len(recordsA) != 3 || len(recordsB) != 3 {
		t.Fatalf("record counts = %d, %d, want 3 and 3", len(recordsA), len(recordsB))
	}
	for i, record := range recordsA {
		if record.AgentID != "agent-a" || record.DeviceID != "home-a" {
			t.Errorf("agent A record %d = (%q, %q), want (%q, %q)", i, record.AgentID, record.DeviceID, "agent-a", "home-a")
		}
	}
	for i, record := range recordsB {
		if record.AgentID != "agent-b" || record.DeviceID != "home-b" {
			t.Errorf("agent B record %d = (%q, %q), want (%q, %q)", i, record.AgentID, record.DeviceID, "agent-b", "home-b")
		}
	}
}

func runIsolated(t *testing.T, ctx context.Context, args []string, output io.Writer) error {
	t.Helper()
	databaseArgs := []string{"-database", filepath.Join(t.TempDir(), "wattfeder.db")}
	return run(ctx, append(args, databaseArgs...), output)
}

func TestRunWithOpsAddressServesHealthReadinessAndMetrics(t *testing.T) {
	databaseArgs := []string{"-database", filepath.Join(t.TempDir(), "wattfeder.db")}
	args := append([]string{
		"-interval", "1ms", "-pace", "fast", "-intervals", "2",
		"-ops-address", "127.0.0.1:0",
	}, databaseArgs...)

	addrFound := make(chan string, 1)

	var output bytes.Buffer
	errPipeR, errPipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	t.Cleanup(func() { errPipeR.Close() })

	done := make(chan error, 1)
	go func() {
		done <- runWithErrOutput(t.Context(), args, &output, errPipeW)
		errPipeW.Close()
	}()

	go func() {
		decoder := json.NewDecoder(errPipeR)
		for {
			var line map[string]any
			if err := decoder.Decode(&line); err != nil {
				return
			}
			if line["msg"] == "ops_server_listening" {
				addrFound <- line["address"].(string)
				return
			}
		}
	}()

	var addr string
	select {
	case addr = <-addrFound:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the ops server to log its address")
	}

	if resp, err := http.Get("http://" + addr + "/healthz"); err != nil {
		t.Errorf("GET /healthz error = %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("/healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	}

	if resp, err := http.Get("http://" + addr + "/metrics"); err != nil {
		t.Errorf("GET /metrics error = %v", err)
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(body), "wattfeder_") {
			t.Errorf("/metrics body does not contain wattfeder_ series:\n%s", body)
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRunWithEmptyOpsAddressStartsNoListener(t *testing.T) {
	var output bytes.Buffer
	if err := runIsolated(t, context.Background(), []string{"-interval", "24h", "-pace", "fast", "-intervals", "1"}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRunWithBadOpsAddressFailsBeforeOpeningTheDatabase(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	err := run(context.Background(), []string{"-ops-address", "not-an-address:not-a-port", "-intervals", "1"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "listen on -ops-address") {
		t.Errorf("run() error = %v, want an ops-address listen error", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("a rejected -ops-address left %v behind, want an untouched directory", entries)
	}
}

func TestRunReleasesTheOpsPortWhenALaterStartupStepFails(t *testing.T) {
	address := freeAddress(t)

	// A blank -database is rejected inside sqlite.Open, which runs after the ops listener binds.
	err := run(context.Background(), []string{"-ops-address", address, "-database", " ", "-intervals", "1"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("run() error = nil, want the blank database path rejected")
	}

	listener, listenErr := net.Listen("tcp", address)
	if listenErr != nil {
		t.Fatalf("the ops server still holds %s after run() failed with %v: %v", address, err, listenErr)
	}
	listener.Close()
}

// freeAddress returns a loopback address that is free right now, for a test that needs to know
// the address before the code under test binds it.
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}

func TestRunSeparatesLogsFromTheRecordStream(t *testing.T) {
	var output, errOutput bytes.Buffer
	databaseArgs := []string{"-database", filepath.Join(t.TempDir(), "wattfeder.db")}
	args := append([]string{"-interval", "24h", "-pace", "fast", "-intervals", "2"}, databaseArgs...)

	if err := runWithErrOutput(context.Background(), args, &output, &errOutput); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if errOutput.Len() == 0 {
		t.Fatal("errOutput is empty, want structured log lines")
	}
	decoder := json.NewDecoder(&errOutput)
	logLines := 0
	for {
		var line map[string]any
		if err := decoder.Decode(&line); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode log line: %v", err)
		}
		if line["msg"] == "interval_processed" {
			logLines++
		}
	}
	if logLines != 2 {
		t.Errorf("interval_processed log lines = %d, want 2", logLines)
	}

	decoder = json.NewDecoder(&output)
	for {
		var record application.Record
		if err := decoder.Decode(&record); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode record: %v (stdout must carry only records)", err)
		}
	}
}

func decodeRecords(t *testing.T, input io.Reader) []application.Record {
	t.Helper()
	decoder := json.NewDecoder(input)
	var records []application.Record
	for {
		var record application.Record
		if err := decoder.Decode(&record); errors.Is(err, io.EOF) {
			return records
		} else if err != nil {
			t.Fatalf("decode record: %v", err)
		}
		records = append(records, record)
	}
}

// errorWriter makes JSON encoding fail deterministically without filesystem I/O
type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
