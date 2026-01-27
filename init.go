package spectra

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
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

	// ErrInvalidEndpoint is returned when endpoint doesn't have a valid scheme.
	ErrInvalidEndpoint = errors.New("endpoint must have scheme (grpc://, http://, or https://)")

	// ErrNotInitialized is returned when Spectra is used before initialization.
	ErrNotInitialized = errors.New("spectra not initialized")

	// ErrAlreadyShutdown is returned when operations are attempted after shutdown.
	ErrAlreadyShutdown = errors.New("spectra already shutdown")
)

type protocol string

const (
	protocolGRPC  protocol = "grpc"
	protocolHTTP  protocol = "http"
	protocolHTTPS protocol = "https"
)

func parseProtocol(endpoint string) (protocol, string, error) {
	switch {
	case strings.HasPrefix(endpoint, "grpc://"):
		return protocolGRPC, strings.TrimPrefix(endpoint, "grpc://"), nil
	case strings.HasPrefix(endpoint, "http://"):
		return protocolHTTP, strings.TrimPrefix(endpoint, "http://"), nil
	case strings.HasPrefix(endpoint, "https://"):
		return protocolHTTPS, strings.TrimPrefix(endpoint, "https://"), nil
	default:
		return "", "", ErrInvalidEndpoint
	}
}

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

// Init initializes OpenTelemetry providers for test instrumentation.
// It returns a Spectra instance that manages the telemetry lifecycle.
//
// ServiceName and Endpoint are required. Endpoint must include a scheme:
//   - grpc://host:port - gRPC protocol
//   - http://host:port - HTTP protocol (no TLS)
//   - https://host:port - HTTPS protocol (TLS)
//
// Example:
//
//	func TestMain(m *testing.M) {
//	    sp, err := spectra.Init(
//	        spectra.WithServiceName("my-service-tests"),
//	        spectra.WithEndpoint("grpc://localhost:4317"),
//	    )
//	    if err != nil {
//	        log.Fatalf("spectra init: %v", err)
//	    }
//	    defer sp.Shutdown()
//	    os.Exit(m.Run())
//	}
func Init(opts ...Option) (*Spectra, error) {
	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}

	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	sp := &Spectra{
		config:      cfg,
		initialized: true,
	}

	ctx := context.Background()

	res, err := createResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	if !cfg.DisableTraces {
		tp, _, err := setupTracing(ctx, cfg, res)
		if err != nil {
			return nil, fmt.Errorf("setup tracing: %w", err)
		}

		sp.tracerProvider = tp
		sp.tracer = tp.Tracer("spectra")
	}

	if !cfg.DisableMetrics {
		mp, _, err := setupMetrics(ctx, cfg, res, sp)
		if err != nil {
			return nil, fmt.Errorf("setup metrics: %w", err)
		}

		sp.meterProvider = mp
	}

	return sp, nil
}

// createResource creates the OTEL resource with service info.
func createResource(cfg config) (*resource.Resource, error) {
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("test"),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	return res, nil
}

// setupTracing configures the trace provider and returns a shutdown function.
func setupTracing(ctx context.Context, cfg config, res *resource.Resource) (*sdktrace.TracerProvider, func(), error) {
	proto, endpoint, err := parseProtocol(cfg.Endpoint)
	if err != nil {
		return nil, nil, err
	}

	var exporter sdktrace.SpanExporter

	switch proto {
	case protocolHTTP:
		exporter, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(endpoint),
			otlptracehttp.WithInsecure(),
		)
	case protocolHTTPS:
		opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint)}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithTLSClientConfig(&tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // User explicitly requested insecure mode.
			}))
		}

		exporter, err = otlptracehttp.New(ctx, opts...)
	case protocolGRPC:
		opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}

		exporter, err = otlptracegrpc.New(ctx, opts...)
	}

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

		err := tp.Shutdown(shutdownCtx)
		if err != nil {
			log.Printf("spectra: failed to shutdown tracer provider: %v", err)
		}
	}, nil
}

// setupMetrics configures the meter provider and returns a shutdown function.
func setupMetrics(
	ctx context.Context,
	cfg config,
	res *resource.Resource,
	sp *Spectra,
) (*metric.MeterProvider, func(), error) {
	proto, endpoint, err := parseProtocol(cfg.Endpoint)
	if err != nil {
		return nil, nil, err
	}

	var exporter metric.Exporter

	switch proto {
	case protocolHTTP:
		exporter, err = otlpmetrichttp.New(ctx,
			otlpmetrichttp.WithEndpoint(endpoint),
			otlpmetrichttp.WithInsecure(),
		)
	case protocolHTTPS:
		opts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(endpoint)}
		if cfg.Insecure {
			opts = append(opts, otlpmetrichttp.WithTLSClientConfig(&tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // User explicitly requested insecure mode.
			}))
		}

		exporter, err = otlpmetrichttp.New(ctx, opts...)
	case protocolGRPC:
		opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(endpoint)}
		if cfg.Insecure {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}

		exporter, err = otlpmetricgrpc.New(ctx, opts...)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("create metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter)),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	err = sp.initMetrics()
	if err != nil {
		return nil, nil, fmt.Errorf("init metrics: %w", err)
	}

	//nolint:contextcheck // Shutdown uses fresh context with timeout, not the init context.
	return mp, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		err := mp.Shutdown(shutdownCtx)
		if err != nil {
			log.Printf("spectra: failed to shutdown meter provider: %v", err)
		}
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
