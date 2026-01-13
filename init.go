package spectra

import (
	"context"
	"os"
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

const (
	defaultServiceName = "test-service"
	defaultEndpoint    = "localhost:4317"
	shutdownTimeout    = 5 * time.Second
)

// Config holds configuration for spectra initialization.
type Config struct {
	// ServiceName is the name of the service for telemetry.
	// Defaults to "test-service" or OTEL_SERVICE_NAME env var.
	ServiceName string

	// Endpoint is the OTLP collector endpoint.
	// Defaults to "localhost:4317" or OTEL_EXPORTER_OTLP_ENDPOINT env var.
	Endpoint string

	// Insecure disables TLS for the OTLP exporter.
	// Defaults to true for local development.
	Insecure bool
}

// Init initializes OpenTelemetry providers for test instrumentation.
// It returns a shutdown function that should be deferred in TestMain.
//
// Example:
//
//	func TestMain(m *testing.M) {
//	    shutdown := spectra.Init(spectra.Config{
//	        ServiceName: "my-service-tests",
//	    })
//	    defer shutdown()
//	    os.Exit(m.Run())
//	}
func Init(cfg Config) func() {
	cfg = applyDefaults(cfg)
	ctx := context.Background()
	res := createResource(cfg)

	tp, traceShutdown := setupTracing(ctx, cfg, res)
	if tp == nil {
		return func() {}
	}

	_, metricShutdown := setupMetrics(ctx, cfg, res)

	return func() {
		traceShutdown()

		if metricShutdown != nil {
			metricShutdown()
		}
	}
}

// createResource creates the OTEL resource with service info.
func createResource(cfg Config) *resource.Resource {
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
func setupTracing(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, func()) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	//nolint:contextcheck // Shutdown uses fresh context with timeout, not the init context.
	return tp, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		_ = tp.Shutdown(shutdownCtx)
	}
}

// setupMetrics configures the meter provider and returns a shutdown function.
func setupMetrics(ctx context.Context, cfg Config, res *resource.Resource) (*metric.MeterProvider, func()) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter)),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	initMetrics()

	//nolint:contextcheck // Shutdown uses fresh context with timeout, not the init context.
	return mp, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		_ = mp.Shutdown(shutdownCtx)
	}
}

// applyDefaults fills in default values for the config.
func applyDefaults(cfg Config) Config {
	if cfg.ServiceName == "" {
		cfg.ServiceName = os.Getenv("OTEL_SERVICE_NAME")
		if cfg.ServiceName == "" {
			cfg.ServiceName = defaultServiceName
		}
	}

	if cfg.Endpoint == "" {
		cfg.Endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		if cfg.Endpoint == "" {
			cfg.Endpoint = defaultEndpoint
		}
	}

	// Default to insecure for local development.
	if !cfg.Insecure {
		cfg.Insecure = true
	}

	return cfg
}
