const { test } = require('node:test');
const assert = require('node:assert/strict');
const { mkdtempSync, mkdirSync, writeFileSync, copyFileSync, rmSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join, resolve } = require('node:path');
const { spawnSync } = require('node:child_process');

const checker = resolve(__dirname, 'check-functions.go');
const go = process.env.GO || 'go';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'airoute-size-'));
  for (const dir of ['internal', 'cmd', 'scripts']) mkdirSync(join(root, dir));
  writeFileSync(join(root, 'scripts/function-size-baseline.txt'), '');
  writeFileSync(join(root, 'scripts/size-baseline.txt'), '');
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}

function businessFunction(comments) {
  return 'package fixture\ntype Worker struct{}\nfunc (w *Worker) Execute() {\n' + ' // work\n'.repeat(comments) + '}\n';
}

test('new oversized functions fail; existing functions can shrink but cannot grow', t => {
  const root = fixture(t);
  const source = join(root, 'internal/work.go');
  writeFileSync(source, businessFunction(59));
  const run = () => spawnSync(go, ['run', checker, '-root', root], { encoding: 'utf8' });
  let result = run();
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /internal\/work.go:Worker.Execute \(61 lines/);
  writeFileSync(join(root, 'scripts/function-size-baseline.txt'), 'internal/work.go:Worker.Execute 61\n');
  assert.equal(run().status, 0);
  writeFileSync(source, businessFunction(60));
  assert.notEqual(run().status, 0);
  writeFileSync(source, businessFunction(58));
  assert.equal(run().status, 0);
});

test('the shell gate rejects a new oversized file and ratchets an existing file', t => {
  const root = fixture(t);
  copyFileSync(join(__dirname, 'check-size.sh'), join(root, 'scripts/check-size.sh'));
  copyFileSync(checker, join(root, 'scripts/check-functions.go'));
  const source = join(root, 'internal/declarations.go');
  writeFileSync(source, 'package fixture\n' + '// declaration\n'.repeat(300));
  const run = () => spawnSync('sh', ['scripts/check-size.sh'], { cwd: root, encoding: 'utf8' });
  assert.notEqual(run().status, 0);
  writeFileSync(join(root, 'scripts/size-baseline.txt'), 'internal/declarations.go 301\n');
  assert.equal(run().status, 0);
  writeFileSync(source, 'package fixture\n' + '// declaration\n'.repeat(301));
  assert.notEqual(run().status, 0);
});
