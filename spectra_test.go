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

// mockTB is a mock testing.TB that doesn't actually fail tests.
type mockTB struct {
	testing.TB
	name     string
	cleanups []func()
	failed   bool
	skipped  bool
}

func newMockTB(name string) *mockTB {
	return &mockTB{name: name}
}

func (m *mockTB) Name() string              { return m.name }
func (m *mockTB) Helper()                   {}
func (m *mockTB) Log(_ ...any)              {}
func (m *mockTB) Logf(_ string, _ ...any)   {}
func (m *mockTB) Error(_ ...any)            { m.failed = true }
func (m *mockTB) Errorf(_ string, _ ...any) { m.failed = true }
func (m *mockTB) Fatal(_ ...any)            { m.failed = true }
func (m *mockTB) Fatalf(_ string, _ ...any) { m.failed = true }
func (m *mockTB) Skip(_ ...any)             { m.skipped = true }
func (m *mockTB) Skipf(_ string, _ ...any)  { m.skipped = true }
func (m *mockTB) Failed() bool              { return m.failed }
func (m *mockTB) Skipped() bool             { return m.skipped }
func (m *mockTB) Cleanup(f func())          { m.cleanups = append(m.cleanups, f) }
func (m *mockTB) TempDir() string           { return "" }
func (m *mockTB) Setenv(_ string, _ string) {}
func (m *mockTB) FailNow()                  { m.failed = true }
func (m *mockTB) Fail()                     { m.failed = true }
func (m *mockTB) SkipNow()                  { m.skipped = true }

func (m *mockTB) runCleanups() {
	for i := len(m.cleanups) - 1; i >= 0; i-- {
		m.cleanups[i]()
	}
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

func TestT_Error(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)
	mock := newMockTB("TestT_Error")

	// when
	st := spectra.New(mock)
	st.Error("test error message")
	st.Errorf("formatted error: %s", "details")
	mock.runCleanups()

	// then
	spans := exporter.GetSpans()

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_Error" {
			targetSpan = s

			break
		}
	}

	errorEvents := 0

	for _, event := range targetSpan.Events {
		if event.Name == "log" {
			for _, attr := range event.Attributes {
				if attr.Key == "level" && attr.Value.AsString() == "error" {
					errorEvents++
				}
			}
		}
	}

	if errorEvents < 2 {
		t.Errorf("expected at least 2 error events, got %d", errorEvents)
	}

	if !mock.failed {
		t.Error("expected mock to be marked as failed")
	}
}

func TestT_Fatal(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)
	mock := newMockTB("TestT_Fatal")

	// when
	st := spectra.New(mock)
	st.Fatal("fatal error")
	mock.runCleanups()

	// then
	spans := exporter.GetSpans()

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_Fatal" {
			targetSpan = s

			break
		}
	}

	fatalFound := false

	for _, event := range targetSpan.Events {
		if event.Name == "log" {
			for _, attr := range event.Attributes {
				if attr.Key == "level" && attr.Value.AsString() == "fatal" {
					fatalFound = true
				}
			}
		}
	}

	if !fatalFound {
		t.Error("expected fatal log event not found")
	}

	if targetSpan.Status.Code != codes.Error {
		t.Error("expected span status to be Error")
	}
}

func TestT_Fatalf(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)
	mock := newMockTB("TestT_Fatalf")

	// when
	st := spectra.New(mock)
	st.Fatalf("fatal error: %s", "formatted")
	mock.runCleanups()

	// then
	spans := exporter.GetSpans()

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_Fatalf" {
			targetSpan = s

			break
		}
	}

	fatalFound := false

	for _, event := range targetSpan.Events {
		if event.Name == "log" {
			for _, attr := range event.Attributes {
				if attr.Key == "level" && attr.Value.AsString() == "fatal" {
					fatalFound = true
				}
			}
		}
	}

	if !fatalFound {
		t.Error("expected fatal log event not found")
	}
}

func TestT_Skip(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)
	mock := newMockTB("TestT_Skip")

	// when
	st := spectra.New(mock)
	st.Skip("skipping test")
	mock.runCleanups()

	// then
	spans := exporter.GetSpans()

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_Skip" {
			targetSpan = s

			break
		}
	}

	skipFound := false

	for _, event := range targetSpan.Events {
		if event.Name == "log" {
			for _, attr := range event.Attributes {
				if attr.Key == "level" && attr.Value.AsString() == "skip" {
					skipFound = true
				}
			}
		}
	}

	if !skipFound {
		t.Error("expected skip log event not found")
	}

	if !mock.skipped {
		t.Error("expected mock to be marked as skipped")
	}
}

func TestT_Skipf(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	exporter := setupTestTracer(t)
	mock := newMockTB("TestT_Skipf")

	// when
	st := spectra.New(mock)
	st.Skipf("skipping: %s", "reason")
	mock.runCleanups()

	// then
	spans := exporter.GetSpans()

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_Skipf" {
			targetSpan = s

			break
		}
	}

	skipFound := false

	for _, event := range targetSpan.Events {
		if event.Name == "log" {
			for _, attr := range event.Attributes {
				if attr.Key == "level" && attr.Value.AsString() == "skip" {
					skipFound = true
				}
			}
		}
	}

	if !skipFound {
		t.Error("expected skip log event not found")
	}
}

func TestT_Parallel(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given
	_ = setupTestTracer(t)

	// when - run in subtest with Parallel.
	t.Run("parallel_test", func(innerT *testing.T) {
		st := spectra.New(innerT)
		st.Parallel()
		st.Log("running in parallel")
	})

	// then - test passes if no panic occurred.
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
