#!/usr/bin/env bash
# Shared helpers for the e2e suite. Sourced by run.sh.
#
# All readiness is detected by polling with a bounded timeout; there are no
# fixed sleeps. Every failure path dumps diagnostics and exits non-zero so a
# stuck collector fails the build instead of hanging it.

set -euo pipefail

readonly OUT_FILE="out/telemetry.json"

log() { printf '[e2e] %s\n' "$*" >&2; }

# Dump compose logs and the export file, then abort the run.
fail() {
    log "FAIL: $*"
    log "--- docker compose ps ---"
    docker compose ps >&2 || true
    log "--- docker compose logs ---"
    docker compose logs --no-color >&2 || true
    log "--- ${OUT_FILE} ---"
    cat "${OUT_FILE}" >&2 2>/dev/null || log "(no export file)"
    exit 1
}

# compose_down tears the stack down and removes volumes/networks so retained
# MQTT messages and broker sessions never leak between cases.
compose_down() {
    docker compose down -v --remove-orphans >/dev/null 2>&1 || true
}

# reset_out empties the host-mounted /share dir. Files inside are root-owned
# (written by the container), but removing them only needs write on the dir.
reset_out() {
    rm -rf out
    mkdir -p out
}

# wait_file_nonempty <timeout_s>: block until the export file exists and is
# non-empty.
wait_file_nonempty() {
    local timeout="$1" deadline
    deadline=$(( $(date +%s) + timeout ))
    while [ "$(date +%s)" -lt "${deadline}" ]; do
        if [ -s "${OUT_FILE}" ]; then
            return 0
        fi
        sleep 1
    done
    return 1
}

# assert_jq <jq-filter> <description>: slurp every JSON line of the export file
# into one array and evaluate the filter; the filter must return a truthy value.
assert_jq() {
    local filter="$1" desc="$2"
    if jq -se "${filter}" "${OUT_FILE}" >/dev/null; then
        log "PASS: ${desc}"
    else
        fail "${desc}"
    fi
}

# build_image builds otelcol:e2e if it is not already present (local runs). CI
# builds it in a dedicated step, so this is a no-op there.
build_image() {
    if docker image inspect otelcol:e2e >/dev/null 2>&1; then
        return 0
    fi
    log "otelcol:e2e not found; building it"
    local otelcol_dir base
    otelcol_dir="$(cd ../.. && pwd)"
    base="$(grep -E '^\s+amd64:' "${otelcol_dir}/build.yaml" | sed -E 's/.*"(.*)".*/\1/')"
    [ -n "${base}" ] || fail "could not read amd64 build_from from build.yaml"
    docker build \
        --build-arg "BUILD_FROM=${base}" \
        --build-arg "BUILD_ARCH=amd64" \
        -t otelcol:e2e "${otelcol_dir}"
}
