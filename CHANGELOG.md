# Changelog

## [v0.3.0](https://github.com/monkescience/spectra/compare/v0.2.0...v0.3.0) (2026-04-18)

### ⚠ BREAKING CHANGES

- migrate release pipeline from release-please to yeet ([b15aa7d](https://github.com/monkescience/spectra/commit/b15aa7d65b9492f0c89e14b13fb0d4fe86fa9c1e))
- make OTEL global provider registration opt-in ([d7722ef](https://github.com/monkescience/spectra/commit/d7722ef09971737a66c33412bff5c116d1b98ffd))
### Features

- make OTEL global provider registration opt-in ([d7722ef](https://github.com/monkescience/spectra/commit/d7722ef09971737a66c33412bff5c116d1b98ffd))
- add WithLogger option, use slog for shutdown errors ([87ab4fe](https://github.com/monkescience/spectra/commit/87ab4fe38b8c10ac3f663d879a81fc9276144559))

## [0.2.0](https://github.com/monkescience/spectra/compare/v0.1.0...v0.2.0) (2026-04-18)


### ⚠ BREAKING CHANGES

* **api:** return error from Init instead of panicking

### Features

* add OpenTelemetry test instrumentation library ([210ade4](https://github.com/monkescience/spectra/commit/210ade434d6d76804cfd51882dea034eefe1f181))
* **api:** add FailNow and SkipNow methods to T ([4909f66](https://github.com/monkescience/spectra/commit/4909f663ffc99bc118df08208ab9da12b7a9469b))
* **api:** add functional options and disable controls ([c592a00](https://github.com/monkescience/spectra/commit/c592a00b0e74d1dc0cbf3a10c1ad8a6dc6766f63))
* **api:** add support for custom tracer provider and restore global state on shutdown ([9a8a1ee](https://github.com/monkescience/spectra/commit/9a8a1eeef78607c30859210c59dbd163d39b3213))
* **transport:** add HTTP/HTTPS protocol support ([e61e494](https://github.com/monkescience/spectra/commit/e61e4941bbbb89a27be0d429d48d08e1b02bfbaf))


### Bug Fixes

* **api:** return error from Init instead of panicking ([52e0ce1](https://github.com/monkescience/spectra/commit/52e0ce138730f337f5b5fbc026f009a996a45ff8))
* **deps:** update module go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp to v1.43.0 [security] ([#30](https://github.com/monkescience/spectra/issues/30)) ([d084acf](https://github.com/monkescience/spectra/commit/d084acf8639838b2704af14f91bd0f6443f76e3e))
* **deps:** update module go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp to v1.43.0 [security] ([#31](https://github.com/monkescience/spectra/issues/31)) ([263f32a](https://github.com/monkescience/spectra/commit/263f32a5a037d5a65257da9f7375daa6b67577d2))
* **deps:** update opentelemetry-go monorepo to v1.40.0 ([#12](https://github.com/monkescience/spectra/issues/12)) ([8cf25bd](https://github.com/monkescience/spectra/commit/8cf25bda5a03e8f13734f332d967ee31f90eb236))
* **errors:** remove silent failures in metrics and resource creation ([05a6f63](https://github.com/monkescience/spectra/commit/05a6f637f12c5d5cfb4be82c01a1d778faa7875b))
