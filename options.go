package spectra

import "time"

// Option configures spectra initialization.
type Option func(*config)

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
