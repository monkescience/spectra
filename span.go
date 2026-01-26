package spectra

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// StartSpan creates a child span for tracing operations within a test.
// The caller is responsible for ending the span with span.End().
//
// Example:
//
//	func TestDatabaseQuery(t *testing.T) {
//	    st := spectra.New(t)
//	    ctx, span := st.StartSpan("db-query")
//	    defer span.End()
//	    result, err := db.Query(ctx, "SELECT ...")
//	}
//
//nolint:spancheck // Caller is responsible for ending the span.
func (t *T) StartSpan(name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(t.ctx, name, opts...)
}

// Setup runs a setup function within a traced span.
// The setup span is automatically ended when the function returns.
//
// Example:
//
//	func TestWithFixtures(t *testing.T) {
//	    st := spectra.New(t)
//	    st.Setup(func(ctx context.Context) {
//	        seedDatabase(ctx)
//	    })
//	}
func (t *T) Setup(fn func(ctx context.Context)) {
	t.Helper()

	ctx, span := t.tracer.Start(
		t.ctx,
		t.Name()+spanSetup,
		trace.WithAttributes(
			attribute.String(attrTestPhase, "setup"),
		),
	)
	defer span.End()

	fn(ctx)
}

// Teardown registers a teardown function that runs within a traced span.
// The teardown is registered via t.Cleanup and runs after the test completes.
//
// Example:
//
//	func TestWithFixtures(t *testing.T) {
//	    st := spectra.New(t)
//	    st.Teardown(func(ctx context.Context) {
//	        cleanupDatabase(ctx)
//	    })
//	}
func (t *T) Teardown(fn func(ctx context.Context)) {
	t.Helper()

	t.Cleanup(func() {
		ctx, span := t.tracer.Start(
			t.ctx,
			t.Name()+spanTeardown,
			trace.WithAttributes(
				attribute.String(attrTestPhase, "teardown"),
			),
		)
		defer span.End()

		fn(ctx)
	})
}
