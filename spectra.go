// Package spectra provides OpenTelemetry instrumentation for Go tests.
// It wraps testing.TB to automatically create spans, capture logs, and record metrics
// for test execution, making tests observable and traceable.
package spectra

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Event names
	logEventName = "log"

	// Attribute keys
	attrMessage    = "message"
	attrLevel      = "level"
	attrTestName   = "test.name"
	attrTestPhase  = "test.phase"
	attrTestParent = "test.parent"
	attrTestStatus = "test.status"

	// Log levels
	levelInfo  = "info"
	levelError = "error"
	levelFatal = "fatal"
	levelSkip  = "skip"

	// Span name suffixes
	spanSetup    = "/setup"
	spanTeardown = "/teardown"

	// Status strings
	statusPass = "pass"
	statusFail = "fail"
	statusSkip = "skip"
)

type Spectra struct {
	config         config
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *metric.MeterProvider
	tracer         trace.Tracer
	shutdownOnce   sync.Once
	initialized    bool
	shutdown       bool
	mu             sync.RWMutex
}

func (s *Spectra) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.mu.Lock()
		s.shutdown = true
		s.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
		defer cancel()

		if s.tracerProvider != nil {
			if err := s.tracerProvider.Shutdown(ctx); err != nil {
				log.Printf("spectra: failed to shutdown tracer provider: %v", err)
			}
		}

		if s.meterProvider != nil {
			if err := s.meterProvider.Shutdown(ctx); err != nil {
				log.Printf("spectra: failed to shutdown meter provider: %v", err)
			}
		}
	})
}

// T wraps testing.TB with OpenTelemetry instrumentation.
// It creates spans for test execution, captures logs, and records metrics.
type T struct {
	tb      testing.TB
	ctx     context.Context //nolint:containedctx // Context is needed for span propagation in tests.
	span    trace.Span
	tracer  trace.Tracer
	spectra *Spectra

	mu        sync.Mutex
	failed    bool
	startTime time.Time
}

func (t *T) setFailed() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
}

func (t *T) hasFailed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failed
}

func (t *T) recordLog(message, level string) {
	if t.spectra != nil && t.spectra.config.DisableLogs {
		return
	}
	t.span.AddEvent(logEventName, trace.WithAttributes(
		attribute.String(attrMessage, message),
		attribute.String(attrLevel, level),
	))
}

func (t *T) determineStatus() (codes.Code, string, string) {
	switch {
	case t.hasFailed() || t.tb.Failed():
		return codes.Error, "test failed", statusFail
	case t.tb.Skipped():
		return codes.Ok, "test skipped", statusSkip
	default:
		return codes.Ok, "test passed", statusPass
	}
}

func determineSubtestStatus(tb testing.TB) (codes.Code, string) {
	switch {
	case tb.Failed():
		return codes.Error, "subtest failed"
	case tb.Skipped():
		return codes.Ok, "subtest skipped"
	default:
		return codes.Ok, "subtest passed"
	}
}

// New creates a new instrumented test wrapper.
// It creates a span for the test and sets up cleanup to end the span
// with the appropriate status when the test completes.
//
//nolint:spancheck // Span is ended in tb.Cleanup, not visible to static analysis.
func (s *Spectra) New(tb testing.TB) (*T, error) {
	tb.Helper()

	if s == nil || !s.initialized {
		return nil, ErrNotInitialized
	}

	s.mu.RLock()
	shutdown := s.shutdown
	s.mu.RUnlock()

	if shutdown {
		return nil, ErrAlreadyShutdown
	}

	tracer := s.tracer
	if tracer == nil {
		tracer = otel.Tracer("spectra")
	}

	ctx, span := tracer.Start(
		context.Background(),
		tb.Name(),
		trace.WithAttributes(
			attribute.String(attrTestName, tb.Name()),
		),
	)

	t := &T{
		tb:        tb,
		ctx:       ctx,
		span:      span,
		tracer:    tracer,
		spectra:   s,
		startTime: time.Now(),
	}

	tb.Cleanup(func() {
		duration := time.Since(t.startTime)

		code, message, status := t.determineStatus()
		span.SetStatus(code, message)

		span.End()

		recordTestMetrics(ctx, tb.Name(), duration, status)
	})

	return t, nil
}

// Name returns the name of the test.
func (t *T) Name() string {
	return t.tb.Name()
}

// Helper marks the calling function as a test helper function.
func (t *T) Helper() {
	t.tb.Helper()
}

// Cleanup registers a function to be called when the test completes.
func (t *T) Cleanup(f func()) {
	t.tb.Cleanup(f)
}

// Context returns the context associated with this test's span.
func (t *T) Context() context.Context {
	return t.ctx
}

// Span returns the span associated with this test.
func (t *T) Span() trace.Span {
	return t.span
}

// SetAttributes adds attributes to the test span.
func (t *T) SetAttributes(attrs ...attribute.KeyValue) {
	t.span.SetAttributes(attrs...)
}

// AddEvent adds an event to the test span.
func (t *T) AddEvent(name string, attrs ...attribute.KeyValue) {
	t.span.AddEvent(name, trace.WithAttributes(attrs...))
}

// Log logs a message and records it as a span event.
func (t *T) Log(args ...any) {
	t.Helper()
	t.tb.Log(args...)

	t.recordLog(formatArgs(args...), levelInfo)
}

// Logf logs a formatted message and records it as a span event.
func (t *T) Logf(format string, args ...any) {
	t.Helper()
	t.tb.Logf(format, args...)

	t.recordLog(formatf(format, args...), levelInfo)
}

// Error logs an error and records it as a span event.
func (t *T) Error(args ...any) {
	t.Helper()

	t.setFailed()

	t.tb.Error(args...)

	t.recordLog(formatArgs(args...), levelError)
}

// Errorf logs a formatted error and records it as a span event.
func (t *T) Errorf(format string, args ...any) {
	t.Helper()

	t.setFailed()

	t.tb.Errorf(format, args...)

	t.recordLog(formatf(format, args...), levelError)
}

// Fatal logs a fatal error and records it as a span event.
func (t *T) Fatal(args ...any) {
	t.Helper()

	t.setFailed()

	t.recordLog(formatArgs(args...), levelFatal)

	t.span.SetStatus(codes.Error, "test fatal")
	t.tb.Fatal(args...)
}

// Fatalf logs a formatted fatal error and records it as a span event.
func (t *T) Fatalf(format string, args ...any) {
	t.Helper()

	t.setFailed()

	t.recordLog(formatf(format, args...), levelFatal)

	t.span.SetStatus(codes.Error, "test fatal")
	t.tb.Fatalf(format, args...)
}

// Skip logs a skip message and records it as a span event.
func (t *T) Skip(args ...any) {
	t.Helper()

	t.recordLog(formatArgs(args...), levelSkip)

	t.span.SetStatus(codes.Ok, "test skipped")
	t.tb.Skip(args...)
}

// Skipf logs a formatted skip message and records it as a span event.
func (t *T) Skipf(format string, args ...any) {
	t.Helper()

	t.recordLog(formatf(format, args...), levelSkip)

	t.span.SetStatus(codes.Ok, "test skipped")
	t.tb.Skipf(format, args...)
}
