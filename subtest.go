package spectra

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Run runs a subtest with its own span as a child of the current test span.
// For parallel tests (when t.Parallel() is called), the subtest span is linked
// to the parent span rather than being a direct child.
//
//nolint:spancheck // Span is ended in innerT.Cleanup, not visible to static analysis.
func (t *T) Run(name string, f func(*T)) bool {
	t.Helper()

	tt, ok := t.TB.(*testing.T)
	if !ok {
		t.Fatal("spectra: Run() requires *testing.T, not *testing.B")

		return false
	}

	return tt.Run(name, func(innerT *testing.T) {
		innerT.Helper()

		ctx, span := t.tracer.Start(
			t.ctx,
			innerT.Name(),
			trace.WithAttributes(
				attribute.String("test.name", innerT.Name()),
				attribute.String("test.parent", t.Name()),
			),
		)

		st := &T{
			TB:     innerT,
			ctx:    ctx,
			span:   span,
			tracer: t.tracer,
		}

		innerT.Cleanup(func() {
			switch {
			case innerT.Failed():
				span.SetStatus(codes.Error, "subtest failed")
			case innerT.Skipped():
				span.SetStatus(codes.Ok, "subtest skipped")
			default:
				span.SetStatus(codes.Ok, "subtest passed")
			}

			span.End()
		})

		f(st)
	})
}

// Parallel marks the test as capable of running in parallel.
// When parallel is used, the span relationship is preserved via span links
// rather than parent-child relationships.
func (t *T) Parallel() {
	t.Helper()

	tt, ok := t.TB.(*testing.T)
	if !ok {
		return
	}

	// Add link to parent span before going parallel.
	t.span.AddEvent("parallel", trace.WithAttributes(
		attribute.String("parent.trace_id", t.span.SpanContext().TraceID().String()),
	))

	tt.Parallel()
}
