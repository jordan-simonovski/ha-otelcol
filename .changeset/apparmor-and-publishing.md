---
"otelcol-addon": minor
---

Enable the bundled AppArmor profile (`apparmor: true`) to confine the collector,
and ship prebuilt multi-arch images from GHCR (`image:`) so installs download
instead of compiling on the device. Adds CI (changeset/test/lint/apparmor/build)
and a version-gated multi-arch publish workflow.
