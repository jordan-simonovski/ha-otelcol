---
"otelcol-addon": minor
---

Ship the otelcol processors config fix that was merged but never published. The
`v0.2.0` image was tagged from an out-of-band version edit before the fix landed,
so the build pipeline skipped it. Cutting a fresh, untagged version releases it.
