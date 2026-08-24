package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/Stewz00/wattfeder/internal/application"
)

const tracerName = "github.com/Stewz00/wattfeder"

// NewTracerProvider builds a tracer provider that exports spans over OTLP/HTTP to endpoint (for
// example "localhost:4318"), tagging every span with serviceName. Callers must Shutdown it when
// the run ends, bounded by a timeout.
func NewTracerProvider(ctx context.Context, endpoint, serviceName string) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("build OTLP exporter: %w", err)
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	), nil
}

// Tracer is an application.Observer that opens one span per interval, carrying disposition,
// health, decision, and event ID as attributes.
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer wraps provider as an application.Observer.
func NewTracer(provider trace.TracerProvider) *Tracer {
	return &Tracer{tracer: provider.Tracer(tracerName)}
}

// BeginInterval starts a span for one interval; the returned EndInterval sets its attributes
// from the resulting Record and ends the span.
func (t *Tracer) BeginInterval(ctx context.Context) (context.Context, application.EndInterval) {
	spanCtx, span := t.tracer.Start(ctx, "interval")
	return spanCtx, func(record application.Record, err error) {
		span.SetAttributes(
			attribute.String("disposition", string(record.Disposition)),
			attribute.String("health_status", string(record.HealthStatus)),
			attribute.String("decision", string(record.Decision)),
			attribute.String("event_id", string(record.EventID)),
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
}
