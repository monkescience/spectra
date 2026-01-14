package spectra_test

import (
	"context"
	"testing"

	"github.com/monkescience/spectra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	return exporter
}

func TestNew(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)

	// when - run in subtest so span completes.
	t.Run("creates_span", func(innerT *testing.T) {
		st := spectra.New(innerT)
		st.Log("test message")
	})

	// then - check spans after subtest completes.
	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	found := false

	for _, span := range spans {
		if span.Name == "TestNew/creates_span" {
			found = true

			break
		}
	}

	if !found {
		t.Error("expected span with test name not found")
	}
}

func TestT_Log(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)

	// when
	t.Run("logs_message", func(innerT *testing.T) {
		st := spectra.New(innerT)
		st.Log("hello", "world")
		st.Logf("formatted %s", "message")
	})

	// then
	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_Log/logs_message" {
			targetSpan = s

			break
		}
	}

	events := targetSpan.Events
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	if events[0].Name != "log" {
		t.Errorf("expected event name 'log', got %q", events[0].Name)
	}
}

func TestT_SetAttributes(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)

	// when
	t.Run("sets_attributes", func(innerT *testing.T) {
		st := spectra.New(innerT)
		st.SetAttributes(
			attribute.String("custom.key", "custom.value"),
			attribute.Int("custom.number", 42),
		)
	})

	// then
	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_SetAttributes/sets_attributes" {
			targetSpan = s

			break
		}
	}

	found := false

	for _, attr := range targetSpan.Attributes {
		if attr.Key == "custom.key" && attr.Value.AsString() == "custom.value" {
			found = true

			break
		}
	}

	if !found {
		t.Error("expected custom attribute not found")
	}
}

func TestT_AddEvent(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)

	// when
	t.Run("adds_event", func(innerT *testing.T) {
		st := spectra.New(innerT)
		st.AddEvent("custom.event", attribute.String("key", "value"))
	})

	// then
	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_AddEvent/adds_event" {
			targetSpan = s

			break
		}
	}

	found := false

	for _, event := range targetSpan.Events {
		if event.Name == "custom.event" {
			found = true

			break
		}
	}

	if !found {
		t.Error("expected custom event not found")
	}
}

func TestT_Context(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	_ = setupTestTracer(t)
	st := spectra.New(t)

	// when
	ctx := st.Context()

	// then
	if ctx == nil {
		t.Error("expected non-nil context")
	}
}

func TestT_Span(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	_ = setupTestTracer(t)
	st := spectra.New(t)

	// when
	span := st.Span()

	// then
	if span == nil {
		t.Error("expected non-nil span")
	}

	if !span.SpanContext().IsValid() {
		t.Error("expected valid span context")
	}
}

func TestT_Run(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)

	// when - run parent and subtest.
	t.Run("parent", func(innerT *testing.T) {
		st := spectra.New(innerT)
		st.Run("subtest", func(subST *spectra.T) {
			subST.Log("subtest message")
		})
	})

	// then
	spans := exporter.GetSpans()
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans (parent + subtest), got %d", len(spans))
	}

	// Verify both parent and child spans exist.
	parentFound := false
	childFound := false

	for _, s := range spans {
		if s.Name == "TestT_Run/parent" {
			parentFound = true
		}

		if s.Name == "TestT_Run/parent/subtest" {
			childFound = true
		}
	}

	if !parentFound {
		t.Error("expected parent span not found")
	}

	if !childFound {
		t.Error("expected child span not found")
	}
}

func TestT_StartSpan(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)

	// when
	t.Run("creates_child_span", func(innerT *testing.T) {
		st := spectra.New(innerT)
		ctx, span := st.StartSpan("custom-operation")
		span.End()

		if ctx == nil {
			innerT.Error("expected non-nil context")
		}
	})

	// then
	spans := exporter.GetSpans()
	found := false

	for _, s := range spans {
		if s.Name == "custom-operation" {
			found = true

			break
		}
	}

	if !found {
		t.Error("expected custom span not found")
	}
}

