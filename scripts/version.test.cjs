const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

test('version changes update every consumer and drift fails without modifying files', t => {
  const root = path.resolve(__dirname, '..');
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'airoute-version-'));
  t.after(() => fs.rmSync(temporary, { recursive: true, force: true }));
  const files = ['VERSION', 'scripts/version.mjs', 'internal/buildinfo/version.go', 'web/package.json', 'web/package-lock.json', 'Dockerfile', 'docker-compose.yml', 'helm/airoute/Chart.yaml'];
  for (const name of files) {
    fs.mkdirSync(path.dirname(path.join(temporary, name)), { recursive: true });
    fs.copyFileSync(path.join(root, name), path.join(temporary, name));
  }
  const current=fs.readFileSync(path.join(root,'VERSION'),'utf8').trim();
  const [major,minor]=current.split('.').map(Number);
  const next=`${major}.${minor+1}.0`;
  const run = value => spawnSync(process.execPath, [path.join(temporary, 'scripts/version.mjs'), value], { encoding: 'utf8' });
  const succeeds=value=>{const result=run(value);assert.equal(result.status,0,result.stderr||result.stdout);};
  succeeds('--check');
  succeeds(next);
  succeeds('--check');
  assert.equal(JSON.parse(fs.readFileSync(path.join(temporary, 'web/package-lock.json'))).packages[''].version, next);
  assert.equal(fs.readFileSync(path.join(temporary,'helm/airoute/Chart.yaml'),'utf8').match(/^appVersion: .+$/m)[0],`appVersion: "${next}"`);
  assert.notEqual(run(current).status, 0);
  assert.notEqual(run('v3.0.0').status, 0);
  fs.writeFileSync(path.join(temporary, 'helm/airoute/Chart.yaml'), fs.readFileSync(path.join(temporary, 'helm/airoute/Chart.yaml'), 'utf8').replace(`appVersion: "${next}"`, 'appVersion: "stale"'));
  const before = fs.readFileSync(path.join(temporary, 'helm/airoute/Chart.yaml'), 'utf8');
  assert.notEqual(run('--check').status, 0);
  assert.equal(fs.readFileSync(path.join(temporary, 'helm/airoute/Chart.yaml'), 'utf8'), before);
});
