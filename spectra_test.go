package spectra_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"testing"

	"github.com/monkescience/spectra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func setupTestTracer(t *testing.T) (*tracetest.InMemoryExporter, *spectra.Spectra) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)

	sp, err := spectra.Init(
		spectra.WithServiceName("test"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithTracerProvider(tp),
		spectra.WithoutMetrics(),
	)
	if err != nil {
		t.Fatalf("failed to init spectra: %v", err)
	}

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		sp.Shutdown()
	})

	return exporter, sp
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
	for _, cleanup := range slices.Backward(m.cleanups) {
		cleanup()
	}
}

type testPropagator struct{}

func (testPropagator) Inject(_ context.Context, _ propagation.TextMapCarrier) {}

func (testPropagator) Extract(ctx context.Context, _ propagation.TextMapCarrier) context.Context {
	return ctx
}

func (testPropagator) Fields() []string {
	return nil
}

func TestNew(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra instance with an in-memory tracer
	exporter, sp := setupTestTracer(t)

	// when: a test is created and logs a message in a subtest
	t.Run("creates_span", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		st.Log("test message")
	})

	// then: the span for the subtest is recorded
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

	// given: a spectra instance with an in-memory tracer
	exporter, sp := setupTestTracer(t)

	// when: the test logs messages
	t.Run("logs_message", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		st.Log("hello", "world")
		st.Logf("formatted %s", "message")
	})

	// then: the messages are recorded as span log events
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

	// given: a spectra instance with an in-memory tracer
	exporter, sp := setupTestTracer(t)

	// when: the test sets custom attributes
	t.Run("sets_attributes", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		st.SetAttributes(
			attribute.String("custom.key", "custom.value"),
			attribute.Int("custom.number", 42),
		)
	})

	// then: the attributes are recorded on the span
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

	// given: a spectra instance with an in-memory tracer
	exporter, sp := setupTestTracer(t)

	// when: the test adds a custom event
	t.Run("adds_event", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		st.AddEvent("custom.event", attribute.String("key", "value"))
	})

	// then: the event is recorded on the span
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

	// given: a spectra test wrapper
	_, sp := setupTestTracer(t)

	st, err := sp.New(t)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	// when: the caller asks for the test context
	ctx := st.Context()

	// then: a non-nil context is returned
	if ctx == nil {
		t.Error("expected non-nil context")
	}
}

func TestT_Span(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra test wrapper
	_, sp := setupTestTracer(t)

	st, err := sp.New(t)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	// when: the caller asks for the test span
	span := st.Span()

	// then: a non-nil span with a valid context is returned
	if span == nil {
		t.Error("expected non-nil span")
	}

	if !span.SpanContext().IsValid() {
		t.Error("expected valid span context")
	}
}

