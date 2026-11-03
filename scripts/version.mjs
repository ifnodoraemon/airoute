#!/usr/bin/env node
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = name => readFileSync(path.join(root, name), 'utf8');
const stable = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const current = read('VERSION').trim();
const requested = process.argv[2] || '--check';
const checking = requested === '--check';
const version = checking ? current : requested;
if (!stable.test(version)) throw new Error('Expected a stable MAJOR.MINOR.PATCH version');
if (!checking) {
  const before = current.split('.').map(Number), after = version.split('.').map(Number);
  const differing = after.findIndex((part, index) => part !== before[index]);
  if (differing >= 0 && after[differing] < before[differing]) throw new Error('Versions cannot move backwards');
}

const replacements = new Map();
const replace = (name, pattern, value) => {
  const original = replacements.get(name) || read(name);
  if (!pattern.test(original)) throw new Error(`Cannot find version metadata in ${name}`);
  replacements.set(name, original.replace(pattern, value));
};
replacements.set('VERSION', version + '\n');
replace('internal/buildinfo/version.go', /const BaseVersion = "[^"]+"/, `const BaseVersion = "${version}"`);
for (const name of ['web/package.json', 'web/package-lock.json']) {
  const data = JSON.parse(read(name));
  data.version = version;
  if (data.packages?.['']) data.packages[''].version = version;
  replacements.set(name, JSON.stringify(data, null, 2) + '\n');
}
replace('Dockerfile', /^ARG VERSION=.+$/m, `ARG VERSION=${version}-dev`);
replace('docker-compose.yml', /VERSION: \$\{VERSION:-[^}]+\}/, `VERSION: \${VERSION:-${version}-dev}`);
replace('helm/airoute/Chart.yaml', /^version: .+$/m, `version: ${version}`);
replace('helm/airoute/Chart.yaml', /^appVersion: .+$/m, `appVersion: "${version}"`);

const drift = [...replacements].filter(([name, text]) => read(name) !== text);
if (checking && drift.length) throw new Error('Version metadata differs from VERSION: ' + drift.map(([name]) => name).join(', ') + '. Run node scripts/version.mjs ' + current);
if (!checking) for (const [name, text] of replacements) writeFileSync(path.join(root, name), text);
console.log(checking ? `Version metadata agrees: ${version}` : `Version metadata updated: ${version}`);
