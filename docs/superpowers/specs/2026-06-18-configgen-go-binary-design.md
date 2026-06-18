# Config generation as a Go binary

## Problem

The OpenTelemetry Collector config is rendered by `otelcol/rootfs/etc/services.d/otelcol/run`:
roughly 300 lines of `echo`-into-a-file bash with hand-rolled YAML quoting, CSV joining,
and header splitting on `=`. It is a quoting/escaping bug farm, and the only way to test it
is to boot a Docker image and run `otelcol validate`. Every option permutation costs a
container start.

## Goal

Replace the bash config generator with a single Go binary shipped in the image. The binary
reads the add-on options and emits the generated collector config to stdout. This removes the
fragile string-concatenation YAML, and — the real payoff — makes config generation unit-testable
as pure Go with no Docker.

Non-goals: changing the collector's behavior, the options schema, the layered-config model
(selfmonitoring + generated + override), or the e2e telemetry suite.

## Architecture

New Go module `otelcol/configgen/` with its own `go.mod` (mirrors `otelcol/components/mqttreceiver`).
Produces one binary, `otelcol-config`.

- **Input:** reads `/data/options.json` by default; path overridable as the first arg
  (`otelcol-config [path]`). Eases testing.
- **Output:** generated YAML to **stdout**. Diagnostics (mqtt enabled/disabled, skipped exporter,
  fallback warnings) to **stderr** so the `run` script / bashio can still surface them.
- **Rendering:** decode JSON into a typed `Options` struct, build a typed `Config` struct,
  marshal with `gopkg.in/yaml.v3`. No template engine. Correct quoting/escaping for free;
  invalid states are hard to construct.
- **Dependencies:** standard library plus `gopkg.in/yaml.v3`. Nothing else.

### Exit behavior

- Success: write YAML to stdout, exit 0.
- Malformed `options.json` or unreadable input: write error to stderr, exit non-zero.
  The `run` script treats a non-zero generator exit as fatal (the collector cannot start
  without a config).
- All "soft" problems (bad exporter, missing endpoint) are warnings on stderr plus a documented
  fallback, never a hard failure — same as today.

## Behavior parity

The binary reproduces the current `run` semantics exactly. These are the rules the fixtures
depend on:

**Receivers**
- `otlp` (grpc `0.0.0.0:4317`, http `0.0.0.0:4318`) is always present.
- `mqtt` present only when `mqtt.broker` is non-empty.
  - `signal` is `metrics` unless explicitly `logs`.
  - `port`: integer in `[1,65535]`, default `1883` on missing/invalid.
  - `qos`: integer in `[0,2]`, default `0` on missing/invalid.
  - `client_id`: default `otelcol-ha` when empty.
  - `tls`: default `false`.
  - `username` / `password` emitted only when non-empty.
  - `topics`: emitted as a YAML list; empty entries dropped.

**Processors**
- `memory_limiter` fixed: `check_interval: 5s`, `limit_percentage: 80`, `spike_limit_percentage: 25`.
  Not user-tunable.
- `batch`: defaults to `{}`. If any of `timeout` / `send_batch_size` / `send_batch_max_size` is
  set, emit only the set keys. `send_batch_size` / `send_batch_max_size` must be integers
  (invalid values dropped).

**Exporters** (array; each has a `type`)
- `otlp`, `otlphttp`: require `endpoint`; skipped with a warning if missing. Usable in all signals.
  Optional `headers` (`key=value` strings) emitted as a `headers:` map.
- `prometheus`: `endpoint` defaults to `0.0.0.0:8889`. Metrics only.
- `file`: `endpoint` (used as `path`) defaults to `/share/otelcol/telemetry.json`. All signals.
- `clickhouse`: requires `endpoint`; skipped with a warning if missing. Optional
  `database` / `username` / `password` / `ttl` / `timeout` emitted only when non-empty. All signals.
- `debug`: `verbosity: basic`. All signals.
- Unknown type: warning, skipped.
- Exporters are named `<type>/<index>` using their position in the input array (preserves today's
  naming so golden output matches).

**Fallbacks** (order preserved from current script)
- Empty `exporters` array → single `debug` exporter (named `debug`), usable in all signals.
- After processing, if no exporter landed in either the signal bucket or the metric bucket →
  add `debug` fallback.
