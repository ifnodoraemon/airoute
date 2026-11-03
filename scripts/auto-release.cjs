// Called only after the main-branch quality gate; APIs are injected for testing.
module.exports = async function autoRelease({ github, context, core, version, retry = false }) {
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)) throw new Error('Invalid release version');
  const repo = context.repo;
  const run = context.payload.workflow_run;
  if (run && (run.event !== 'push' || run.head_branch !== 'main' || run.conclusion !== 'success' || run.head_repository.full_name !== `${repo.owner}/${repo.repo}`)) {
    throw new Error('Release requires a successful trusted main-branch push');
  }
  const sha = run?.head_sha || context.sha;
  const branch = await github.rest.repos.getBranch({ ...repo, branch: 'main' });
  if (branch.data.commit.sha !== sha) { core.info('A newer main commit exists; skipping stale release'); return; }
  const checks = await github.rest.actions.listWorkflowRuns({ ...repo, workflow_id: 'ci.yml', head_sha: sha, branch: 'main', event: 'push', status: 'success', per_page: 10 });
  if (!checks.data.workflow_runs.some(check => check.head_sha === sha && check.conclusion === 'success' && check.head_repository.full_name === `${repo.owner}/${repo.repo}`)) throw new Error('This commit has no successful main quality gate');

  const tag = `v${version}`;
  let existing;
  try { existing = await github.rest.git.getRef({ ...repo, ref: `tags/${tag}` }); }
  catch (error) { if (error.status !== 404) throw error; }
  if (existing) {
    let object = existing.data.object;
    while (object.type === 'tag') object = (await github.rest.git.getTag({ ...repo, tag_sha: object.sha })).data.object;
    if (object.sha !== sha) {
      if (retry) throw new Error(`${tag} belongs to another commit. Increase VERSION for a new release.`);
      core.info(`${tag} is already published; keeping the immutable release`);
      return;
    }
    if (!retry) { core.info(`${tag} already exists for this commit`); return; }
  } else {
    const annotation = await github.rest.git.createTag({ ...repo, tag, message: `Release ${tag}`, object: sha, type: 'commit' });
    await github.rest.git.createRef({ ...repo, ref: `refs/tags/${tag}`, sha: annotation.data.sha });
  }

  // GITHUB_TOKEN tag creation does not emit a workflow-triggering push.
  // Dispatch explicitly so publication needs no extra PAT or App secret.
  const workflows = ['release.yml', 'docker.yml'];
  const results = await Promise.allSettled(workflows.map(workflow_id => github.rest.actions.createWorkflowDispatch({ ...repo, workflow_id, ref: tag })));
  const failed = results.flatMap((result, index) => result.status === 'rejected' ? [`${workflows[index]}: ${result.reason.message}`] : []);
  if (failed.length) throw new Error('Publication dispatch failed; retry this release: ' + failed.join('; '));
  core.info(`${tag} created/verified at ${sha}; binary and image publication dispatched`);
};
