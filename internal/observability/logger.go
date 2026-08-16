// Package observability adapts the application runtime's Observer seam to concrete
// infrastructure — structured logs today, metrics and tracing as later tasks add them. Nothing
// in internal/application or internal/household imports this package; it depends on them, never
// the other way around.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/household"
)

// ParseLevel maps the -log-level flag's values to a slog.Level. It accepts "debug", "info",
// "warn", and "error"; anything else is rejected rather than silently defaulted.
func ParseLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid -log-level value %q: must be \"debug\", \"info\", \"warn\" or \"error\"", level)
	}
}

// Logger is an application.Observer that writes one structured log line per interval. It never
// writes anything but log lines: the record stream on stdout is untouched.
type Logger struct {
	log *slog.Logger
}

// NewLogger wraps log as an application.Observer.
func NewLogger(log *slog.Logger) *Logger {
	return &Logger{log: log}
}

// BeginInterval starts timing one interval; the returned EndInterval logs it when the interval
// closes.
func (l *Logger) BeginInterval(ctx context.Context) (context.Context, application.EndInterval) {
	start := time.Now()
	spanContext := trace.SpanContextFromContext(ctx)
	return ctx, func(record application.Record, err error) {
		l.logInterval(record, err, time.Since(start), spanContext)
	}
}

func (l *Logger) logInterval(record application.Record, err error, duration time.Duration, spanContext trace.SpanContext) {
	attrs := []slog.Attr{}
	if spanContext.HasTraceID() {
		attrs = append(attrs, slog.String("trace_id", spanContext.TraceID().String()))
	}
	attrs = append(attrs,
		slog.String("agent_id", record.AgentID),
		slog.String("device_id", record.DeviceID),
		slog.String("event_id", string(record.EventID)),
		slog.String("disposition", string(record.Disposition)),
		slog.String("disposition_reason", record.DispositionReason),
		slog.String("health_status", string(record.HealthStatus)),
		slog.String("decision", string(record.Decision)),
		slog.Int64("duration_ms", duration.Milliseconds()),
	)
	if record.Timestamp != nil {
		attrs = append(attrs, slog.Float64("event_lag_seconds", record.ReceivedAt.Sub(*record.Timestamp).Seconds()))
	}

	level := slog.LevelInfo
	switch record.Disposition {
	case household.DispositionRejected, household.DispositionUnavailable:
		level = slog.LevelWarn
	}
	if err != nil {
		level = slog.LevelError
		attrs = append(attrs, slog.String("error", err.Error()))
	}

	l.log.LogAttrs(context.Background(), level, "interval_processed", attrs...)
}
