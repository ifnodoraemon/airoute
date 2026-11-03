const { test } = require('node:test');
const assert = require('node:assert/strict');
const publish = require('./auto-release.cjs');

function fixture({ existing, main = 'head', gate = true, dispatchFailure = false } = {}) {
  const calls = [];
  const context = { repo: { owner: 'owner', repo: 'airoute' }, sha: 'head', payload: {} };
  const github = { rest: {
    repos: { getBranch: async () => ({ data: { commit: { sha: main } } }) },
    actions: {
      listWorkflowRuns: async () => ({ data: { workflow_runs: gate ? [{ head_sha: 'head', conclusion: 'success', head_repository: { full_name: 'owner/airoute' } }] : [] } }),
      createWorkflowDispatch: async args => { calls.push(['dispatch', args]); if (dispatchFailure && args.workflow_id === 'docker.yml') throw new Error('denied'); },
    },
    git: {
      getRef: async () => { if (!existing) throw Object.assign(new Error('missing'), { status: 404 }); return { data: { object: { type: 'tag', sha: 'annotation' } } }; },
      getTag: async () => ({ data: { object: { type: 'commit', sha: existing } } }),
      createTag: async args => { calls.push(['tag', args]); return { data: { sha: 'annotation' } }; },
      createRef: async args => { calls.push(['ref', args]); },
    },
  } };
  return { input: { github, context, core: { info() {} }, version: '2.0.0' }, calls };
}

test('creates an annotated tag at the tested SHA and dispatches both publishers', async () => {
  const { input, calls } = fixture();
  await publish(input);
  assert.equal(calls.find(([kind]) => kind === 'tag')[1].object, 'head');
  assert.equal(calls.find(([kind]) => kind === 'ref')[1].ref, 'refs/tags/v2.0.0');
  assert.deepEqual(calls.filter(([kind]) => kind === 'dispatch').map(([, args]) => [args.workflow_id, args.ref]).sort(), [['docker.yml', 'v2.0.0'], ['release.yml', 'v2.0.0']]);
});

test('never publishes without a successful CI gate', async () => {
  const { input, calls } = fixture({ gate: false });
  await assert.rejects(publish(input), /quality gate/);
  assert.equal(calls.length, 0);
});

test('skips a completed CI run overtaken by a newer main commit', async () => {
  const { input, calls } = fixture({ main: 'newer' });
  await publish(input);
  assert.equal(calls.length, 0);
});

test('rejects untrusted workflow_run payloads', async () => {
  const { input, calls } = fixture();
  input.context.payload.workflow_run = { event: 'pull_request', head_branch: 'main', conclusion: 'success', head_repository: { full_name: 'fork/airoute' } };
  await assert.rejects(publish(input), /trusted/);
  assert.equal(calls.length, 0);
});

test('keeps published tags immutable and does not publish twice', async () => {
  for (const existing of ['old-release', 'head']) {
    const { input, calls } = fixture({ existing });
    await publish(input);
    assert.equal(calls.length, 0);
  }
});

test('explicit retry only dispatches publishers for the exact existing tag SHA', async () => {
  const { input, calls } = fixture({ existing: 'head' });
  await publish({ ...input, retry: true });
  assert.deepEqual(calls.map(([kind]) => kind), ['dispatch', 'dispatch']);
  await assert.rejects(publish({ ...fixture({ existing: 'old' }).input, retry: true }), /another commit/);
});

test('reports partial dispatch failure so it can be retried', async () => {
  const { input, calls } = fixture({ dispatchFailure: true });
  await assert.rejects(publish(input), /docker.yml: denied/);
  assert.equal(calls.filter(([kind]) => kind === 'dispatch').length, 2);
});
