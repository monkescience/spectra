package spectra

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Run runs a subtest with its own span as a direct child of the current test
// span. The parent-child span relationship holds even when the subtest calls
// Parallel.
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
				attribute.String(attrTestName, innerT.Name()),
				attribute.String(attrTestParent, t.Name()),
			),
		)

		st := &T{
			TB:      innerT,
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

// Parallel signals that this test is to be run in parallel with (and only
// with) other parallel tests, delegating to the underlying *testing.T. It
// records a "parallel" event on the test span to mark the transition and is a
// no-op for benchmarks. The subtest span remains a direct child of its parent.
func (t *T) Parallel() {
	t.Helper()

	tt, ok := t.TB.(*testing.T)
	if !ok {
		return
	}

	t.span.AddEvent("parallel")

	tt.Parallel()
}
