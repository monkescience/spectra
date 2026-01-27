package spectra

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Run runs a subtest with its own span as a child of the current test span.
// For parallel tests (when t.Parallel() is called), the subtest span is linked
// to the parent span rather than being a direct child.
func (t *T) Run(name string, f func(*T)) bool {
	t.Helper()

	tt, ok := t.tb.(*testing.T)
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
				attribute.String(attrTestName, innerT.Name()),
				attribute.String(attrTestParent, t.Name()),
			),
		)

		st := &T{
			tb:      innerT,
			ctx:     ctx,
			span:    span,
			tracer:  t.tracer,
			spectra: t.spectra,
		}

		innerT.Cleanup(func() {
			code, message := determineSubtestStatus(innerT)
			span.SetStatus(code, message)

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

	tt, ok := t.tb.(*testing.T)
	if !ok {
		return
	}

	// Add link to parent span before going parallel.
	t.span.AddEvent("parallel", trace.WithAttributes(
		attribute.String("parent.trace_id", t.span.SpanContext().TraceID().String()),
	))

	tt.Parallel()
}
