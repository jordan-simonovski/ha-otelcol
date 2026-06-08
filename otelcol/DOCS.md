# Home Assistant Add-on: OpenTelemetry Collector

A custom-built [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
with a native **MQTT receiver** and the ability to export to multiple
destinations. Configuration is driven entirely from this add-on's options;
changes take effect when you **Save** and **Restart** the add-on.

## How configuration works

At start, the add-on renders your options into a collector config and launches
the collector with three layered `--config` files (the collector deep-merges
them, last one wins):

1. `selfmonitoring.yaml` - a static internal metrics pipeline.
2. A generated config built from the **MQTT** and **Exporters** options below.
3. Your optional **Raw config override** (`extra_config`).

This is "apply on restart", not hot reload. Edit options, Save, Restart.

## Options

### `mqtt`

Enables the MQTT receiver. Leave `broker` empty to disable it.

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `broker` | string | `""` | Broker hostname/IP (no scheme/port). Empty disables MQTT. |
| `port` | int | `1883` | Broker port (use `8883` with TLS). |
| `topics` | list(string) | `[]` | Topic filters to subscribe to (wildcards `+`/`#` allowed). |
| `qos` | int (0-2) | `0` | MQTT quality of service. |
| `username` | string | - | Optional broker username. |
| `password` | password | - | Optional broker password. |
| `client_id` | string | `otelcol-ha` | MQTT client ID. |
| `tls` | bool | `false` | Connect with TLS (`ssl://`). |
| `signal` | metrics \| logs | `metrics` | What to turn MQTT messages into. |

To consume Home Assistant's MQTT, point `broker` at your MQTT add-on/broker
(commonly `core-mosquitto`) and subscribe to e.g. `homeassistant/#` or
`zigbee2mqtt/#`.

#### `signal: metrics` (default)

Each message is decoded into one or more **gauge** data points, with the source
topic on the `mqtt.topic` attribute. The payload is interpreted as:

- A **JSON object**: every numeric (or boolean) field becomes a gauge named by
  its key; nested objects are flattened with dotted names. Non-numeric fields are
  skipped. Example: `{"temperature":21.5,"humidity":60,"state":"ON"}` yields
  gauges `temperature=21.5` and `humidity=60` (`state` is ignored).
- A **bare number** (JSON or plain text, e.g. `21.5`): one gauge named after the
  topic (slashes become dots, e.g. `home/livingroom/temp` -> `home.livingroom.temp`).
- A **boolean**: a gauge of `1`/`0`.

Messages with no numeric content are dropped.

#### `signal: logs`

Each message becomes **one log record**: the payload is the log body, with
`mqtt.topic`, `mqtt.qos`, `mqtt.retained`, `mqtt.message_id` as attributes.

> The `signal` option feeds whichever single pipeline it names. To process the
> same MQTT data as both metrics and logs at once, see
> [MQTT as both metrics and logs](#mqtt-as-both-metrics-and-logs).

### `exporters`

A list of destinations. The generated config wires non-metric-only exporters
into the `traces`, `logs`, and `metrics` pipelines; `prometheus` is wired into
`metrics` only.

| Field | Type | Description |
| --- | --- | --- |
| `type` | otlp \| otlphttp \| prometheus \| file \| debug | Exporter type. |
| `endpoint` | string | OTLP/OTLP-HTTP endpoint, Prometheus listen address, or file path. |
| `headers` | list(string) | `Key=Value` pairs (OTLP/OTLP-HTTP only). |

Notes:
- `otlp` / `otlphttp` require `endpoint`; entries without one are skipped.
- `prometheus` defaults `endpoint` to `0.0.0.0:8889` (map the port to scrape it).
- `file` defaults `endpoint` (path) to `/share/otelcol/telemetry.json`.
- If no valid exporter remains, the add-on falls back to `debug` so it still boots.

Example exporting to an OTLP backend with an auth header:

```yaml
exporters:
  - type: otlp
    endpoint: "otel.example.com:4317"
    headers:
      - "authorization=Bearer SECRET_TOKEN"
  - type: debug
```

### `extra_config`

Raw OpenTelemetry Collector YAML, applied last and merged over the generated
config. Use it for components/settings not exposed as structured options.

```yaml
extra_config: |
  processors:
    batch:
      timeout: 10s
  exporters:
    otlphttp/loki:
      endpoint: "http://loki:3100/otlp"
```

## MQTT as both metrics and logs

The `mqtt.signal` option wires the MQTT receiver into exactly one pipeline,
which covers the common case. It is not a hard limit of the receiver: the
collector can run the receiver for both signals at once. The MQTT receiver
factory implements both `CreateMetrics` and `CreateLogs`, so a receiver placed
in a metrics pipeline decodes payloads into gauges, and the same component
placed in a logs pipeline stores raw messages as log records.

Use `extra_config` to define the extra wiring. There are two equivalent
approaches.

### Option 1: one receiver in both pipelines

Reference a single `mqtt` receiver from both a `metrics` and a `logs` pipeline.
The collector calls `CreateMetrics` and `CreateLogs` separately, producing two
receiver instances. Each instance automatically appends `-metrics` / `-logs` to
its `client_id`, so they do not evict each other on the broker.

```yaml
extra_config: |
  receivers:
    mqtt:
      broker: core-mosquitto
      topics: ["zigbee2mqtt/#"]
  service:
    pipelines:
      metrics:
        receivers: [otlp, mqtt]
        exporters: [debug]
      logs:
        receivers: [otlp, mqtt]
        exporters: [debug]
```

### Option 2: two named receivers

Define `mqtt/metrics` and `mqtt/logs` as separate named receivers, one per
pipeline. With explicitly named receivers you **must** give them distinct
`client_id` values yourself, otherwise the broker evicts one session.

```yaml
extra_config: |
  receivers:
    mqtt/metrics:
      broker: core-mosquitto
      client_id: ha-metrics
      topics: ["zigbee2mqtt/#"]
    mqtt/logs:
      broker: core-mosquitto
      client_id: ha-logs
      topics: ["zigbee2mqtt/#"]
  service:
    pipelines:
      metrics:
        receivers: [otlp, mqtt/metrics]
        exporters: [debug]
      logs:
        receivers: [otlp, mqtt/logs]
        exporters: [debug]
```

### Cost

Both approaches open **two MQTT connections** and **two subscriptions** to the
same data; there is no shared connection (the collector's `sharedcomponent`
helper is an internal, non-importable package). For Home Assistant traffic this
overhead is negligible.

> When you hand-wire pipelines in `extra_config`, the `service.pipelines` keys
> you define replace the generated ones for those signals (the collector merges
> maps by key, and the override file is applied last). Include the exporters you
> want for those pipelines, as shown above.

## Ports

| Port | Purpose |
| --- | --- |
| `4317/tcp` | OTLP gRPC receiver |
| `4318/tcp` | OTLP HTTP receiver |
| `8889/tcp` | Prometheus exporter (only if a `prometheus` exporter is configured) |

## Changelog

Release history is tracked with [changesets](https://github.com/changesets/changesets)
and recorded in the add-on changelog:
[otelcol/CHANGELOG.md](https://github.com/jordan-simonovski/ha-otelcol/blob/main/otelcol/CHANGELOG.md).

## Constraints

- MQTT maps to either metrics or logs (one `signal` at a time), not traces.
- Apply-on-restart, not hot reload.
- Installs pull a prebuilt multi-arch image from GHCR; the collector is compiled
  in CI, not on the device.
- The add-on runs under a custom AppArmor profile (`apparmor: true`).
- Architectures: `amd64`, `aarch64`.
