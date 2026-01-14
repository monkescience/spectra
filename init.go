package spectra

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const defaultShutdownTimeout = 5 * time.Second

var (
	// ErrMissingServiceName is returned when ServiceName is not configured.
	ErrMissingServiceName = errors.New("service name is required")

	// ErrMissingEndpoint is returned when Endpoint is not configured.
	ErrMissingEndpoint = errors.New("endpoint is required")
)

// config holds configuration for spectra initialization.
type config struct {
	// ServiceName is the name of the service for telemetry. Required.
	// Can also be set via OTEL_SERVICE_NAME env var.
	ServiceName string

	// Endpoint is the OTLP collector endpoint. Required.
	// Can also be set via OTEL_EXPORTER_OTLP_ENDPOINT env var.
	Endpoint string

	// Insecure disables TLS for the OTLP exporter.
	Insecure bool

	// ShutdownTimeout is the timeout for graceful shutdown.
	// Defaults to 5 seconds.
	ShutdownTimeout time.Duration

	// DisableTraces disables trace collection.
	DisableTraces bool

	// DisableMetrics disables metrics collection.
	DisableMetrics bool

	// DisableLogs disables log capture as span events.
	DisableLogs bool
}

var globalConfig config //nolint:gochecknoglobals // config needs to be accessible from T methods.

// Init initializes OpenTelemetry providers for test instrumentation.
// It returns a shutdown function that should be deferred in TestMain.
//
// ServiceName and Endpoint are required (either via options or env vars).
//
// Example:
//
//	func TestMain(m *testing.M) {
//	    shutdown, err := spectra.Init(
//	        spectra.WithServiceName("my-service-tests"),
//	        spectra.WithEndpoint("localhost:4317"),
//	    )
//	    if err != nil {
//	        log.Fatalf("spectra init: %v", err)
//	    }
//	    defer shutdown()
//	    os.Exit(m.Run())
//	}
func Init(opts ...Option) (func(), error) {
	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}

	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	globalConfig = cfg

	ctx := context.Background()
	res := createResource(cfg)

	var traceShutdown, metricShutdown func()

	if !cfg.DisableTraces {
		_, traceShutdown, err = setupTracing(ctx, cfg, res)
		if err != nil {
			return nil, fmt.Errorf("setup tracing: %w", err)
		}
	}

	if !cfg.DisableMetrics {
		_, metricShutdown, err = setupMetrics(ctx, cfg, res)
		if err != nil {
			return nil, fmt.Errorf("setup metrics: %w", err)
		}
	}

	return func() {
		if traceShutdown != nil {
			traceShutdown()
		}

		if metricShutdown != nil {
			metricShutdown()
		}
	}, nil
}

// createResource creates the OTEL resource with service info.
func createResource(cfg config) *resource.Resource {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("test"),
		),
	)
	if err != nil {
		return resource.Default()
	}

	return res
}

// setupTracing configures the trace provider and returns a shutdown function.
func setupTracing(ctx context.Context, cfg config, res *resource.Resource) (*sdktrace.TracerProvider, func(), error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	//nolint:contextcheck // Shutdown uses fresh context with timeout, not the init context.
	return tp, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		_ = tp.Shutdown(shutdownCtx)
	}, nil
}

// setupMetrics configures the meter provider and returns a shutdown function.
func setupMetrics(ctx context.Context, cfg config, res *resource.Resource) (*metric.MeterProvider, func(), error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter)),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	initMetrics()

	//nolint:contextcheck // Shutdown uses fresh context with timeout, not the init context.
	return mp, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		_ = mp.Shutdown(shutdownCtx)
	}, nil
}

// validateConfig validates required fields and sets defaults.
func validateConfig(cfg config) (config, error) {
	if cfg.ServiceName == "" {
		return cfg, ErrMissingServiceName
	}

	if cfg.Endpoint == "" {
		return cfg, ErrMissingEndpoint
	}

	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}

	return cfg, nil
}
