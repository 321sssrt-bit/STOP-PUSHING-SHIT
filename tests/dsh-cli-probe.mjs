// Mount through a DSH --patch overlay after installing the bundle in an isolated home.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { randomUUID } from 'node:crypto';
import { writeFileSync } from 'node:fs';

export const name = 'sps-cli-probe';
export const inject = ['tools', 'agents', 'agentLoop', 'llm'];

export async function apply(ctx) {
  try {
    const requireDsh = createRequire(join(process.env.SPS_DSH_ROOT, 'package.json'));
    const { LlmAdapter, createUserMessage } = await import(pathToFileURL(requireDsh.resolve('@deepseek-ai/dsh-llm')));
    let pending = false;
    let executed = 0;
    const tool = 'mcp_sps_fixture_create_issue';
    const args = { owner: 'fixture', repo: 'repo', title: 'CLI local test', body: 'fixture' };
    class FixtureAdapter extends LlmAdapter {
      async *stream() {
        if (pending) {
          pending = false;
          const id = `call-${randomUUID()}`;
          yield { type: 'block-start', index: 0, blockType: 'tool-call' };
          yield { type: 'tool-call-delta', index: 0, id, name: tool, argumentsDelta: JSON.stringify(args) };
          yield { type: 'block-end', index: 0, block: { type: 'tool-call', id, name: tool, arguments: args } };
          yield { type: 'finish', reason: 'tool_calls' };
        } else {
          yield { type: 'block-start', index: 0, blockType: 'text' };
          yield { type: 'text-delta', index: 0, text: 'done' };
          yield { type: 'block-end', index: 0, block: { type: 'text', text: 'done' } };
          yield { type: 'finish', reason: 'stop' };
        }
      }
    }
    ctx.llm.registerAdapter(['sps-fixture'], new FixtureAdapter());
    ctx.tools.register({
      name: tool, description: 'Local-only CLI installation probe.',
      parameters: { type: 'object', properties: {} },
      output: { schema: { type: 'object' }, render: () => [{ type: 'text', text: 'local fixture' }] },
      execute: () => {
        executed++;
        return {};
      },
    });
    const handle = await ctx.agents.create({
      sessionId: `sps-cli-${randomUUID()}`,
      meta: { cwd: process.env.SPS_DSH_WORKDIR },
      agentOptions: { provider: 'sps-fixture', model: 'fixture' },
    });
    const results = [];
    for (const prompt of ['prepare', '授权提交', 'continue']) {
      pending = true;
      handle.agent.followup(createUserMessage({ content: [{ type: 'text', text: prompt }], source: { kind: 'user' } }));
      await handle.agent.whenIdle();
      assert.equal(pending, false);
      results.push(executed);
    }
    assert.deepEqual(results, [0, 1, 1]);
    await handle.dispose();
    writeFileSync(process.env.SPS_DSH_CLI_RESULT, JSON.stringify({ status: 'passed', executionCounts: results }) + '\n');
    console.log('SPS_DSH_CLI_PROBE_PASSED');
    process.exit(0);
  } catch (error) {
    console.error(error);
    process.exit(1);
  }
}
