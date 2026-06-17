#!/usr/bin/env bash
# End-to-end suite for the OpenTelemetry Collector add-on.
#
# Runs the actual add-on image against a real mosquitto broker and asserts that
# telemetry makes it all the way to the file exporter:
#   1. MQTT metrics: a retained publish becomes a gauge in the export.
#   2. OTLP/HTTP metrics: a pushed gauge becomes a gauge in the export.
#
# Usage: otelcol/tests/e2e/run.sh   (builds otelcol:e2e if missing)

set -euo pipefail

cd "$(dirname "$0")"
# shellcheck source=lib.sh
source ./lib.sh

trap compose_down EXIT

readonly MQTT_TOPIC="e2e/sensor"

# --- Case 1: MQTT metrics -----------------------------------------------------
case_mqtt_metrics() {
    log "case: mqtt metrics"
    reset_out
    cp options/metrics.json options.json
    compose_down
    docker compose up -d >/dev/null

    # The publish retry loop doubles as the broker-readiness check: a failed
    # connection just retries. Retained delivery removes the subscribe-vs-publish
    # race: the collector gets the message the instant it subscribes.
    local deadline published=false
    deadline=$(( $(date +%s) + 30 ))
    while [ "$(date +%s)" -lt "${deadline}" ]; do
        if mosquitto_pub -h localhost -p 1883 -r \
            -t "${MQTT_TOPIC}" -m '{"temperature":21.5}' 2>/dev/null; then
            published=true
            break
        fi
        sleep 1
    done
    [ "${published}" = true ] || fail "could not publish to mqtt broker within timeout"

    wait_file_nonempty 60 || fail "no telemetry exported within timeout (mqtt)"

    assert_jq '
        [ .[].resourceMetrics[]?.scopeMetrics[]?.metrics[]?
          | select(.name == "temperature")
          | .gauge.dataPoints[]?
          | select(any((.attributes // [])[]; .key == "mqtt.topic" and .value.stringValue == "'"${MQTT_TOPIC}"'"))
          | .asDouble ]
        | any(. == 21.5)
    ' "mqtt gauge 'temperature'=21.5 with mqtt.topic='${MQTT_TOPIC}' exported"

    compose_down
}

# --- Case 2: OTLP/HTTP metrics ------------------------------------------------
case_otlp_metrics() {
    log "case: otlp/http metrics"
    reset_out
    cp options/otlp.json options.json
    compose_down
    docker compose up -d >/dev/null

    local deadline code
    deadline=$(( $(date +%s) + 30 ))
    while [ "$(date +%s)" -lt "${deadline}" ]; do
        code="$(curl -s -o /dev/null -w '%{http_code}' \
            -H 'Content-Type: application/json' \
            --data @otlp-metric.json \
            http://localhost:4318/v1/metrics || true)"
        [ "${code}" = "200" ] && break
        sleep 1
    done
    [ "${code:-}" = "200" ] || fail "otlp/http endpoint never returned 200 (last: ${code:-none})"

    wait_file_nonempty 60 || fail "no telemetry exported within timeout (otlp)"

    assert_jq '
        [ .[].resourceMetrics[]?.scopeMetrics[]?.metrics[]?
          | select(.name == "e2e.otlp.metric") ]
        | length > 0
    ' "otlp metric 'e2e.otlp.metric' exported"

    compose_down
}

main() {
    build_image
    case_mqtt_metrics
    case_otlp_metrics
    log "all cases passed"
}

main "$@"
