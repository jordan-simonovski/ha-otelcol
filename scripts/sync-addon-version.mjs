#!/usr/bin/env node
// Copies the version from otelcol/package.json (managed by changesets) into
// otelcol/config.yaml, which is the version Home Assistant actually reads.
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const pkgPath = join(root, 'otelcol', 'package.json');
const cfgPath = join(root, 'otelcol', 'config.yaml');

const { version } = JSON.parse(readFileSync(pkgPath, 'utf8'));
if (!version) {
  console.error('sync-addon-version: no version found in otelcol/package.json');
  process.exit(1);
}

const cfg = readFileSync(cfgPath, 'utf8');
const versionLine = /^version:.*$/m;
if (!versionLine.test(cfg)) {
  console.error('sync-addon-version: no "version:" line found in otelcol/config.yaml');
  process.exit(1);
}

const updated = cfg.replace(versionLine, `version: ${version}`);
writeFileSync(cfgPath, updated);
console.log(`sync-addon-version: otelcol/config.yaml version -> ${version}`);