- If mqtt is enabled with `signal: logs` but the signal bucket is empty → add `debug` fallback
  so the logs pipeline has an exporter.

**Extensions**
- `pprof` (`127.0.0.1:1777`), `zpages` (`127.0.0.1:55679`). `health_check` comes from
  selfmonitoring.yaml, not the generated layer, but the generated `service.extensions` list
  still references `[health_check, pprof, zpages]` (unchanged).

**Service / pipelines**
- `traces` and `logs` pipelines emitted only when the signal-exporter bucket is non-empty;
  receivers `[otlp]` (logs adds `mqtt` when mqtt+logs); processors `[memory_limiter, batch]`.
- `metrics` pipeline emitted only when the metric-exporter bucket is non-empty; receivers
  `[otlp]` (adds `mqtt` when mqtt+metrics); processors `[memory_limiter, batch]`.

> Note on key ordering: yaml.v3 marshals struct fields in declaration order. Struct field order
> is chosen so generated output is stable and readable. Pipeline `receivers`/`processors`/`exporters`
> are sequences; `otelcol validate` does not care about key order, and golden tests pin whatever the
> marshaler produces.

## The `run` script after the change

The generation block (lines ~31–274 today) collapses to a single invocation. The script keeps:
- `log_level` → `bashio::log.level`.
- `extra_config` → write `OVERRIDE`, append `--config`.
- debug-level dump of the generated file.
- `RENDER_ONLY` path: generate, then `exec otelcol validate <config_args>` (validation stays a
  separate step, in the script, not in the binary).
- normal path: generate, then `exec otelcol <config_args>`.

Sketch:

```bash
bashio::log.info "Rendering OpenTelemetry Collector configuration..."
if ! otelcol-config > "${GENERATED}" 2> >(while IFS= read -r l; do bashio::log.info "${l}"; done); then
    bashio::log.error "config generation failed"
    exit 1
fi
```

(Exact stderr plumbing finalized in implementation; the contract is: non-zero exit is fatal,
stderr lines surface in the add-on log.)

## Dockerfile

Compile `configgen` for the target arch in the existing `build` stage (same `BUILD_ARCH` →
`GOARCH` mapping already present), then `COPY --from=build .../otelcol-config /usr/bin/otelcol-config`
in the runtime stage. CGO stays disabled; static binary, no new runtime deps.

## Testing

**Unit (new, primary):** `otelcol/configgen` table-driven Go test runs every
`otelcol/tests/config/fixtures/valid/*.json` and `invalid/*.json` through the generator.
- valid fixtures: assert generated YAML equals a checked-in golden `.yaml`.
- a `-update` flag regenerates goldens so diffs are reviewable in PRs.
- malformed-input fixtures (e.g. `malformed-yaml.json`): assert the generator exits non-zero.
  (`bad-pipeline-ref.json` exercises `extra_config`, which the binary does not touch — that case
  stays an integration concern, see below.)

**Integration (kept):** `otelcol/tests/config/run.sh` still builds the image and runs
`otelcol validate` on the layered config. It remains the authority on "do the components and
merged pipelines actually resolve" — only the collector knows that. It is no longer the only
line of defense, and most permutation coverage moves to the fast Go tests.

**e2e (unchanged):** the telemetry round-trip suite is untouched.

## Files

- `otelcol/configgen/go.mod`, `go.sum`
- `otelcol/configgen/main.go` — entrypoint, arg/stdin handling, exit codes
- `otelcol/configgen/options.go` — `Options` input struct + JSON decode
- `otelcol/configgen/config.go` — `Config` output structs + build logic (parity rules)
- `otelcol/configgen/config_test.go` — golden test harness with `-update`
- `otelcol/configgen/testdata/golden/<fixture>.yaml` — generated goldens, one per valid fixture, keyed by fixture basename
- `otelcol/Dockerfile` — build + copy `otelcol-config`
- `otelcol/rootfs/etc/services.d/otelcol/run` — collapsed generation block
- changeset entry

## Risks

- **Golden churn:** struct-order marshaling differs cosmetically from the bash output. Acceptable;
  goldens are generated once and reviewed. `otelcol validate` confirms semantic correctness.
- **Behavior drift:** any missed parity rule shows up as a failing golden or a failing
  `validate`. The fixture set is the spec; if a rule is not covered by a fixture, add one.
