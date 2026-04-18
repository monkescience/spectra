package spectra

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds the test metrics instruments.
type Metrics struct {
	duration metric.Float64Histogram
	count    metric.Int64Counter
}

// initMetrics initializes the metrics instruments.
// This is called automatically by spectra.Init().
func (s *Spectra) initMetrics() error {
	if s == nil || s.meterProvider == nil {
		return nil
	}

	meter := s.meterProvider.Meter("spectra")

	duration, err := meter.Float64Histogram(
		"test.duration",
		metric.WithDescription("Duration of test execution in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("create duration histogram: %w", err)
	}

	count, err := meter.Int64Counter(
		"test.count",
		metric.WithDescription("Number of tests executed"),
		metric.WithUnit("{test}"),
	)
	if err != nil {
		return fmt.Errorf("create count counter: %w", err)
	}

	s.metrics = &Metrics{
		duration: duration,
		count:    count,
	}

	return nil
}

// recordTestMetrics records metrics for a completed test.
func (s *Spectra) recordTestMetrics(ctx context.Context, testName string, duration time.Duration, status string) {
	if s == nil || s.metrics == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String(attrTestName, testName),
		attribute.String(attrTestStatus, status),
	}

	s.metrics.duration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	s.metrics.count.Add(ctx, 1, metric.WithAttributes(attrs...))
}
