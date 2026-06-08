# otelcol-addon

## 0.1.1

### Patch Changes

- Fix the add-on image build on Home Assistant. `BUILD_FROM` is now declared as a
  global Docker `ARG` before the first `FROM`, so the runtime stage correctly
  resolves the Home Assistant base image instead of failing with
  `base name (${BUILD_FROM}) should not be blank`.
