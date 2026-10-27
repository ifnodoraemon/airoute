import { describe, it, expect, vi } from 'vitest';
import { executeHarnessTool, COPILOT_HARNESS_TOOLS, HARNESS_BENCHMARKS } from './copilotHarness';

// The copilot's tool surface. These functions are what an agent can actually do to
// the console, so their contract matters twice: the declarations have to be valid
// OpenAI function-calling schemas (or the model cannot call them at all), and the
// executors have to behave as described.

function context(over = {}) {
  return {
    channels: [],
    models: [],
    modelRoutes: [],
    logs: [],
    stats: {},
    keys: [],
    prices: [],
    isZh: true,
    ...over,
  };
}

describe('COPILOT_HARNESS_TOOLS', () => {
  it('declares each tool as a valid function schema', () => {
    expect(COPILOT_HARNESS_TOOLS.length).toBeGreaterThan(0);
    for (const tool of COPILOT_HARNESS_TOOLS) {
      expect(tool.type).toBe('function');
      expect(typeof tool.function.name).toBe('string');
      expect(tool.function.name).toMatch(/^[a-z][a-z0-9_]*$/);
      expect(typeof tool.function.description).toBe('string');
      expect(tool.function.description.length).toBeGreaterThan(10);
      expect(tool.function.parameters).toBeTypeOf('object');
      expect(tool.function.parameters.type).toBe('object');
    }
  });

  it('uses unique tool names', () => {
    const names = COPILOT_HARNESS_TOOLS.map((t) => t.function.name);
    expect(new Set(names).size).toBe(names.length);
  });

  it('names every tool the executor handles', () => {
    // A declared-but-unhandled tool fails at call time; a handled-but-undeclared one
    // is unreachable.
    const declared = COPILOT_HARNESS_TOOLS.map((t) => t.function.name).sort();
    expect(declared).toEqual([
      'audit_routing_spof',
      'diagnose_recent_errors',
      'generate_client_config',
      'get_cluster_telemetry',
      'navigate_console_view',
      'probe_upstream_channel',
    ]);
  });
});

