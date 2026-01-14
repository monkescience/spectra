# spectra

OpenTelemetry instrumentation for Go tests. Make your tests observable and traceable.

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

## Usage

### Initialize in TestMain

```go
func TestMain(m *testing.M) {
    shutdown, err := spectra.Init(
        spectra.WithServiceName("my-service-tests"),
        spectra.WithEndpoint("grpc://localhost:4317"),
        spectra.WithInsecure(), // skip TLS verification
    )
    if err != nil {
        log.Fatalf("spectra init: %v", err)
    }
    defer shutdown()

    os.Exit(m.Run())
}
```

### Wrap Tests

```go
func TestFeature(t *testing.T) {
    st := spectra.New(t)

    // Logs are captured as span events
    st.Log("starting test")

    // Add custom attributes
    st.SetAttributes(attribute.String("feature", "login"))

    // Subtests become child spans
    st.Run("validates_input", func(st *spectra.T) {
        // test code...
    })
}
```

### Trace Operations Under Test

```go
func TestDatabaseQuery(t *testing.T) {
    st := spectra.New(t)

    // Create child span for the operation you're testing
    ctx, span := st.StartSpan("db-query")
    defer span.End()

    result, err := db.Query(ctx, "SELECT ...")
    require.NoError(t, err)
}
```

### Setup and Teardown

```go
func TestWithFixtures(t *testing.T) {
    st := spectra.New(t)

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

## Configuration

| Option | Description |
|--------|-------------|
| `WithServiceName(name)` | Service name for telemetry (required) |
| `WithEndpoint(endpoint)` | OTLP collector endpoint with scheme (required) |
| `WithInsecure()` | gRPC: disable TLS; HTTPS: skip cert verification |
| `WithShutdownTimeout(d)` | Graceful shutdown timeout (default: 5s) |
| `WithoutTraces()` | Disable trace collection |
| `WithoutMetrics()` | Disable metrics collection |
| `WithoutLogs()` | Disable log capture as span events |

### Endpoint Format

The endpoint must include a scheme:

| Scheme | Protocol | TLS |
|--------|----------|-----|
| `grpc://host:port` | gRPC | Yes (use `WithInsecure()` to disable) |
| `http://host:port` | HTTP | No |
| `https://host:port` | HTTPS | Yes (use `WithInsecure()` to skip cert verification) |

## Telemetry

### Traces

- Test span per `spectra.New()` call
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
