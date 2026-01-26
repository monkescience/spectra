// Package spectra provides OpenTelemetry instrumentation for Go tests.
// It wraps testing.TB to automatically create spans, capture logs, and record metrics
// for test execution, making tests observable and traceable.
package spectra

import (
	"context"
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

var globalSpectra *Spectra //nolint:gochecknoglobals // Temporary global until New() is refactored to take *Spectra.

type Spectra struct {
	config         config
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *metric.MeterProvider
	tracer         trace.Tracer
	shutdownOnce   sync.Once
	initialized    bool
}

func (s *Spectra) Shutdown() {
	s.shutdownOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
		defer cancel()

		if s.tracerProvider != nil {
			_ = s.tracerProvider.Shutdown(ctx)
		}

		if s.meterProvider != nil {
			_ = s.meterProvider.Shutdown(ctx)
		}
	})
}

// T wraps testing.TB with OpenTelemetry instrumentation.
// It creates spans for test execution, captures logs, and records metrics.
type T struct {
	testing.TB
	ctx    context.Context //nolint:containedctx // Context is needed for span propagation in tests.
	span   trace.Span
	tracer trace.Tracer

	mu        sync.Mutex
	failed    bool
	startTime time.Time
}

// New creates a new instrumented test wrapper.
// It creates a span for the test and sets up cleanup to end the span
// with the appropriate status when the test completes.
//
//nolint:spancheck // Span is ended in tb.Cleanup, not visible to static analysis.
func New(tb testing.TB) *T {
	tb.Helper()

	tracer := otel.Tracer("spectra")
	ctx, span := tracer.Start(
		context.Background(),
		tb.Name(),
		trace.WithAttributes(
			attribute.String("test.name", tb.Name()),
		),
	)

	t := &T{
		TB:        tb,
		ctx:       ctx,
		span:      span,
		tracer:    tracer,
		startTime: time.Now(),
	}

	tb.Cleanup(func() {
		duration := time.Since(t.startTime)

		t.mu.Lock()
		failed := t.failed
		t.mu.Unlock()

		var status string

		switch {
		case failed || tb.Failed():
			span.SetStatus(codes.Error, "test failed")

			status = "fail"
		case tb.Skipped():
			span.SetStatus(codes.Ok, "test skipped")

			status = "skip"
		default:
			span.SetStatus(codes.Ok, "test passed")

			status = "pass"
		}

		span.End()

		// Record metrics.
		recordTestMetrics(ctx, tb.Name(), duration, status)
	})

	return t
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
	t.TB.Log(args...)

	if globalSpectra == nil || !globalSpectra.config.DisableLogs {
		t.span.AddEvent("log", trace.WithAttributes(
			attribute.String("message", formatArgs(args...)),
			attribute.String("level", "info"),
		))
	}
}

// Logf logs a formatted message and records it as a span event.
func (t *T) Logf(format string, args ...any) {
	t.Helper()
	t.TB.Logf(format, args...)

	if globalSpectra == nil || !globalSpectra.config.DisableLogs {
		t.span.AddEvent("log", trace.WithAttributes(
			attribute.String("message", formatf(format, args...)),
			attribute.String("level", "info"),
		))
	}
}

// Error logs an error and records it as a span event.
func (t *T) Error(args ...any) {
	t.Helper()

	t.mu.Lock()
	t.failed = true
	t.mu.Unlock()

	t.TB.Error(args...)

	if globalSpectra == nil || !globalSpectra.config.DisableLogs {
		t.span.AddEvent("log", trace.WithAttributes(
			attribute.String("message", formatArgs(args...)),
			attribute.String("level", "error"),
		))
	}
}

// Errorf logs a formatted error and records it as a span event.
func (t *T) Errorf(format string, args ...any) {
	t.Helper()

	t.mu.Lock()
	t.failed = true
	t.mu.Unlock()

	t.TB.Errorf(format, args...)

	if globalSpectra == nil || !globalSpectra.config.DisableLogs {
		t.span.AddEvent("log", trace.WithAttributes(
			attribute.String("message", formatf(format, args...)),
			attribute.String("level", "error"),
		))
	}
}

// Fatal logs a fatal error and records it as a span event.
func (t *T) Fatal(args ...any) {
	t.Helper()

	t.mu.Lock()
	t.failed = true
	t.mu.Unlock()

	if globalSpectra == nil || !globalSpectra.config.DisableLogs {
		t.span.AddEvent("log", trace.WithAttributes(
			attribute.String("message", formatArgs(args...)),
			attribute.String("level", "fatal"),
		))
	}

	t.span.SetStatus(codes.Error, "test fatal")
	t.TB.Fatal(args...)
}

// Fatalf logs a formatted fatal error and records it as a span event.
func (t *T) Fatalf(format string, args ...any) {
	t.Helper()

	t.mu.Lock()
	t.failed = true
	t.mu.Unlock()

	if globalSpectra == nil || !globalSpectra.config.DisableLogs {
		t.span.AddEvent("log", trace.WithAttributes(
			attribute.String("message", formatf(format, args...)),
			attribute.String("level", "fatal"),
		))
	}

	t.span.SetStatus(codes.Error, "test fatal")
	t.TB.Fatalf(format, args...)
}

// Skip logs a skip message and records it as a span event.
func (t *T) Skip(args ...any) {
	t.Helper()

	if globalSpectra == nil || !globalSpectra.config.DisableLogs {
		t.span.AddEvent("log", trace.WithAttributes(
			attribute.String("message", formatArgs(args...)),
			attribute.String("level", "skip"),
		))
	}

	t.span.SetStatus(codes.Ok, "test skipped")
	t.TB.Skip(args...)
}

// Skipf logs a formatted skip message and records it as a span event.
func (t *T) Skipf(format string, args ...any) {
	t.Helper()

	if globalSpectra == nil || !globalSpectra.config.DisableLogs {
		t.span.AddEvent("log", trace.WithAttributes(
			attribute.String("message", formatf(format, args...)),
			attribute.String("level", "skip"),
		))
	}

	t.span.SetStatus(codes.Ok, "test skipped")
	t.TB.Skipf(format, args...)
}