describe('HARNESS_BENCHMARKS', () => {
  it('describes each benchmark with an id, a prompt and both labels', () => {
    expect(HARNESS_BENCHMARKS.length).toBeGreaterThan(0);
    for (const bm of HARNESS_BENCHMARKS) {
      expect(bm.id).toBeTruthy();
      expect(bm.prompt).toBeTruthy();
      expect(bm.label).toBeTruthy();
      expect(bm.enLabel).toBeTruthy();
      expect(bm.code).toBeTruthy();
    }
  });

  it('uses unique ids', () => {
    const ids = HARNESS_BENCHMARKS.map((b) => b.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

describe('executeHarnessTool', () => {
  describe('argument handling', () => {
    it('accepts an object', async () => {
      const out = await executeHarnessTool('get_cluster_telemetry', { include_channels: false }, context());

      expect(out.status).toBe('success');
      expect(out.args).toEqual({ include_channels: false });
    });

    it('accepts a JSON string, which is how the model usually sends them', async () => {
      const out = await executeHarnessTool('get_cluster_telemetry', '{"include_channels":false}', context());

      expect(out.status).toBe('success');
      expect(out.args).toEqual({ include_channels: false });
    });

    it('treats malformed JSON as no arguments rather than failing', async () => {
      const out = await executeHarnessTool('get_cluster_telemetry', '{not json', context());

      expect(out.status).toBe('success');
      expect(out.args).toEqual({});
    });

    it('treats a non-object argument as no arguments', async () => {
      const out = await executeHarnessTool('get_cluster_telemetry', 42, context());

      expect(out.status).toBe('success');
      expect(out.args).toEqual({});
    });

    it('reports an unknown tool as an error result instead of throwing', async () => {
      const out = await executeHarnessTool('no_such_tool', {}, context());

      expect(out.status).toBe('error');
      expect(out.error).toMatch(/no_such_tool/);
    });
  });

  describe('get_cluster_telemetry', () => {
    it('counts the fleet and scores its health', async () => {
      const out = await executeHarnessTool('get_cluster_telemetry', {}, context({
        channels: [
          { id: 1, name: 'a', status: 'active', breaker_status: 'closed' },
          { id: 2, name: 'b', status: 'active', breaker_status: 'open' },
          { id: 3, name: 'c', status: 'disabled', breaker_status: 'closed' },
        ],
        models: ['gpt-6'],
        modelRoutes: [{ model: 'gpt-6' }],
      }));

      expect(out.status).toBe('success');
      const summary = out.result.cluster_summary;
      expect(summary.total_channels).toBe(3);
      expect(summary.active_channels).toBe(2);
      expect(summary.tripped_channels).toBe(1);
      // Two of three channels are not tripped.
      expect(summary.health_score).toBe(67);
      expect(summary.mounted_models_count).toBe(1);
      expect(summary.active_routes_count).toBe(1);
    });

    it('reports a perfect score for an empty fleet rather than dividing by zero', async () => {
      const out = await executeHarnessTool('get_cluster_telemetry', {}, context());

      expect(out.result.cluster_summary.health_score).toBe(100);
      expect(Number.isFinite(out.result.cluster_summary.health_score)).toBe(true);
    });

    it('includes the channel list by default', async () => {
      const out = await executeHarnessTool('get_cluster_telemetry', {}, context({
        channels: [{ id: 1, name: 'a', status: 'active', models: ['gpt-6'] }],
      }));

      expect(out.result.channels).toHaveLength(1);
      expect(out.result.channels[0].name).toBe('a');
    });

    it('omits the channel list when asked not to include it', async () => {
      const out = await executeHarnessTool('get_cluster_telemetry', { include_channels: false }, context({
        channels: [{ id: 1, name: 'a', status: 'active' }],
      }));

      expect(out.result.channels).toBeUndefined();
    });

    it('reports the performance metrics it was given', async () => {
      const out = await executeHarnessTool('get_cluster_telemetry', {}, context({
        stats: { total_requests: 500, avg_ttft_ms: 88, cache_hit_rate: '42%', total_cost: 3.5 },
      }));

      const perf = out.result.performance_metrics;
      expect(perf.total_requests).toBe(500);
      expect(perf.avg_ttft_ms).toBe(88);
      expect(perf.cache_hit_rate).toBe('42%');
      expect(perf.total_cost_usd).toBe(3.5);
    });
  });

  describe('diagnose_recent_errors', () => {
    it('reports when there is nothing to diagnose', async () => {
      const out = await executeHarnessTool('diagnose_recent_errors', {}, context());

      expect(out.status).toBe('success');
      expect(out.result).toBeTruthy();
    });

    it('aggregates the failed rows it was given', async () => {
      const out = await executeHarnessTool('diagnose_recent_errors', {}, context({
        logs: [
          { status_code: 429, model: 'gpt-6', error_message: 'rate limited' },
          { status_code: 429, model: 'gpt-6', error_message: 'rate limited' },
          { status_code: 503, model: 'claude-5', error_message: 'upstream down' },
          { status_code: 200, model: 'gpt-6' },
        ],
      }));

      expect(out.status).toBe('success');
      // The healthy row must not appear in a diagnosis.
      const text = JSON.stringify(out.result);
      expect(text).toContain('429');
      expect(text).toContain('503');
    });
  });

  describe('audit_routing_spof', () => {
    it('flags a model served by a single channel', async () => {
      // One channel for a model is the single point of failure the audit exists for.
      const out = await executeHarnessTool('audit_routing_spof', {}, context({
        channels: [{ id: 1, name: 'only-one', status: 'active', models: ['gpt-6'] }],
        modelRoutes: [{ model: 'gpt-6', providers: [{ channel_id: 1 }] }],
      }));

      expect(out.status).toBe('success');
      expect(JSON.stringify(out.result)).toContain('gpt-6');
    });

    it('runs on an empty configuration', async () => {
      const out = await executeHarnessTool('audit_routing_spof', {}, context());

      expect(out.status).toBe('success');
    });
  });

  describe('navigate_console_view', () => {
    it('switches the tab and the view', async () => {
      const setCurrentTab = vi.fn();
      const setViewMode = vi.fn();

      const out = await executeHarnessTool('navigate_console_view', { target_tab: 'logs' }, context({
        setCurrentTab, setViewMode,
      }));

      expect(out.status).toBe('success');
      expect(setCurrentTab).toHaveBeenCalledWith('logs');
      expect(setViewMode).toHaveBeenCalledWith('console');
      expect(out.result.navigated_to).toBe('logs');
    });

    it('does not navigate without a target', async () => {
      const setCurrentTab = vi.fn();

      const out = await executeHarnessTool('navigate_console_view', {}, context({ setCurrentTab }));

      expect(out.status).toBe('success');
      expect(setCurrentTab).not.toHaveBeenCalled();
    });

    it('does not fail when the console gave it no navigation callbacks', async () => {
      const out = await executeHarnessTool('navigate_console_view', { target_tab: 'logs' }, context());

      expect(out.status).toBe('success');
    });
  });

  describe('generate_client_config', () => {
    it('generates every client when no type is given', async () => {
      const out = await executeHarnessTool('generate_client_config', {}, context());

      expect(out.status).toBe('success');
      // Order is not part of the contract; the set of clients is.
      expect(Object.keys(out.result).sort()).toEqual(['claude_code', 'curl', 'cursor', 'python']);
    });

    it('generates only the requested client', async () => {
      const out = await executeHarnessTool('generate_client_config', { client_type: 'python' }, context());

      expect(Object.keys(out.result)).toEqual(['python']);
    });

    it('uses a placeholder instead of a listed key', async () => {
      // The list only carries display masks now (credentials are stored as
      // digests), so a generated config must not paste one into the snippet.
      const out = await executeHarnessTool('generate_client_config', { client_type: 'cursor' }, context({
        keys: [{ key: 'sk-disabled', status: 'disabled' }, { key: 'sk-active', status: 'active' }],
      }));

      expect(out.result.cursor.api_key).toBe('<YOUR_API_KEY>');
    });

    it('never embeds a display mask in any snippet', async () => {
      const masked = '3a4b5c6d***9f8e';
      const out = await executeHarnessTool('generate_client_config', { client_type: 'all' }, context({
        keys: [{ id: 1, key: masked, key_fingerprint: 'kfp_1', status: 'active' }],
        baseUrl: 'https://gw.example/v1',
        origin: 'https://gw.example',
      }));

      const serialized = JSON.stringify(out.result);
      expect(serialized).not.toContain('***');
      expect(serialized).toContain('<YOUR_API_KEY>');
      expect(out.result.curl.command).toContain('Authorization: Bearer <YOUR_API_KEY>');
      expect(out.result.claude_code.bash_env).toContain('ANTHROPIC_API_KEY="<YOUR_API_KEY>"');
      expect(out.result.python.code_snippet).toContain('api_key="<YOUR_API_KEY>"');
    });

    it('falls back to a placeholder when no key exists', async () => {
      // A snippet with an empty credential would look usable and fail on first call.
      const out = await executeHarnessTool('generate_client_config', { client_type: 'cursor' }, context());

      expect(out.result.cursor.api_key).toBeTruthy();
      expect(out.result.cursor.api_key).not.toBe('');
    });

    it('honours the preferred model', async () => {
      const out = await executeHarnessTool('generate_client_config', { client_type: 'cursor', preferred_model: 'my-model' }, context());

      expect(out.result.cursor.model).toBe('my-model');
    });

    it('falls back to the first available model', async () => {
      const out = await executeHarnessTool('generate_client_config', { client_type: 'cursor' }, context({
        models: ['first-model', 'second-model'],
      }));

      expect(out.result.cursor.model).toBe('first-model');
    });

    it('embeds the gateway base URL in the snippets', async () => {
      const out = await executeHarnessTool('generate_client_config', { client_type: 'all' }, context({
        baseUrl: 'https://gw.example/v1',
        origin: 'https://gw.example',
      }));

      expect(out.result.cursor.base_url).toBe('https://gw.example/v1');
      expect(out.result.python.code_snippet).toContain('https://gw.example/v1');
      expect(out.result.curl.command).toContain('https://gw.example/v1');
      expect(out.result.claude_code.bash_env).toContain('https://gw.example');
    });
  });

  it('reports how long each call took', async () => {
    const out = await executeHarnessTool('get_cluster_telemetry', {}, context());

    expect(typeof out.duration_ms).toBe('number');
    expect(out.duration_ms).toBeGreaterThanOrEqual(0);
  });

  it('echoes the tool name and arguments back', async () => {
    const out = await executeHarnessTool('get_cluster_telemetry', { include_channels: false }, context());

    expect(out.tool).toBe('get_cluster_telemetry');
    expect(out.args).toEqual({ include_channels: false });
  });
});
