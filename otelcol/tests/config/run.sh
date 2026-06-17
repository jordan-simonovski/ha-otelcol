#!/usr/bin/env bash
# Config-rendering suite for the OpenTelemetry Collector add-on.
#
# Renders the real add-on config for a matrix of /data/options.json fixtures and
# runs `otelcol validate` on the layered result (selfmonitoring + generated +
# override). This is the cheap, authoritative check that every option
# permutation produces a config the collector accepts -- no ports, no broker, no
# telemetry round-trip. It catches the boot-loop class of bug (unknown
# component, dangling pipeline ref, bad merge) that the telemetry e2e misses.
#
#   fixtures/valid/*.json   -> validate MUST succeed
#   fixtures/invalid/*.json -> validate MUST fail (proves the check has teeth)
#
# Usage: otelcol/tests/config/run.sh   (builds otelcol:e2e if missing)

set -euo pipefail

cd "$(dirname "$0")"
# shellcheck source=../e2e/lib.sh
source ../e2e/lib.sh

# Reuse build_image from the e2e lib, but relabel log output for this suite.
log() { printf '[config] %s\n' "$*" >&2; }

# render <fixture>: mount the fixture as the add-on options and run the add-on's
# own run script in RENDER_ONLY mode. The HA s6 init needs the Supervisor API to
# boot, so override the entrypoint and invoke the script directly, exactly like
# the e2e compose does.
render() {
    docker run --rm \
        -e RENDER_ONLY=true \
        --entrypoint bashio \
        -v "${PWD}/${1}:/data/options.json:ro" \
        otelcol:e2e /etc/services.d/otelcol/run
}

main() {
    build_image

    local f out fails=0

    for f in fixtures/valid/*.json; do
        if out="$(render "${f}" 2>&1)"; then
            log "PASS (valid):   ${f}"
        else
            log "FAIL (valid):   ${f} -- validate rejected a config it should accept"
            printf '%s\n' "${out}" >&2
            fails=$((fails + 1))
        fi
    done

    for f in fixtures/invalid/*.json; do
        if render "${f}" >/dev/null 2>&1; then
            log "FAIL (invalid): ${f} -- validate accepted a config it should reject"
            fails=$((fails + 1))
        else
            log "PASS (invalid): ${f}"
        fi
    done

    if [ "${fails}" -ne 0 ]; then
        log "${fails} config case(s) failed"
        exit 1
    fi
    log "all config cases passed"
}

main "$@"