func TestT_Run(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra instance with an in-memory tracer
	exporter, sp := setupTestTracer(t)

	// when: the test runs a nested subtest
	t.Run("parent", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		st.Run("subtest", func(subST *spectra.T) {
			subST.Log("subtest message")
		})
	})

	// then: both the parent span and the child span are recorded
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

	// given: a spectra instance with an in-memory tracer
	exporter, sp := setupTestTracer(t)

	// when: the test starts a custom child span
	t.Run("creates_child_span", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		ctx, span := st.StartSpan("custom-operation")
		span.End()

		if ctx == nil {
			innerT.Error("expected non-nil context")
		}
	})

	// then: the child span is recorded with the given name
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

	// given: a spectra instance with an in-memory tracer
	exporter, sp := setupTestTracer(t)

	// when: the test runs a setup block
	t.Run("runs_setup", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		setupCalled := false

		st.Setup(func(_ context.Context) {
			setupCalled = true
		})

		if !setupCalled {
			innerT.Error("expected setup function to be called")
		}
	})

	// then: the setup span is recorded
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

	// given: a spectra instance with an in-memory tracer
	exporter, sp := setupTestTracer(t)
	teardownCalled := false

	// when: the test registers a teardown block
	t.Run("runs_teardown", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		st.Teardown(func(_ context.Context) {
			teardownCalled = true
		})

		// Teardown hasn't been called yet.
		if teardownCalled {
			innerT.Error("teardown should not be called until cleanup")
		}
	})

	// then: the teardown span is recorded and the teardown function ran
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

	// given: a spectra instance with an in-memory tracer
	exporter, sp := setupTestTracer(t)

	// when: the test runs without failing
	t.Run("passing", func(innerT *testing.T) {
		_, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}
		// Test passes without any errors.
	})

	// then: the span records status Ok
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

	// given: a spectra instance wrapping a mock testing.TB
	exporter, sp := setupTestTracer(t)
	mock := newMockTB("TestT_Error")

	// when: the test calls Error and Errorf
	st, err := sp.New(mock)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	st.Error("test error message")
	st.Errorf("formatted error: %s", "details")
	mock.runCleanups()

	// then: each call is recorded as an error-level log event and the mock is failed
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
		if event.Name != "log" {
			continue
		}

		for _, attr := range event.Attributes {
			if attr.Key == "level" && attr.Value.AsString() == "error" {
				errorEvents++
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

	// given: a spectra instance wrapping a mock testing.TB
	exporter, sp := setupTestTracer(t)
	mock := newMockTB("TestT_Fatal")

	// when: the test calls Fatal
	st, err := sp.New(mock)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	st.Fatal("fatal error")
	mock.runCleanups()

	// then: a fatal log event is recorded and the span status is Error
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
		if event.Name != "log" {
			continue
		}

		for _, attr := range event.Attributes {
			if attr.Key == "level" && attr.Value.AsString() == "fatal" {
				fatalFound = true
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

	// given: a spectra instance wrapping a mock testing.TB
	exporter, sp := setupTestTracer(t)
	mock := newMockTB("TestT_Fatalf")

	// when: the test calls Fatalf
	st, err := sp.New(mock)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	st.Fatalf("fatal error: %s", "formatted")
	mock.runCleanups()

	// then: a fatal log event is recorded
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
		if event.Name != "log" {
			continue
		}

		for _, attr := range event.Attributes {
			if attr.Key == "level" && attr.Value.AsString() == "fatal" {
				fatalFound = true
			}
		}
	}

	if !fatalFound {
		t.Error("expected fatal log event not found")
	}
}

func TestT_Skip(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra instance wrapping a mock testing.TB
	exporter, sp := setupTestTracer(t)
	mock := newMockTB("TestT_Skip")

	// when: the test calls Skip
	st, err := sp.New(mock)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	st.Skip("skipping test")
	mock.runCleanups()

	// then: a skip log event is recorded and the mock is marked skipped
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
		if event.Name != "log" {
			continue
		}

		for _, attr := range event.Attributes {
			if attr.Key == "level" && attr.Value.AsString() == "skip" {
				skipFound = true
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

	// given: a spectra instance wrapping a mock testing.TB
	exporter, sp := setupTestTracer(t)
	mock := newMockTB("TestT_Skipf")

	// when: the test calls Skipf
	st, err := sp.New(mock)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	st.Skipf("skipping: %s", "reason")
	mock.runCleanups()

	// then: a skip log event is recorded
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
		if event.Name != "log" {
			continue
		}

		for _, attr := range event.Attributes {
			if attr.Key == "level" && attr.Value.AsString() == "skip" {
				skipFound = true
			}
		}
	}

	if !skipFound {
		t.Error("expected skip log event not found")
	}
}

func TestT_Parallel(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra test wrapper
	_, sp := setupTestTracer(t)

	// when: the test marks itself Parallel
	t.Run("parallel_test", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		st.Parallel()
		st.Log("running in parallel")
	})

	// then: the call completes without panicking
}

func TestInit(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a service name and an insecure gRPC endpoint
	// when: spectra.Init is called
	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithInsecure(),
	)
	// then: a valid Spectra instance is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp == nil {
		t.Error("expected non-nil Spectra instance")
	}

	sp.Shutdown()
}

