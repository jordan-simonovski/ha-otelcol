# Changesets

This folder is managed by [changesets](https://github.com/changesets/changesets).
It tracks pending version changes for the OpenTelemetry Collector add-on.

## Workflow

1. After making a change worth releasing, record it:

   ```bash
   npm run changeset
   ```

   Pick the bump type (`patch` / `minor` / `major`) for `otelcol-addon` and write
   a short, user-facing summary. This creates a markdown file in `.changeset/`.

2. When you are ready to cut a release, apply the pending changesets:

   ```bash
   npm run changeset:version
   ```

   This bumps `otelcol/package.json`, updates `otelcol/CHANGELOG.md`, and syncs
   the new version into `otelcol/config.yaml` (which is what Home Assistant reads).

3. Commit the result and push. Bumping the `version:` in `config.yaml` is what
   makes Home Assistant offer the add-on update.
