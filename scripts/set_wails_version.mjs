#!/usr/bin/env node

import fs from 'node:fs';

const usage = 'Usage: node scripts/set_wails_version.mjs (--check|--write) [version]';
const [mode, rawVersion] = process.argv.slice(2);
const requestedVersion = rawVersion || process.env.RELEASE_VERSION || process.env.GITHUB_REF_NAME;

if (!['--check', '--write'].includes(mode) || !requestedVersion) {
  console.error(usage);
  process.exit(2);
}

const version = requestedVersion.replace(/^v/, '');
if (!/^\d+\.\d+\.\d+$/.test(version)) {
  console.error(`Invalid version "${requestedVersion}". Expected vX.Y.Z or X.Y.Z.`);
  process.exit(2);
}

const files = ['wails.json', 'cmd/dashboard/wails.json'];
let failed = false;

for (const file of files) {
  const json = JSON.parse(fs.readFileSync(file, 'utf8'));
  json.info ??= {};
  const current = json.info.productVersion;

  if (mode === '--check') {
    if (current !== version) {
      console.error(`${file}: productVersion is ${current ?? '<missing>'}, expected ${version}`);
      failed = true;
    }
    continue;
  }

  json.info.productVersion = version;
  fs.writeFileSync(file, `${JSON.stringify(json, null, 2)}\n`);
  console.log(`${file}: productVersion=${version}`);
}

if (failed) {
  process.exit(1);
}