func TestInit_HTTP(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a service name and an HTTP endpoint
	// when: spectra.Init is called
	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("http://localhost:4318"),
	)
	// then: a valid Spectra instance is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp == nil {
		t.Error("expected non-nil Spectra instance")
	}

	sp.Shutdown()
}

func TestInit_HTTPS(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a service name and an HTTPS endpoint
	// when: spectra.Init is called
	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("https://localhost:4318"),
	)
	// then: a valid Spectra instance is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp == nil {
		t.Error("expected non-nil Spectra instance")
	}

	sp.Shutdown()
}

func TestInit_HTTPS_Insecure(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: an HTTPS endpoint paired with WithInsecure to skip cert verification
	// when: spectra.Init is called
	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("https://localhost:4318"),
		spectra.WithInsecure(),
	)
	// then: a valid Spectra instance is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp == nil {
		t.Error("expected non-nil Spectra instance")
	}

	sp.Shutdown()
}

func TestInit_InvalidEndpoint(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: an endpoint without a scheme
	// when: spectra.Init is called
	_, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("localhost:4317"),
	)

	// then: an error is returned
	if err == nil {
		t.Fatal("expected error for endpoint without scheme")
	}
}

func TestInit_DisableTraces(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: WithoutTraces is passed alongside the required options
	// when: spectra.Init is called
	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithoutTraces(),
	)
	// then: a valid Spectra instance is returned even with traces disabled
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp == nil {
		t.Error("expected non-nil Spectra instance")
	}

	sp.Shutdown()
}

func TestInit_WithoutTraces_DisablesSpanCreation(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra instance initialized with WithoutTraces
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)

	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithoutTraces(),
		spectra.WithoutMetrics(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Cleanup(func() {
		sp.Shutdown()
		_ = tp.Shutdown(context.Background())
	})

	mock := newMockTB("TestInit_WithoutTraces_DisablesSpanCreation")

	// when: a test wrapper is created and logs a message
	st, err := sp.New(mock)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	st.Log("this should not create a span")
	mock.runCleanups()

	// then: no spans are recorded by the exporter
	spans := exporter.GetSpans()
	if len(spans) != 0 {
		t.Fatalf("expected no spans when traces are disabled, got %d", len(spans))
	}
}

func TestInit_DisableMetrics(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: WithoutMetrics is passed alongside the required options
	// when: spectra.Init is called
	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithoutMetrics(),
	)
	// then: a valid Spectra instance is returned even with metrics disabled
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp == nil {
		t.Error("expected non-nil Spectra instance")
	}

	sp.Shutdown()
}

