package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func newMux(metrics *Metrics, readiness *Readiness, now func() time.Time) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		failing, ready := readiness.Check(now())
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "failing_check": failing})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})

	mux.Handle("/metrics", promhttp.HandlerFor(metrics.Registry(), promhttp.HandlerOpts{}))

	return mux
}

// Server serves /healthz, /readyz and /metrics on one address.
type Server struct {
	httpServer *http.Server
}

// NewServer builds a Server bound to address once Listen is called. address is not resolved
// here, so building a Server never fails.
func NewServer(address string, metrics *Metrics, readiness *Readiness) *Server {
	return &Server{httpServer: &http.Server{
		Addr:    address,
		Handler: newMux(metrics, readiness, time.Now),
		// Without this, a client that opens a connection and never finishes sending its headers
		// holds a goroutine indefinitely. The endpoints are unauthenticated and Compose publishes
		// them, so the cheapest bound belongs here.
		ReadHeaderTimeout: 5 * time.Second,
	}}
}

// Listen binds the configured address without serving yet, so a bad address is discovered
// before any other startup step.
func (s *Server) Listen() (net.Listener, error) {
	return net.Listen("tcp", s.httpServer.Addr)
}

// Serve accepts connections on listener until Shutdown is called.
func (s *Server) Serve(listener net.Listener) error {
	if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the server without interrupting active connections past ctx's deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
