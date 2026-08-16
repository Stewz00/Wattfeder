package observability

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
)

func TestHealthzAlwaysReports200(t *testing.T) {
	server := httptest.NewServer(newMux(NewMetrics(), NewReadiness(time.Hour), time.Now))
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestReadyzReportsStartingAsNotReady(t *testing.T) {
	server := httptest.NewServer(newMux(NewMetrics(), NewReadiness(time.Hour), time.Now))
	defer server.Close()

	resp, err := http.Get(server.URL + "/readyz")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["failing_check"] != "telemetry" {
		t.Errorf("failing_check = %q, want %q", body["failing_check"], "telemetry")
	}
}

func TestReadyzReportsReadyAfterAnInterval(t *testing.T) {
	readiness := NewReadiness(time.Hour)
	_, end := readiness.BeginInterval(t.Context())
	end(application.Record{}, nil)

	server := httptest.NewServer(newMux(NewMetrics(), readiness, time.Now))
	defer server.Close()

	resp, err := http.Get(server.URL + "/readyz")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestReadyzReportsTelemetryStalledLongAfterAnInterval(t *testing.T) {
	readiness := NewReadiness(time.Minute)
	_, end := readiness.BeginInterval(t.Context())
	end(application.Record{}, nil)

	future := func() time.Time { return time.Now().Add(4 * time.Minute) }
	server := httptest.NewServer(newMux(NewMetrics(), readiness, future))
	defer server.Close()

	resp, err := http.Get(server.URL + "/readyz")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["failing_check"] != "telemetry" {
		t.Errorf("failing_check = %q, want %q (a stalled loop is a telemetry failure, not storage)", body["failing_check"], "telemetry")
	}
}

func TestReadyzReportsStorageFailureAfterACommitFailure(t *testing.T) {
	readiness := NewReadiness(time.Hour)
	_, end := readiness.BeginInterval(t.Context())
	end(application.Record{}, errFake)

	server := httptest.NewServer(newMux(NewMetrics(), readiness, time.Now))
	defer server.Close()

	resp, err := http.Get(server.URL + "/readyz")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["failing_check"] != "storage" {
		t.Errorf("failing_check = %q, want %q", body["failing_check"], "storage")
	}
}

func TestMetricsEndpointServesTheWattfederSeries(t *testing.T) {
	metrics := NewMetrics()
	server := httptest.NewServer(newMux(metrics, NewReadiness(time.Hour), time.Now))
	defer server.Close()

	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestServerStartsAndStopsOnAnEphemeralPort(t *testing.T) {
	server := NewServer("127.0.0.1:0", NewMetrics(), NewReadiness(time.Hour))
	listener, err := server.Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	resp, err := http.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	resp.Body.Close()

	if err := server.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := <-serveErr; err != nil {
		t.Errorf("Serve() error = %v, want nil after a clean shutdown", err)
	}
}

var errFake = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "commit processing result: boom" }