func TestInit_DisableLogs(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra instance initialized with WithoutLogs
	exporter, _ := setupTestTracer(t)

	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithoutLogs(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer sp.Shutdown()

	// when: the test logs a message
	t.Run("logs_disabled", func(innerT *testing.T) {
		st, err := sp.New(innerT)
		if err != nil {
			innerT.Fatalf("failed to create test: %v", err)
		}

		st.Log("this should not appear as span event")
	})

	// then: the span is recorded but carries no log events
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

func TestSpectraInit(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: the minimum required options
	// when: spectra.Init is called
	sp, err := spectra.Init(
		spectra.WithServiceName("test"),
		spectra.WithEndpoint("grpc://localhost:4317"),
	)
	// then: a non-nil *Spectra is returned without error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp == nil {
		t.Fatal("expected non-nil *Spectra")
	}

	defer sp.Shutdown()
}

func TestSpectraShutdownIdempotent(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: an initialized spectra instance
	sp, err := spectra.Init(
		spectra.WithServiceName("test"),
		spectra.WithEndpoint("grpc://localhost:4317"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// when: Shutdown is called twice
	sp.Shutdown()
	sp.Shutdown()

	// then: the second call is a no-op and does not panic
}

func TestNewReturnsError(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a nil *Spectra receiver
	var sp *spectra.Spectra

	// when: New is called
	_, err := sp.New(t)

	// then: ErrNotInitialized is returned
	if !errors.Is(err, spectra.ErrNotInitialized) {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestNewAfterShutdown(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra instance that has been shut down
	sp, err := spectra.Init(
		spectra.WithServiceName("test"),
		spectra.WithEndpoint("grpc://localhost:4317"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// when: New is called after Shutdown
	sp.Shutdown()
	_, err = sp.New(t)

	// then: ErrAlreadyShutdown is returned
	if !errors.Is(err, spectra.ErrAlreadyShutdown) {
		t.Errorf("expected ErrAlreadyShutdown, got %v", err)
	}
}

func TestInitMetrics(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: metrics enabled by default and traces disabled to isolate metrics
	// when: spectra.Init is called
	sp, err := spectra.Init(
		spectra.WithServiceName("test"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithoutTraces(),
	)
	// then: Init succeeds and the metrics instruments are created internally
	if err != nil {
		t.Fatalf("unexpected error during init with metrics: %v", err)
	}

	if sp == nil {
		t.Fatal("expected non-nil Spectra instance")
	}

	defer sp.Shutdown()
}

func TestShutdown_RestoresGlobalProviders(t *testing.T) {
	// Tests modify global providers - cannot run in parallel.

	// given: pre-existing OTEL globals and a spectra instance opted into WithSetGlobalProviders
	originalTracerProvider := tracenoop.NewTracerProvider()
	originalMeterProvider := metricnoop.NewMeterProvider()
	originalPropagator := testPropagator{}

	otel.SetTracerProvider(originalTracerProvider)
	otel.SetMeterProvider(originalMeterProvider)
	otel.SetTextMapPropagator(originalPropagator)

	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
		spectra.WithSetGlobalProviders(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// when: Shutdown is called
	sp.Shutdown()

	// then: the original providers and propagator are restored
	if !reflect.DeepEqual(otel.GetTracerProvider(), originalTracerProvider) {
		t.Error("expected tracer provider to be restored on shutdown")
	}

	if !reflect.DeepEqual(otel.GetMeterProvider(), originalMeterProvider) {
		t.Error("expected meter provider to be restored on shutdown")
	}

	if _, ok := otel.GetTextMapPropagator().(testPropagator); !ok {
		t.Error("expected text map propagator to be restored on shutdown")
	}
}

func TestInit_DoesNotTouchGlobals_ByDefault(t *testing.T) {
	// Tests modify global providers - cannot run in parallel.

	// given: pre-existing OTEL global providers
	originalTracerProvider := tracenoop.NewTracerProvider()
	originalMeterProvider := metricnoop.NewMeterProvider()

	otel.SetTracerProvider(originalTracerProvider)
	otel.SetMeterProvider(originalMeterProvider)

	// when: spectra.Init is called without WithSetGlobalProviders
	sp, err := spectra.Init(
		spectra.WithServiceName("test-service"),
		spectra.WithEndpoint("grpc://localhost:4317"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Cleanup(sp.Shutdown)

	// then: the globals remain untouched
	if !reflect.DeepEqual(otel.GetTracerProvider(), originalTracerProvider) {
		t.Error("expected tracer provider to remain untouched when WithSetGlobalProviders is not set")
	}

	if !reflect.DeepEqual(otel.GetMeterProvider(), originalMeterProvider) {
		t.Error("expected meter provider to remain untouched when WithSetGlobalProviders is not set")
	}
}

func TestT_FailNow(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra instance wrapping a mock testing.TB
	exporter, sp := setupTestTracer(t)
	mock := newMockTB("TestT_FailNow")

	// when: the test calls FailNow
	st, err := sp.New(mock)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	st.FailNow()
	mock.runCleanups()

	// then: a fatal log event is recorded, the span status is Error, and the mock is failed
	spans := exporter.GetSpans()

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_FailNow" {
			targetSpan = s

			break
		}
	}

	if targetSpan.Status.Code != codes.Error {
		t.Errorf("expected span status Error, got %v", targetSpan.Status.Code)
	}

	failNowFound := false

	for _, event := range targetSpan.Events {
		if event.Name != "log" {
			continue
		}

		for _, attr := range event.Attributes {
			if attr.Key == "message" && attr.Value.AsString() == "test failed" {
				failNowFound = true

				break
			}
		}
	}

	if !failNowFound {
		t.Error("expected log event with 'test failed' message not found")
	}

	if !mock.failed {
		t.Error("expected mock.failed to be true after FailNow()")
	}
}

func TestT_SkipNow(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra instance wrapping a mock testing.TB
	exporter, sp := setupTestTracer(t)
	mock := newMockTB("TestT_SkipNow")

	// when: the test calls SkipNow
	st, err := sp.New(mock)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	st.SkipNow()
	mock.runCleanups()

	// then: a skip log event is recorded and the mock is marked skipped
	spans := exporter.GetSpans()

	var targetSpan tracetest.SpanStub

	for _, s := range spans {
		if s.Name == "TestT_SkipNow" {
			targetSpan = s

			break
		}
	}

	if targetSpan.Status.Code != codes.Ok {
		t.Errorf("expected span status Ok, got %v", targetSpan.Status.Code)
	}

	skipNowFound := false

	for _, event := range targetSpan.Events {
		if event.Name != "log" {
			continue
		}

		for _, attr := range event.Attributes {
			if attr.Key == "message" && attr.Value.AsString() == "test skipped" {
				skipNowFound = true

				break
			}
		}
	}

	if !skipNowFound {
		t.Error("expected log event with 'test skipped' message not found")
	}

	if !mock.skipped {
		t.Error("expected mock.skipped to be true after SkipNow()")
	}
}

func TestT_ImplementsTestingTB(t *testing.T) {
	// Tests modify global tracer provider - cannot run in parallel.

	// given: a spectra test wrapper around a mock testing.TB
	_, sp := setupTestTracer(t)
	mock := newMockTB("TestT_ImplementsTestingTB")

	st, err := sp.New(mock)
	if err != nil {
		t.Fatalf("failed to create test: %v", err)
	}

	// when: the wrapper is used through the testing.TB interface
	var tb testing.TB = st

	if tb.Failed() {
		t.Error("expected a fresh test not to be failed")
	}

	tb.Error("boom")

	// then: promoted testing.TB methods reflect the wrapped state
	if !tb.Failed() {
		t.Error("expected Failed() to report the failure recorded via Error()")
	}
}

func TestInit_WithLogger_RoutesShutdownErrors(t *testing.T) {
	// given: a spectra instance configured with a custom slog.Logger and a dead endpoint
	var buf bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))

	sp, err := spectra.Init(
		spectra.WithServiceName("test"),
		spectra.WithEndpoint("grpc://127.0.0.1:4317"),
		spectra.WithInsecure(),
		spectra.WithLogger(logger),
		spectra.WithShutdownTimeout(100_000_000), // 100ms
	)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// when: Shutdown is called and fails to flush telemetry
	sp.Shutdown()

	// then: the shutdown error is emitted through the configured slog.Logger
	output := buf.String()
	if len(output) == 0 {
		t.Fatal("expected shutdown errors to be logged via WithLogger")
	}

	if !bytes.Contains(buf.Bytes(), []byte("spectra shutdown failed")) {
		t.Errorf("expected 'spectra shutdown failed' in logger output, got %q", output)
	}
}
