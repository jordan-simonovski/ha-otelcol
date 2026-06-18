# otelcol-addon

## 0.4.0

### Minor Changes

- ea1ef4a: Generate the collector config with a shipped `otelcol-config` Go binary instead
  of the bash run script. Config generation is now unit-tested against golden
  files with no Docker, and the run script no longer hand-rolls YAML quoting.

## 0.3.2

### Patch Changes

- 0cfff12: Removing apparmor which is likely causing issues

## 0.3.1

### Patch Changes

- 62ef5ff: Implementing better debug logging

## 0.3.0

### Minor Changes

- 4473b69: Ship the otelcol processors config fix that was merged but never published. The
  `v0.2.0` image was tagged from an out-of-band version edit before the fix landed,
  so the build pipeline skipped it. Cutting a fresh, untagged version releases it.

## 0.2.0

### Minor Changes

- e4592ce: Enable the bundled AppArmor profile (`apparmor: true`) to confine the collector,
  and ship prebuilt multi-arch images from GHCR (`image:`) so installs download
  instead of compiling on the device. Adds CI (changeset/test/lint/apparmor/build)
  and a version-gated multi-arch publish workflow.

### Patch Changes

- 7b86c35: Fixing issues with otel collector config for processors.

## 0.2.0

### Minor Changes

- Add a `clickhouse` exporter type. Compiled in via the contrib
  `clickhouseexporter` and configurable through structured options (`endpoint`,
  `database`, `username`, `password`, `ttl`, `timeout`). It is wired into the
  traces, logs, and metrics pipelines. Retry-on-failure is left at the
  exporter's defaults (enabled, 5s/30s/300s), which already match a typical
  ClickHouse Cloud setup; tune it via `extra_config` if needed.
- Expose `batch` processor tuning via a `processors.batch` option block
  (`timeout`, `send_batch_size`, `send_batch_max_size`). Unset fields fall back
  to the collector defaults (`batch: {}`). `memory_limiter` stays fixed as an
  internal safety guard.

## 0.1.2

### Patch Changes

- Log an explicit `mqtt: connected` line (broker, port, client_id) on connect so
  boot-time MQTT connectivity is observable instead of inferred from the
  subscribe log.
- Add a `log_level` option and emit the rendered collector config at debug level
  only, so a restart loop no longer spams the log with the full config on every
  boot.

## 0.1.1

### Patch Changes

- Fix the add-on image build on Home Assistant. `BUILD_FROM` is now declared as a
  global Docker `ARG` before the first `FROM`, so the runtime stage correctly
  resolves the Home Assistant base image instead of failing with
  `base name (${BUILD_FROM}) should not be blank`.
