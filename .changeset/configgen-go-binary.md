---
"otelcol-addon": minor
---

Generate the collector config with a shipped `otelcol-config` Go binary instead
of the bash run script. Config generation is now unit-tested against golden
files with no Docker, and the run script no longer hand-rolls YAML quoting.
