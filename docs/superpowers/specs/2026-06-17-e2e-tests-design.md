# E2E Tests for the OpenTelemetry Collector Add-on

Date: 2026-06-17
Status: Approved (design)

## Goal

Verify, in GitHub Actions, that the shipped add-on works end-to-end:

1. The MQTT receiver ingests a published message and the collector exports it.
2. The always-on OTLP receiver ingests a pushed signal and the collector exports it.

These are black-box tests against the **actual add-on Docker image**, so they also
cover the bash config-rendering in `rootfs/etc/services.d/otelcol/run`, not just the
Go receiver.

**Implementation note (discovered while building):** the HA `debian-base` s6 init
(`base-addon-banner`/`timezone`/`log-level`) calls the Supervisor API and fatally
stops the container when no Supervisor is present, so the image cannot boot via its
normal entrypoint standalone. The suite therefore overrides the container entrypoint
to `bashio /etc/services.d/otelcol/run`, running the add-on's own run script directly.
The run script reads `/data/options.json` with `jq` and only uses bashio for logging,
so this still exercises the real config rendering, the real image binary, the receiver,
and the export — only the Supervisor-dependent HA platform glue is skipped.

## Scope

- In scope: MQTT metrics ingestion + export; OTLP/HTTP metrics ingestion + export.
- Out of scope: MQTT logs signal, TLS/auth brokers, every exporter type. Unit tests
  already cover `decodeMetrics`. The `file` exporter is the only sink used (the only
  deterministic local sink to assert on).

## Architecture

```
docker network "e2e"
 ┌────────────┐        publish (retained)        ┌─────────────────────────┐
 │ mosquitto  │ <──────────────────────────────  │ host: mosquitto_pub     │
 │ :1883      │                                   └─────────────────────────┘
 └─────┬──────┘
       │  subscribe + ingest
 ┌─────▼─────────────────────────────┐   curl OTLP/HTTP  ┌────────────────────┐
 │ otelcol (add-on image)            │ <──────────────── │ host: curl :4318   │
 │  /data/options.json  (ro, per case)│                   └────────────────────┘
 │  /share              (rw, host dir)│
 │  file exporter -> /share/telemetry.json
 └─────┬─────────────────────────────┘
       │ write
 ┌─────▼───────────────────────┐   poll + jq assert   ┌────────────────────┐
 │ host: ./out/telemetry.json  │ <──────────────────── │ host: run.sh       │
 └─────────────────────────────┘                       └────────────────────┘
```

All files live under `otelcol/tests/e2e/`.

### Components

- `docker-compose.yml` — two services, `mosquitto` and `otelcol`, on one network.
  - `mosquitto`: `eclipse-mosquitto`, started with an anonymous-allowed config.
  - `otelcol`: image `otelcol:e2e` (built by the CI job before compose runs),
    entrypoint overridden to `bashio /etc/services.d/otelcol/run` (see note above).
    - `/data/options.json` ← `./options.json` (read-only, rewritten per case)
    - `/share` ← `./out` (read-write, host-owned, emptied per case)
- `mosquitto.conf` — `listener 1883` + `allow_anonymous true`.
- `options/metrics.json` — add-on options for the MQTT-metrics case.
- `options/otlp.json` — add-on options for the OTLP case.
- `run.sh` — orchestrator and assertions (bash).
- `lib.sh` — small helpers: `poll_file_contains`, `assert_jq`, `cleanup`.

### Why bash + docker-compose

Matches the add-on's bash-centric style, needs no extra toolchain, and assertions are
file/`jq` based. `jq` and `curl` are preinstalled on the GitHub runner;
`mosquitto-clients` is installed by the job.

## Data flow

### Case 1: MQTT metrics

`options/metrics.json`:

```json
{
  "log_level": "debug",
  "mqtt": {
    "broker": "mosquitto",
    "port": 1883,
    "qos": 0,
    "client_id": "otelcol-e2e",
    "signal": "metrics",
    "topics": ["e2e/sensor"]
  },
  "processors": { "batch": { "timeout": "1s" } },
  "exporters": [{ "type": "file", "endpoint": "/share/telemetry.json" }]
}
```

Steps:

1. `compose up -d` both services.
2. Wait for the broker port to accept connections (bounded poll).
3. Publish retained: `mosquitto_pub -r -t e2e/sensor -m '{"temperature":21.5}'`.
   Retained delivery removes the subscribe-vs-publish race: the collector receives the
   message the moment it subscribes, regardless of ordering.
4. Poll `./out/telemetry.json` (bounded, e.g. 60s) until it exists and is non-empty.
5. Assert with `jq` that a gauge named `temperature` with value `21.5` and attribute
   `mqtt.topic == "e2e/sensor"` is present in the exported pdata.
6. `compose down -v`.

### Case 2: OTLP/HTTP metrics

`options/otlp.json`: no `mqtt` block; same `file` exporter; `batch.timeout: 1s`.
The `otelcol` container publishes port `4318` to the host.

Steps:

1. `compose up -d`.
2. Retry `curl -s -o /dev/null -w '%{http_code}'` POST of a minimal OTLP/HTTP JSON
   metrics payload to `http://localhost:4318/v1/metrics` until it returns `200`
   (bounded retry).
3. Poll `./out/telemetry.json` until non-empty.
4. Assert with `jq` the pushed metric name is present in the export.
5. `compose down -v`.

The OTLP/HTTP JSON payload is a single gauge metric, e.g. metric name `e2e.otlp.metric`,
sent with header `Content-Type: application/json`.

## CI wiring

New job in `.github/workflows/ci.yml`, gated to PRs only:

```yaml
  e2e:
    name: E2E (mqtt + otlp)
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - name: Resolve base image
        id: base
        run: |
          build_from=$(grep -E '^\s+amd64:' otelcol/build.yaml | sed -E 's/.*"(.*)".*/\1/')
          [ -n "$build_from" ] || { echo "no amd64 build_from in build.yaml" >&2; exit 1; }
          echo "build_from=$build_from" >> "$GITHUB_OUTPUT"
      - name: Build add-on image
        run: |
          docker build \
            --build-arg BUILD_FROM=${{ steps.base.outputs.build_from }} \
            --build-arg BUILD_ARCH=amd64 \
            -t otelcol:e2e otelcol
      - name: Install mosquitto-clients
        run: sudo apt-get update && sudo apt-get install -y mosquitto-clients
      - name: Run e2e suite
        run: otelcol/tests/e2e/run.sh
```

The push-to-main branch protection (handled by the user) gates merges on this job.

## Error handling / robustness

- **Fail closed**: every poll loop has a bounded timeout and exits non-zero with a clear
  message and a dump of `docker compose logs` + the export file on failure.
- **No magic sleeps**: readiness is detected by polling (broker port, HTTP 200, file
  contents), not fixed `sleep`.
- **Isolation**: each case empties `./out` and runs its own `compose up`/`down -v`, so
  retained messages and broker sessions never leak between cases.
- **Local runnability**: `run.sh` works on a dev machine too; it builds `otelcol:e2e`
  itself if the image is absent, so it is not CI-only.

## Open questions

None.
