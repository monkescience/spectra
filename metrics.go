package spectra

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	metricsOnce sync.Once //nolint:gochecknoglobals // Singleton initialization.
	testMetrics *Metrics  //nolint:gochecknoglobals // Global metrics instance.
)

// Metrics holds the test metrics instruments.
type Metrics struct {
	duration metric.Float64Histogram
	count    metric.Int64Counter
}

// initMetrics initializes the metrics instruments.
// This is called automatically by spectra.Init().
func (s *Spectra) initMetrics() error {
	var initErr error
	metricsOnce.Do(func() {
		meter := otel.Meter("spectra")

		duration, err := meter.Float64Histogram(
			"test.duration",
			metric.WithDescription("Duration of test execution in seconds"),
			metric.WithUnit("s"),
		)
		if err != nil {
			initErr = fmt.Errorf("create duration histogram: %w", err)
			return
		}

		count, err := meter.Int64Counter(
			"test.count",
			metric.WithDescription("Number of tests executed"),
			metric.WithUnit("{test}"),
		)
		if err != nil {
			initErr = fmt.Errorf("create count counter: %w", err)
			return
		}

		testMetrics = &Metrics{
			duration: duration,
			count:    count,
		}
	})
	return initErr
}

// recordTestMetrics records metrics for a completed test.
func recordTestMetrics(ctx context.Context, testName string, duration time.Duration, status string) {
	if testMetrics == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String(attrTestName, testName),
		attribute.String(attrTestStatus, status),
	}

	testMetrics.duration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	testMetrics.count.Add(ctx, 1, metric.WithAttributes(attrs...))
}
