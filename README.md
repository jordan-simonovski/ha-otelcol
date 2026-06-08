# Home Assistant OpenTelemetry Collector Add-on

[![Open your Home Assistant instance and show the add add-on repository dialog with a specific repository URL pre-filled.](https://my.home-assistant.io/badges/supervisor_add_addon_repository.svg)](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2Fjordan-simonovski%2Fha-otelcol)

A Home Assistant add-on that runs a **custom-built** [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
including an **MQTT receiver** (built into the binary, since upstream ships none) and
the ability to export telemetry to **multiple destinations** (OTLP, OTLP/HTTP,
Prometheus, file, debug).

## Why a custom build?

The upstream `opentelemetry-collector-contrib` distribution does **not** include an
MQTT receiver, and no maintained third-party module exists. To get native MQTT
ingestion into the collector pipeline, this add-on builds its own collector
distribution with the [OpenTelemetry Collector Builder (OCB)](https://opentelemetry.io/docs/collector/extend/ocb/)
and a small in-repo MQTT receiver component (see [`otelcol/components/mqttreceiver`](otelcol/components/mqttreceiver)).

## Installation

1. In Home Assistant, go to **Settings -> Add-ons -> Add-on Store**.
2. Open the top-right menu, choose **Repositories**, and add:

   ```
   https://github.com/jordan-simonovski/ha-otelcol
   ```

3. Install the **OpenTelemetry Collector** add-on. The first build compiles the
   collector from source on the device and may take several minutes.

## Configuration

All configuration is done from the add-on **Configuration** tab. Changes are
applied when you **Save** and **Restart** the add-on. See
[`otelcol/DOCS.md`](otelcol/DOCS.md) for the full option reference.

## Supported architectures

`amd64` and `aarch64`. The Home Assistant base images this add-on builds on are
not published for 32-bit targets at current versions.

## Versioning and changelog

Releases are managed with [changesets](https://github.com/changesets/changesets).
The add-on version lives in `otelcol/package.json` and is mirrored into
`otelcol/config.yaml` (what Home Assistant reads) by
[`scripts/sync-addon-version.mjs`](scripts/sync-addon-version.mjs). History is in
[`otelcol/CHANGELOG.md`](otelcol/CHANGELOG.md).

### CI (automated)

- `.github/workflows/ci.yml` runs on every PR/push: it requires a changeset on
  PRs (`changeset status`), runs the Go tests for the MQTT receiver, and builds
  the add-on image the same way Home Assistant does (so a broken Dockerfile or
  OCB manifest fails in CI, not on your device).
- `.github/workflows/release.yml` runs on pushes to `main`: the
  [changesets action](https://github.com/changesets/action) opens/updates a
  **Version Packages** PR that runs `npm run changeset:version`. Merging that PR
  is the release - it lands the version bump, the regenerated `CHANGELOG.md`, and
  the synced `config.yaml`.

> The release workflow needs **Settings -> Actions -> General -> Allow GitHub
> Actions to create and approve pull requests** enabled on the repo.

### Local (manual fallback)

```bash
npm install            # first time only
npm run changeset      # record a change (pick patch/minor/major)
npm run changeset:version   # bump version, update CHANGELOG.md, sync config.yaml
```

Then commit and push; bumping `config.yaml` is what makes Home Assistant offer
the update.

## License

Apache-2.0
