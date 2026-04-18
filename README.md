# spectra

OpenTelemetry instrumentation for Go tests. Make your tests observable and
traceable by streaming spans, metrics, and log events to any OTLP collector.

## Features

- Automatic span creation for tests and subtests
- Log interception (`t.Log` → OTEL span events)
- Metrics recording (duration histogram, test counters)
- Setup/teardown tracing
- Manual span creation for operations under test
- OTLP export to any compatible collector

## Installation

```bash
go get github.com/monkescience/spectra
```

## Quick start

Initialize once in `TestMain`, then wrap each test with `sp.New(t)`. The
examples below assume a package-level `sp *spectra.Spectra` so every test in
the package can use it.

```go
package mypkg_test

import (
    "log"
    "os"
    "testing"

    "github.com/monkescience/spectra"
)

var sp *spectra.Spectra

func TestMain(m *testing.M) {
    var err error

    sp, err = spectra.Init(
        spectra.WithServiceName("my-service-tests"),
        spectra.WithEndpoint("grpc://localhost:4317"),
        spectra.WithInsecure(), // skip TLS verification for local collectors
    )
    if err != nil {
        log.Fatalf("spectra init: %v", err)
    }
    defer sp.Shutdown()

    os.Exit(m.Run())
}
```

## Usage

### Wrap a test

```go
func TestFeature(t *testing.T) {
    st, err := sp.New(t)
    if err != nil {
        t.Fatalf("spectra: %v", err)
    }

    // Logs become span events
    st.Log("starting test")

    // Add custom attributes to the test span
    st.SetAttributes(attribute.String("feature", "login"))

    // Subtests become child spans
    st.Run("validates_input", func(st *spectra.T) {
        // test code...
    })
}
```

### Trace operations under test

```go
func TestDatabaseQuery(t *testing.T) {
    st, err := sp.New(t)
    if err != nil {
        t.Fatalf("spectra: %v", err)
    }

    // Create a child span for the operation you're testing
    ctx, span := st.StartSpan("db-query")
    defer span.End()

    _, err = db.Query(ctx, "SELECT ...")
    if err != nil {
        t.Fatalf("query: %v", err)
    }
}
```

### Setup and teardown

```go
func TestWithFixtures(t *testing.T) {
    st, err := sp.New(t)
    if err != nil {
        t.Fatalf("spectra: %v", err)
    }

    // Setup gets its own span
    st.Setup(func(ctx context.Context) {
        seedDatabase(ctx)
    })

    // Teardown runs on cleanup with its own span
    st.Teardown(func(ctx context.Context) {
        cleanupDatabase(ctx)
    })

    st.Run("query", func(st *spectra.T) {
        // test with seeded data...
    })
}
```

### Install as OTEL globals (opt-in)

By default, spectra keeps its providers local so it does not interfere with
code under test that reads `otel.GetTracerProvider()`. If your code *relies*
on the global providers being spectra's, opt in:

```go
sp, err := spectra.Init(
    spectra.WithServiceName("my-service-tests"),
    spectra.WithEndpoint("grpc://localhost:4317"),
    spectra.WithSetGlobalProviders(), // previous globals are restored on Shutdown
)
```

## Configuration

| Option | Description |
|---|---|
| `WithServiceName(name)` | Service name for telemetry (**required**) |
| `WithEndpoint(endpoint)` | OTLP collector endpoint with scheme (**required**) |
| `WithInsecure()` | gRPC: disable TLS; HTTPS: skip cert verification |
| `WithShutdownTimeout(d)` | Graceful shutdown timeout (default: 5s) |
| `WithLogger(logger)` | `*slog.Logger` for spectra's own operational messages (default: `slog.Default()`) |
| `WithTracerProvider(tp)` | Use an existing `trace.TracerProvider` instead of creating one |
| `WithSetGlobalProviders()` | Install spectra's providers as OTEL globals (off by default) |
| `WithoutTraces()` | Disable trace collection |
| `WithoutMetrics()` | Disable metrics collection |
| `WithoutLogs()` | Disable log capture as span events |

### Endpoint format

The endpoint **must** include a scheme:

| Scheme | Protocol | TLS |
|---|---|---|
| `grpc://host:port` | gRPC | Yes — use `WithInsecure()` to disable |
| `http://host:port` | HTTP | No |
| `https://host:port` | HTTPS | Yes — use `WithInsecure()` to skip cert verification |

## Error handling

Spectra surfaces a small set of sentinel errors you can branch on with
`errors.Is`:

| Error | When | Resolution |
|---|---|---|
| `ErrMissingServiceName` | `spectra.Init` called without `WithServiceName` | Pass `WithServiceName("…")` |
| `ErrMissingEndpoint` | `spectra.Init` called without `WithEndpoint` | Pass `WithEndpoint("grpc://…")` |
| `ErrInvalidEndpoint` | Endpoint missing a scheme | Use `grpc://`, `http://`, or `https://` |
| `ErrNotInitialized` | `sp.New(t)` called before `spectra.Init()` or on a nil `*Spectra` | Call `spectra.Init()` in `TestMain` first |
| `ErrAlreadyShutdown` | Operations attempted after `sp.Shutdown()` | Ensure tests run before shutdown |

## Telemetry

### Traces

- Test span per `sp.New()` call
- Child spans for subtests via `st.Run()`
- Setup/teardown spans
- Custom spans via `st.StartSpan()`
- Span status reflects test pass/fail/skip

### Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `test.duration` | Histogram | Test execution time in seconds |
| `test.count` | Counter | Number of tests by status (pass/fail/skip) |

### Logs

All `t.Log()`, `t.Error()`, `t.Fatal()`, and `t.Skip()` calls are captured as span events with appropriate severity levels.

## License

MIT
