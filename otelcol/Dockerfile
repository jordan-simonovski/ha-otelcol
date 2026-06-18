# syntax=docker/dockerfile:1
# ==============================================================================
# Global build args. ARGs used in a FROM must be declared before the first FROM.
# ==============================================================================
ARG BUILD_FROM
ARG OTELCOL_VERSION=0.153.0

# ==============================================================================
# Stage 1: build a custom OpenTelemetry Collector with OCB (incl. MQTT receiver)
# ==============================================================================
FROM golang:1.26-bookworm AS build

ARG OTELCOL_VERSION
ARG BUILD_ARCH=amd64

ENV CGO_ENABLED=0 \
    GOOS=linux

# Install the OpenTelemetry Collector Builder pinned to the target version.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install "go.opentelemetry.io/collector/cmd/builder@v${OTELCOL_VERSION}"

WORKDIR /src
COPY builder-config.yaml ./builder-config.yaml
COPY components ./components

# Map the Home Assistant architecture to the Go architecture and build.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eux; \
    case "${BUILD_ARCH}" in \
      amd64)   export GOARCH=amd64 ;; \
      aarch64) export GOARCH=arm64 ;; \
      armv7)   export GOARCH=arm GOARM=7 ;; \
      armhf)   export GOARCH=arm GOARM=6 ;; \
      i386)    export GOARCH=386 ;; \
      *) echo "unsupported BUILD_ARCH=${BUILD_ARCH}" >&2; exit 1 ;; \
    esac; \
    "$(go env GOPATH)/bin/builder" --config=builder-config.yaml; \
    test -x /src/_build/otelcol-ha

# ==============================================================================
# Stage 1b: build the config-generation helper binary
# ==============================================================================
FROM golang:1.26-bookworm AS configgen

ARG BUILD_ARCH=amd64
ENV CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src/configgen
COPY configgen/go.mod configgen/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY configgen/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eux; \
    case "${BUILD_ARCH}" in \
      amd64)   export GOARCH=amd64 ;; \
      aarch64) export GOARCH=arm64 ;; \
      armv7)   export GOARCH=arm GOARM=7 ;; \
      armhf)   export GOARCH=arm GOARM=6 ;; \
      i386)    export GOARCH=386 ;; \
      *) echo "unsupported BUILD_ARCH=${BUILD_ARCH}" >&2; exit 1 ;; \
    esac; \
    go build -trimpath -o /out/otelcol-config .; \
    test -x /out/otelcol-config

# ==============================================================================
# Stage 2: runtime image on the Home Assistant Debian base
# ==============================================================================
# hadolint ignore=DL3006
FROM ${BUILD_FROM}

ARG BUILD_ARCH
ARG BUILD_DATE
ARG BUILD_DESCRIPTION
ARG BUILD_NAME
ARG BUILD_REF
ARG BUILD_REPOSITORY
ARG BUILD_VERSION

# curl is needed for the container health check.
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/*

COPY rootfs /
COPY --from=build /src/_build/otelcol-ha /usr/bin/otelcol
COPY --from=configgen /out/otelcol-config /usr/bin/otelcol-config

LABEL \
    io.hass.name="${BUILD_NAME}" \
    io.hass.description="${BUILD_DESCRIPTION}" \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version="${BUILD_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.source="https://github.com/${BUILD_REPOSITORY}" \
    org.opencontainers.image.licenses="Apache-2.0" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.revision="${BUILD_REF}" \
    org.opencontainers.image.version="${BUILD_VERSION}"

HEALTHCHECK --interval=30s --timeout=3s --start-period=30s \
    CMD curl -f http://127.0.0.1:13133/ || exit 1
