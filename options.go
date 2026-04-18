package spectra

import (
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Option configures spectra initialization.
type Option func(*config)

// WithLogger sets the slog.Logger used for spectra's own operational messages
// (e.g. shutdown failures). Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) {
		c.Logger = logger
	}
}

// WithServiceName sets the service name for telemetry. Required.
func WithServiceName(name string) Option {
	return func(c *config) {
		c.ServiceName = name
	}
}

// WithEndpoint sets the OTLP collector endpoint. Required.
func WithEndpoint(endpoint string) Option {
	return func(c *config) {
		c.Endpoint = endpoint
	}
}

// WithInsecure disables TLS for the OTLP exporter.
func WithInsecure() Option {
	return func(c *config) {
		c.Insecure = true
	}
}

// WithTracerProvider sets a custom tracer provider.
// When provided, spectra skips creating an OTLP trace exporter.
func WithTracerProvider(provider trace.TracerProvider) Option {
	return func(c *config) {
		c.TracerProvider = provider
	}
}

// WithShutdownTimeout sets the timeout for graceful shutdown.
// Defaults to 5 seconds if not specified.
func WithShutdownTimeout(d time.Duration) Option {
	return func(c *config) {
		c.ShutdownTimeout = d
	}
}

// WithoutTraces disables trace collection.
func WithoutTraces() Option {
	return func(c *config) {
		c.DisableTraces = true
	}
}

// WithoutMetrics disables metrics collection.
func WithoutMetrics() Option {
	return func(c *config) {
		c.DisableMetrics = true
	}
}

// WithoutLogs disables log capture as span events.
func WithoutLogs() Option {
	return func(c *config) {
		c.DisableLogs = true
	}
}

// WithSetGlobalProviders installs spectra's tracer, meter, and propagator as
// the OTEL global providers. Off by default — libraries should not mutate
// global state unless the consumer explicitly opts in. When enabled, the
// previous globals are restored on Shutdown.
func WithSetGlobalProviders() Option {
	return func(c *config) {
		c.SetGlobalProviders = true
	}
}
