package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Stewz00/wattfeder/internal/persistence"
)

// TracedRepository decorates a persistence.Repository with a child span around
// CommitProcessing. persistence and its adapters stay free of any tracing import; the decorator
// is the only place OpenTelemetry meets storage.
type TracedRepository struct {
	persistence.Repository
	tracer trace.Tracer
}

// NewTracedRepository wraps repo, starting commit spans from provider's tracer.
func NewTracedRepository(repo persistence.Repository, provider trace.TracerProvider) *TracedRepository {
	return &TracedRepository{Repository: repo, tracer: provider.Tracer(tracerName)}
}

// CommitProcessing commits result inside a child span of whatever span ctx carries — the
// interval span, when the runtime calls it.
func (r *TracedRepository) CommitProcessing(
	ctx context.Context, result persistence.ObservationResult,
) (persistence.CommitStatus, error) {
	ctx, span := r.tracer.Start(ctx, "commit_processing")
	defer span.End()

	status, err := r.Repository.CommitProcessing(ctx, result)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return status, err
	}

	commitStatus := "stored"
	if status == persistence.CommitDuplicate {
		commitStatus = "duplicate"
	}
	span.SetAttributes(attribute.String("commit_status", commitStatus))
	return status, nil
}
