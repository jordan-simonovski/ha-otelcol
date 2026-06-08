# AppArmor confinement + GitHub publishing pipeline

Date: 2026-06-08
Status: Approved

## Problem

The add-on ships a custom `otelcol/apparmor.txt` but `config.yaml` sets
`apparmor: false`, which disables it and costs a Home Assistant security point.
The repo has no GitHub workflows at all, yet `README.md` already documents a
`ci.yml` and `release.yml` that do not exist. There is no published image, so
every device compiles the collector from source on install.

## Goals

1. Enable the existing AppArmor profile properly.
2. Ship prebuilt multi-arch images so devices pull instead of compiling.
3. Make the README true: implement the documented `ci.yml` and `release.yml`.
4. Add a version-gated publish workflow on `main`.

## Non-goals

- armv7 / i386 (HA base images unavailable at current versions).
- Replacing changesets-based versioning.
- CHANGELOG automation beyond what changesets already provides.

## Design

### Add-on config (`otelcol/config.yaml`)

- `apparmor: false` -> `apparmor: true`. The Supervisor auto-loads
  `apparmor.txt`, validates exactly one profile name (file uses `otelcol`,
  matching the slug), and rewrites the name itself.
- Add `image: "ghcr.io/jordan-simonovski/otelcol"` (generic multi-arch
  manifest name; the `version:` field selects the tag). Devices now pull.

### `apparmor.txt`

Unchanged. Already matches the official S6 template with a single `otelcol`
profile, so it satisfies the Supervisor's one-profile-per-file rule.

### Workflows

All builder actions pinned to `home-assistant/builder/...@2026.03.2` (the
monolithic `home-assistant/builder@master` action is deprecated as of
2026.03.0).

**`.github/workflows/ci.yml`** — triggers: `pull_request`, `push` to `main`.

- `changeset` (PR only): `changeset status --since=origin/main` — fail if a PR
  carries no changeset.
- `test`: `go test ./...` in `otelcol/components/mqttreceiver` (its own module).
- `lint`: `frenck/action-addon-linter@v2` on `./otelcol`.
- `apparmor`: `apparmor_parser -Q otelcol/apparmor.txt` (syntax check, no
  kernel load).
- `build`: `build-image` for `amd64` only, `push:false cosign:false`, build
  context `otelcol/`, to verify the Dockerfile/OCB manifest still compile.

**`.github/workflows/release.yml`** — trigger: `push` to `main`.

- `changesets/action` with `version: npm run changeset:version`. Opens/updates
  the **Version Packages** PR. No publish step here.

**`.github/workflows/build.yml`** — trigger: `push` to `main`, paths
`otelcol/**`.

- `init`: read `version` from `config.yaml`; if tag `v<version>` already
  exists, set `should_publish=false` (no-op for doc/CI commits). Otherwise emit
  the build matrix via `prepare-multi-arch-matrix`.
- `build` (matrix `amd64`,`aarch64`; native runners): `build-image` with
  `push:true`, tags `<version>` and `latest`, `build-args`
  `BUILD_FROM` (read per-arch from `build.yaml`) and `BUILD_ARCH`. Cosign
  signing on (keyless, `id-token: write`).
- `manifest`: `publish-multi-arch-manifest` -> `ghcr.io/jordan-simonovski/otelcol`.
- `release`: create git tag `v<version>` and a GitHub Release.

### Single source of truth for `BUILD_FROM`

`build.yml` greps `otelcol/build.yaml` for the matrix arch line so the base
image version lives only in `build.yaml`.

## Data flow (release)

1. PR with a changeset merges to `main`.
2. `release.yml` opens/updates the Version Packages PR.
3. Merging that PR bumps `otelcol/package.json`, regenerates `CHANGELOG.md`,
   and syncs `otelcol/config.yaml` version.
4. The merge push to `main` hits `build.yml`; the new version has no tag, so it
   builds, pushes, tags `v<version>`, and releases.

## Operational notes

- The GHCR package must be set **public** once, or Supervisors cannot pull it.
- The `image:` cutover means the very first published version is the point
  where devices stop building locally. Documented in README.

## Risks

- `apparmor_parser -Q` depends on `tunables/global` and `abstractions/base`
  existing on the runner (they do via `apparmor` package). If parsing proves
  flaky, fall back to the addon-linter's apparmor check only.
- First-publish chicken-and-egg: `image:` is set before the first image exists.
  Mitigated by bumping version (which triggers publish) in the same change.
