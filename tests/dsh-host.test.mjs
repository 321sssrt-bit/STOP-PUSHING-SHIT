import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createRequire } from 'node:module';
import { mkdtempSync, appendFileSync, readFileSync, existsSync, writeFileSync, mkdirSync, symlinkSync, unlinkSync, rmdirSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { randomUUID } from 'node:crypto';
import * as sourcePlugin from '../plugins/dsh/index.js';

const dshRoot = process.env.SPS_DSH_ROOT;
const binary = process.env.SPS_DSH_BINARY;
if (!dshRoot || !binary) throw new Error('Set SPS_DSH_ROOT and SPS_DSH_BINARY to the installed DSH and freshly built guard');
const requireDsh = createRequire(join(resolve(dshRoot), 'package.json'));
const load = (name) => import(pathToFileURL(requireDsh.resolve(`@deepseek-ai/${name}`)));
const { Context } = await load('cordis');
const { LlmAdapter, createUserMessage } = await load('dsh-llm');
const plugin = process.env.SPS_DSH_PLUGIN
  ? await import(pathToFileURL(resolve(process.env.SPS_DSH_PLUGIN))) : sourcePlugin;
const packages = await Promise.all([
  'dsh-session', 'dsh-session-projection', 'dsh-agent', 'dsh-system-prompt',
  'dsh-llm', 'dsh-tools', 'dsh-agent-loop',
].map(load));

async function fixture(t, options = {}) {
  const dir = mkdtempSync(join(tmpdir(), process.platform === 'win32' ? 'sps-dsh-中文 空格-' : 'sps-dsh-host-'));
  const marker = join(dir, 'executed.jsonl');
  const ctx = new Context();
  const errors = [];
  ctx.on('agent/error', ({ error }) => errors.push(String(error)));
  for (const mod of packages) await ctx.plugin(mod.default);
  const plans = new Map();
  class ScriptedAdapter extends LlmAdapter {
    async *stream(options) {
      const plan = plans.get(options.messages.findLast(m => m.source?.kind === 'user')?.id)
        ?? plans.get('next');
      if (plan?.length) {
        const calls = plan.shift();
        for (const [index, call] of calls.entries()) {
          const id = `call-${randomUUID()}`;
          yield { type: 'block-start', index, blockType: 'tool-call' };
          yield { type: 'tool-call-delta', index, id, name: call.name, argumentsDelta: JSON.stringify(call.args) };
          yield { type: 'block-end', index, block: { type: 'tool-call', id, name: call.name, arguments: call.args } };
        }
        yield { type: 'finish', reason: 'tool_calls' };
      } else {
        yield { type: 'block-start', index: 0, blockType: 'text' };
        yield { type: 'text-delta', index: 0, text: 'done' };
        yield { type: 'block-end', index: 0, block: { type: 'text', text: 'done' } };
        yield { type: 'finish', reason: 'stop' };
      }
    }
  }
  ctx.llm.registerAdapter(['fixture'], new ScriptedAdapter());
  const tools = new Map();
  for (const name of ['mcp_github_create_issue', 'mcp_github_create_repository', 'bash', 'pwsh', 'terminal_send', 'read_fixture']) {
    tools.set(name, ctx.tools.register({
      name, description: 'Local fixture; writes only a temporary execution marker.',
      parameters: { type: 'object', properties: { command: { type: 'string' }, workdir: { type: 'string' } } },
      output: { schema: { type: 'object' }, render: () => [{ type: 'text', text: 'fixture completed' }] },
      execute: async (args) => {
        appendFileSync(marker, JSON.stringify({ name, args }) + '\n');
        if (args.body === 'fail') throw new Error('intentional fixture failure');
        return {};
      },
    }));
  }
  const guard = await ctx.plugin(plugin, {
    ...(process.env.SPS_DSH_PLUGIN ? {} : { binary }), dataDir: join(dir, 'state'), ...options,
  });
  t.after(() => ctx.fiber.dispose());
  const handles = [];
  async function agent(parentAgent) {
    const handle = await ctx.agents.create({
      sessionId: `sps-${randomUUID()}`, parentAgent,
      meta: { cwd: dir, ...(parentAgent ? { origin: 'subagent' } : {}) },
      agentOptions: { provider: 'fixture', model: 'fixture' },
    });
    handles.push(handle);
    return handle.agent;
  }
  async function send(agent, text, calls, source = { kind: 'user' }) {
    const message = createUserMessage({ content: [{ type: 'text', text }], source });
    plans.set('next', [calls]);
    agent.followup(message);
    await agent.whenIdle();
    assert.deepEqual(errors, [], 'agent loop errors');
    assert.equal(plans.get('next').length, 0, 'scripted tool call reached the real loop');
    if (process.env.SPS_DSH_DEBUG) console.error(JSON.stringify(agent.session.snapshotEvents().filter(e =>
      ['turn/start', 'user/message', 'tool/result'].includes(e.type)).slice(-6), null, 2));
    return agent.session.snapshotEvents().filter(e => e.type === 'tool/result').at(-1);
  }
  const count = () => existsSync(marker) ? readFileSync(marker, 'utf8').trim().split('\n').length : 0;
  return { ctx, dir, marker, guard, agent, send, count, plans, handles, tools };
}

const issue = (title = 'fixture', body = 'body') => ({
  name: 'mcp_github_create_issue', args: { owner: 'fixture', repo: 'repo', title, body },
});

test('real DSH loop blocks, binds full arguments, consumes one authorization', async (t) => {
  const f = await fixture(t);
  const a = await f.agent();
  await f.send(a, 'create issue', [issue()]);
  assert.equal(f.count(), 0);
  await f.send(a, '授权提交', [issue(), issue()]);
  assert.equal(f.count(), 1);
  await f.send(a, '授权提交', [issue('changed')]);
  assert.equal(f.count(), 1);
  await f.send(a, '授权提交', [issue('changed')]);
  assert.equal(f.count(), 2);
});

test('failure still consumes authorization; unrelated prompts revoke it', async (t) => {
  const f = await fixture(t);
  const a = await f.agent();
  await f.send(a, 'create', [issue('failure', 'fail')]);
  await f.send(a, '授权提交', [issue('failure', 'fail'), issue('failure', 'fail')]);
  assert.equal(f.count(), 1);
  await f.send(a, 'continue', [issue('failure', 'fail')]);
  assert.equal(f.count(), 1);
});

test('automation, other sessions and subagents cannot borrow authorization', async (t) => {
  const f = await fixture(t);
  const a = await f.agent();
  const b = await f.agent();
  const child = await f.agent(a);
  await f.send(a, 'create', [issue()]);
  await f.send(a, '授权提交', [issue()], { kind: 'fixture-automation' });
  await f.send(b, '授权提交', [issue()]);
  await f.send(child, 'create', [issue()]);
  await f.send(child, '授权提交', [issue()]);
  assert.equal(f.count(), 0);
  await f.send(a, '授权提交', [issue()]);
  assert.equal(f.count(), 1);
});

test('shell and terminal bypasses are rejected; ordinary reads execute', async (t) => {
  const f = await fixture(t);
  const a = await f.agent();
  for (const name of ['bash', 'pwsh']) {
    await f.send(a, 'create', [{ name, args: { command: 'gh issue create --title bypass --body bypass' } }]);
  }
  await f.send(a, 'terminal', [{ name: 'terminal_send', args: { text: 'gh issue create\n' } }]);
  assert.equal(f.count(), 0);
  await f.send(a, 'read', [{ name: 'read_fixture', args: {} }]);
  assert.equal(f.count(), 1);
});

test('missing guard executable denies even when another pre-tool listener allows', async (t) => {
  const f = await fixture(t, { binary: join(tmpdir(), `missing-${randomUUID()}`) });
  f.ctx.on('tools/pre-execute', async () => ({ kind: 'allow' }));
  const a = await f.agent();
  await f.send(a, 'create', [issue()]);
  await f.send(a, '授权提交', [issue()]);
  assert.equal(f.count(), 0);
});

test('repository creation needs explicit visibility and binds the entire request', async (t) => {
  const f = await fixture(t);
  const a = await f.agent();
  const create = (args) => ({ name: 'mcp_github_create_repository', args });
  await f.send(a, 'create', [create({ name: 'fixture' })]);
  await f.send(a, '授权提交', [create({ name: 'fixture' })]);
  assert.equal(f.count(), 0);
  await f.send(a, 'create', [create({ name: 'fixture', private: true })]);
  await f.send(a, '授权提交', [create({ name: 'fixture', private: false })]);
  assert.equal(f.count(), 0);
  await f.send(a, '授权提交', [create({ name: 'fixture', private: false })]);
  assert.equal(f.count(), 1);
});

test('authorization expires on turn end and plugin reload', async (t) => {
  const f = await fixture(t);
  const a = await f.agent();
  await f.send(a, 'create', [issue()]);
  await f.send(a, '授权提交', []);
  await f.send(a, 'continue', [issue()]);
  assert.equal(f.count(), 0);
  await f.guard.restart();
  await f.send(a, 'continue', [issue()]);
  assert.equal(f.count(), 0);
  await f.send(a, '授权提交', [issue()]);
  assert.equal(f.count(), 1);
});

test('persistent shell commands are explicitly denied', async (t) => {
  const f = await fixture(t);
  f.tools.get('bash')();
  f.ctx.tools.register({
    name: 'bash', description: 'Persistent fixture shell',
    parameters: { type: 'object', properties: { command: { type: 'string' } } },
    output: { schema: { type: 'object' }, render: () => [] },
    execute: () => { throw new Error('persistent shell body must not execute'); },
  });
  const a = await f.agent();
  const event = await f.send(a, 'shell', [{ name: 'bash', args: { command: 'echo test' } }]);
  assert.match(JSON.stringify(event), /persistent shells are unsupported/);
});

test('cancellation revokes unused authorization before another turn', async (t) => {
  const f = await fixture(t);
  const a = await f.agent();
  await f.send(a, 'create', [issue()]);
  const remove = f.ctx.on('tools/pre-execute', async (exec, next) => {
    exec.agent.cancel({ kind: 'user' });
    return next();
  });
  await f.send(a, '授权提交', [issue()]);
  remove();
  await f.send(a, 'continue', [issue()]);
  assert.equal(f.count(), 0);
  await f.send(a, '授权提交', [issue()]);
  assert.equal(f.count(), 1);
});

test('corrupt persisted state and invalid configuration fail closed', async (t) => {
  const f = await fixture(t);
  const a = await f.agent();
  await f.send(a, 'create', [issue()]);
  writeFileSync(join(f.dir, 'state', `${a.id}.json`), '{corrupt');
  await f.send(a, '授权提交', [issue()]);
  assert.equal(f.count(), 0);
  const invalid = await fixture(t, { binary: 'relative-path' });
  const b = await invalid.agent();
  await invalid.send(b, 'read', [{ name: 'read_fixture', args: {} }]);
  assert.equal(invalid.count(), 0);
});

test('guard crash, malformed output and timeout never execute a tool', async (t) => {
  const behaviors = {
    crash: 'process.exit(17)', malformed: "process.stdout.write('[]')",
    timeout: 'Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0)',
  };
  const previous = { mode: process.env.SPS_DSH_FAULT_MODE, binary: process.env.SPS_DSH_REAL_BINARY };
  t.after(() => {
    for (const [key, value] of [['SPS_DSH_FAULT_MODE', previous.mode], ['SPS_DSH_REAL_BINARY', previous.binary]]) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
  });
  for (const [mode, behavior] of Object.entries(behaviors)) {
    const dir = mkdtempSync(join(tmpdir(), 'sps-dsh-fault-'));
    let wrapper = process.env.SPS_DSH_FAULT_BINARY;
    if (process.platform === 'win32') {
      assert.ok(wrapper, 'Windows requires a freshly built native SPS_DSH_FAULT_BINARY fixture');
      process.env.SPS_DSH_FAULT_MODE = mode;
      process.env.SPS_DSH_REAL_BINARY = binary;
    } else {
      wrapper = join(dir, 'guard.cjs');
      writeFileSync(wrapper, `#!${process.execPath}
const { readFileSync } = require('node:fs');
const { spawnSync } = require('node:child_process');
const input = readFileSync(0);
if (JSON.parse(input).hook_event_name === 'PreToolUse') { ${behavior}; }
else {
  const result = spawnSync(${JSON.stringify(binary)}, [], { input });
  process.stdout.write(result.stdout);
  process.exit(result.status ?? 1);
}
`, { mode: 0o755 });
    }
    const f = await fixture(t, { binary: wrapper });
    const a = await f.agent();
    await f.send(a, 'create', [issue()]);
    assert.equal(f.count(), 0);
  }
});

test('real DSH MCP transport preserves arguments and rejects shortened tool identities', async (t) => {
  const f = await fixture(t);
  const mcpMarker = join(f.dir, 'mcp-executed.jsonl');
  const client = await load('dsh-mcp-client');
  const config = {
    transport: 'stdio', serverName: 'fixture', command: process.execPath,
    args: [fileURLToPath(new URL('./dsh-mcp-fixture.cjs', import.meta.url)), mcpMarker],
    failOnStartupError: true, reconnect: { enabled: false },
  };
  await f.ctx.plugin(client, config);
  const a = await f.agent();
  const call = { name: 'mcp__fixture__create_issue', args: issue().args };
  await f.send(a, 'create', [call]);
  assert.equal(existsSync(mcpMarker), false);
  await f.send(a, '授权提交', [call, call]);
  const records = readFileSync(mcpMarker, 'utf8').trim().split('\n').map(JSON.parse);
  assert.equal(records.length, 1);
  assert.deepEqual(records[0].arguments, call.args);
  await f.ctx.plugin(client, { ...config, serverName: 'a'.repeat(32) });
  const shortened = f.ctx.tools.schemas().find(tool =>
    tool.name.startsWith(`mcp__${'a'.repeat(32)}__`) && tool.name.length === 64);
  assert.ok(shortened, 'DSH actually shortened the template-creation name');
  const event = await f.send(a, 'create template', [{
    name: shortened.name, args: { name: 'repo', private: true },
  }]);
  assert.match(JSON.stringify(event), /original operation cannot be verified/);
  assert.equal(readFileSync(mcpMarker, 'utf8').trim().split('\n').length, 1);
});

test('real DSH platform shell: command blocking and a local-only git push', async (t) => {
  const f = await fixture(t);
  const windows = process.platform === 'win32';
  const shellName = windows ? 'pwsh' : 'bash';
  f.tools.get(shellName)();
  const dependencies = ['dsh-subprocess-local', 'dsh-fs-local', 'dsh-shell-env', `dsh-${shellName}-local`];
  // Older DSH uses the session header; newer releases have a directory service.
  try { requireDsh.resolve('@deepseek-ai/dsh-working-directory'); dependencies.push('dsh-working-directory'); } catch {}
  for (const name of dependencies) await f.ctx.plugin((await load(name)).default ?? await load(name));
  await f.ctx.plugin(await load(`dsh-tool-${shellName}`), { enableRunInBackground: false });
  const a = await f.agent();
  const shell = (command, workdir = f.dir) => ({
    name: shellName, args: { description: 'Local test fixture', command, workdir },
  });
  const blocked = join(f.dir, 'must-not-exist');
  const write = (path) => windows
    ? `Set-Content -LiteralPath '${path.replaceAll("'", "''")}' -Value ok -NoNewline`
    : `printf ok > '${path.replaceAll("'", "'\\''")}'`;
  await f.send(a, 'create', [shell(`${write(blocked)}; gh issue create --title test --body test`)]);
  assert.equal(existsSync(blocked), false, 'actual shell process must not start');
  const allowed = join(f.dir, 'read-ran');
  await f.send(a, 'read', [shell(write(allowed))]);
  assert.equal(readFileSync(allowed, 'utf8'), 'ok', 'actual shell did run');

  const repo = join(f.dir, 'repository');
  const remote = join(f.dir, 'remote.git');
  mkdirSync(repo);
  const git = (...args) => execFileSync('git', args, { cwd: repo, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  git('init', '-b', 'main');
  git('init', '--bare', remote);
  writeFileSync(join(repo, 'fixture.txt'), 'initial\n');
  git('add', 'fixture.txt');
  git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-m', 'fixture');
  git('remote', 'add', 'origin', remote);
  const pushed = () => {
    try { return git('--git-dir', remote, 'rev-parse', '--verify', 'refs/heads/main'); } catch { return ''; }
  };
  await f.send(a, 'push', [shell('git push origin main', repo)]);
  assert.equal(pushed(), '');
  await f.send(a, '授权提交', [shell('git push origin main', repo)]);
  assert.equal(pushed(), git('rev-parse', 'HEAD'));
  writeFileSync(join(repo, 'fixture.txt'), 'second\n');
  git('add', 'fixture.txt');
  git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-m', 'second');
  await f.send(a, 'push', [shell('git push origin main', repo)]);
  const previous = pushed();
  writeFileSync(join(repo, 'fixture.txt'), 'third\n');
  git('add', 'fixture.txt');
  git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-m', 'third');
  await f.send(a, '授权提交', [shell('git push origin main', repo)]);
  assert.equal(pushed(), previous, 'changed commits invalidate the reviewed target');
});

test('real DSH directory binding matches the shell and Git; retargeting revokes authorization', async (t) => {
  const f = await fixture(t);
  const windows = process.platform === 'win32';
  const shellName = windows ? 'pwsh' : 'bash';
  f.tools.get(shellName)();
  const dependencies = ['dsh-subprocess-local', 'dsh-fs-local', 'dsh-shell-env', `dsh-${shellName}-local`];
  try { requireDsh.resolve('@deepseek-ai/dsh-working-directory'); dependencies.push('dsh-working-directory'); } catch {}
  for (const name of dependencies) await f.ctx.plugin((await load(name)).default ?? await load(name));
  await f.ctx.plugin(await load(`dsh-tool-${shellName}`), { enableRunInBackground: false });
  const git = (cwd, ...args) => execFileSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  const repos = ['A', 'B'].map(name => {
    const cwd = join(f.dir, name);
    mkdirSync(cwd);
    git(cwd, 'init', '-b', 'main');
    git(cwd, '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
      '-c', 'commit.gpgsign=false', 'commit', '--allow-empty', '-m', name);
    git(cwd, 'init', '--bare', join(f.dir, `${name}.git`));
    git(cwd, 'remote', 'add', 'origin', join(f.dir, `${name}.git`));
    return cwd;
  });
  mkdirSync(join(repos[1], 'sub'));
  symlinkSync(join(repos[1], 'sub'), join(repos[0], 'link'), windows ? 'junction' : 'dir');
  const shell = (command, workdir) => ({
    name: shellName, args: { description: 'Local directory binding fixture', command, workdir },
  });
  const quote = value => `'${value.replaceAll("'", windows ? "''" : "'\\''")}'`;
  for (const [name, workdir, prefix] of [
    ['plain', repos[0], 'git'],
    ['relative-workdir', `A${sep}link${sep}..`, 'git'],
    ['absolute-workdir', `${repos[0]}${sep}link${sep}..`, 'git'],
    ['relative-C', repos[0], 'git -C link/..'],
    ['absolute-C', f.dir, `git -C ${quote(`${repos[0]}/link/..`)}`],
    ['successive-C', repos[0], 'git -C link -C ..'],
    ['workdir-and-C', `A${sep}link`, 'git -C ..'],
  ]) {
    await t.test(name, async () => {
      const a = await f.agent();
      const marker = join(f.dir, `${name}.txt`);
      await f.send(a, 'read actual repository', [shell(`${prefix} rev-parse --show-toplevel > ${quote(marker)}`, workdir)]);
      const bytes = readFileSync(marker);
      const actual = bytes.toString(windows ? 'utf16le' : 'utf8').replace(/^\uFEFF/, '').trim();
      await f.send(a, 'review push', [shell(`${prefix} push origin main`, workdir)]);
      const pending = JSON.parse(readFileSync(join(f.dir, 'state', `${a.session.id}.json`), 'utf8'));
      assert.equal(pending.target?.push?.repository, actual, 'review must match the official shell execution');
      assert.equal(pending.target.push.destination_url, git(actual, 'remote', 'get-url', '--push', 'origin'));
    });
  }

  const link = join(f.dir, 'selected');
  symlinkSync(repos[0], link, windows ? 'junction' : 'dir');
  const a = await f.agent();
  const call = shell('git push origin main', link);
  const pushed = repo => {
    try { git(repo, 'ls-remote', '--exit-code', 'origin', 'refs/heads/main'); return true; } catch { return false; }
  };
  await f.send(a, 'review A', [call]);
  assert.equal(pushed(repos[0]), false);
  if (windows) rmdirSync(link); else unlinkSync(link);
  symlinkSync(repos[1], link, windows ? 'junction' : 'dir');
  await f.send(a, '授权提交', [call]);
  assert.equal(pushed(repos[1]), false, 'changing the directory target invalidates the grant');
  await f.send(a, '授权提交', [call, call]);
  assert.equal(pushed(repos[1]), true);
  assert.equal(pushed(repos[0]), false);
  const results = a.session.snapshotEvents().filter(e => e.type === 'tool/result');
  assert.match(JSON.stringify(results.at(-1)), /授权提交/, 'second call cannot reuse the grant');
});