func TestT_Setup(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)

	// when
	t.Run("runs_setup", func(innerT *testing.T) {
		st := spectra.New(innerT)
		setupCalled := false

		st.Setup(func(_ context.Context) {
			setupCalled = true
		})

		if !setupCalled {
			innerT.Error("expected setup function to be called")
		}
	})

	// then
	spans := exporter.GetSpans()
	found := false

	for _, s := range spans {
		if s.Name == "TestT_Setup/runs_setup/setup" {
			found = true

			break
		}
	}

	if !found {
		t.Error("expected setup span not found")
	}
}

func TestT_Teardown(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)
	teardownCalled := false

	// when
	t.Run("runs_teardown", func(innerT *testing.T) {
		st := spectra.New(innerT)

		st.Teardown(func(_ context.Context) {
			teardownCalled = true
		})

		// Teardown hasn't been called yet.
		if teardownCalled {
			innerT.Error("teardown should not be called until cleanup")
		}
	})

	// then - after subtest completes, teardown should have run.
	if !teardownCalled {
		t.Error("expected teardown to be called after test cleanup")
	}

	spans := exporter.GetSpans()
	found := false

	for _, s := range spans {
		if s.Name == "TestT_Teardown/runs_teardown/teardown" {
			found = true

			break
		}
	}

	if !found {
		t.Error("expected teardown span not found")
	}
}

func TestT_SpanStatus_Pass(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)

	// when - run a passing test.
	t.Run("passing", func(innerT *testing.T) {
		_ = spectra.New(innerT)
		// Test passes without any errors.
	})

	// then
	spans := exporter.GetSpans()
	found := false

	for _, s := range spans {
		if s.Name == "TestT_SpanStatus_Pass/passing" && s.Status.Code == codes.Ok {
			found = true

			break
		}
	}

	if !found {
		t.Error("expected span with Ok status not found")
	}
}

func TestInit(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given/when
	shutdown, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithInsecure(),
	)
	// then - should return a valid shutdown function.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if shutdown == nil {
		t.Error("expected non-nil shutdown function")
	}

	// Cleanup.
	shutdown()
}

func TestInit_HTTP(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given/when
	shutdown, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("http://localhost:4318"),
	)
	// then - should return a valid shutdown function.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if shutdown == nil {
		t.Error("expected non-nil shutdown function")
	}

	shutdown()
}

func TestInit_HTTPS(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given/when
	shutdown, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("https://localhost:4318"),
	)
	// then - should return a valid shutdown function.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if shutdown == nil {
		t.Error("expected non-nil shutdown function")
	}

	shutdown()
}

func TestInit_HTTPS_Insecure(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given/when
	shutdown, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("https://localhost:4318"),
		spectra.WithInsecure(),
	)
	// then - should return a valid shutdown function.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if shutdown == nil {
		t.Error("expected non-nil shutdown function")
	}

	shutdown()
}

func TestInit_InvalidEndpoint(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given/when - endpoint without scheme
	_, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("localhost:4317"),
	)

	// then - should return error
	if err == nil {
		t.Fatal("expected error for endpoint without scheme")
	}
}

func TestInit_DisableTraces(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given/when
	shutdown, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithoutTraces(),
	)
	// then - should return a valid shutdown function even with traces disabled.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if shutdown == nil {
		t.Error("expected non-nil shutdown function")
	}

	shutdown()
}

func TestInit_DisableMetrics(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given/when
	shutdown, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithoutMetrics(),
	)
	// then - should return a valid shutdown function even with metrics disabled.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if shutdown == nil {
		t.Error("expected non-nil shutdown function")
	}

	shutdown()
}

func TestInit_DisableLogs(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)

	shutdown, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithoutLogs(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer shutdown()

	// when
	t.Run("logs_disabled", func(innerT *testing.T) {
		st := spectra.New(innerT)
		st.Log("this should not appear as span event")
	})

	// then - span should exist but without log events.
	spans := exporter.GetSpans()

	for _, s := range spans {
		if s.Name == "TestInit_DisableLogs/logs_disabled" {
			for _, event := range s.Events {
				if event.Name == "log" {
					t.Error("expected no log events when DisableLogs is true")
				}
			}

			return
		}
	}
}
