import assert from 'node:assert/strict';
import { test } from 'node:test';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync, rmSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const targets = ['darwin-arm64', 'darwin-amd64', 'linux-arm64', 'linux-amd64', 'windows-amd64'];
const read = path => readFileSync(path);
const json = path => JSON.parse(read(path));
const unpack = (host, destination) => {
  const { name, version } = json(join(repo, 'plugins', host, 'package.json'));
  execFileSync('tar', ['-xzf', join(repo, 'dist/npm', `${name}-${version}.tgz`), '-C', destination]);
  return join(destination, 'package');
};

for (const host of ['codex', 'dsh']) {
  test(`${host} tarball contains only release files and the exact distribution binaries`, t => {
    const temporary = mkdtempSync(join(tmpdir(), 'sps-npm-contents-'));
    t.after(() => rmSync(temporary, { recursive: true, force: true }));
    const pkg = unpack(host, temporary);
    const manifest = json(join(pkg, 'package.json'));
    assert.deepEqual(manifest, json(join(repo, 'plugins', host, 'package.json')));
    assert.equal(manifest.scripts, undefined, 'installation must not download or execute extra code');
    assert.equal(manifest.dependencies, undefined);
    assert.deepEqual(read(join(pkg, 'README.md')), read(join(repo, 'plugins', host, host === 'codex' ? 'INSTALL.md' : 'README.md')));
    const expected = ['package.json', 'README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.txt'];
    const prefix = host === 'codex' ? 'marketplace/plugin/' : '';
    if (host === 'codex') {
      expected.push('cli.mjs', 'marketplace/.agents/plugins/marketplace.json');
      for (const name of ['.codex-plugin/plugin.json', 'hooks/hooks.json', 'hooks/run.sh', 'hooks/run.ps1']) {
        expected.push(prefix + name);
        assert.deepEqual(read(join(pkg, prefix, name)), read(join(repo, 'plugins/codex', name)));
      }
      assert.equal(json(join(pkg, prefix, '.codex-plugin/plugin.json')).version, manifest.version);
      assert.equal(json(join(pkg, 'marketplace/.agents/plugins/marketplace.json')).plugins[0].source.path, './plugin');
    } else {
      for (const name of ['index.js', 'cordis.patch.yml']) {
        expected.push(name);
        assert.deepEqual(read(join(pkg, name)), read(join(repo, 'plugins/dsh', name)));
      }
    }
    for (const target of targets) {
      if (host === 'dsh' && ['darwin-amd64', 'linux-amd64'].includes(target)) continue;
      const name = `issue-pr-guard${target === 'windows-amd64' ? '.exe' : ''}`;
      const folder = host === 'dsh' && target === 'windows-amd64' ? 'win32-x64' : target;
      const relative = `${prefix}bin/${folder}/${name}`;
      expected.push(relative);
      assert.deepEqual(read(join(pkg, relative)), read(join(repo, 'plugins/codex/bin', target, name)));
      if (process.platform !== 'win32') assert.ok(statSync(join(pkg, relative)).mode & 0o111);
    }
    const files = readdirSync(pkg, { recursive: true }).filter(name => statSync(join(pkg, name)).isFile());
    assert.deepEqual(files.map(name => name.replaceAll('\\', '/')).sort(), expected.sort());
  });
}

test('Codex installs, reinstalls, and enforces authorization after the npm source is removed', t => {
  const temporary = mkdtempSync(join(tmpdir(), 'sps-npm-中文 空格 &-'));
  t.after(() => rmSync(temporary, { recursive: true, force: true }));
  const pkg = unpack('codex', temporary);
  const version = json(join(pkg, 'package.json')).version;
  const isolated = join(temporary, 'codex-home');
  const env = { ...process.env, CODEX_HOME: isolated };
  const cli = join(pkg, 'cli.mjs');
  assert.equal(execFileSync(process.execPath, [cli, '--version'], { env, encoding: 'utf8' }).trim(), version);
  const bad = spawnSync(process.execPath, [cli, '--unexpected'], { env, encoding: 'utf8' });
  assert.equal(bad.status, 1);
  for (let i = 0; i < 2; i++) execFileSync(process.execPath, [cli], { env });
  rmSync(pkg, { recursive: true });
  const source = join(isolated, 'npm/codex-stop-pushing-shit');
  const result = process.platform === 'win32'
    ? spawnSync('codex plugin add issue-pr-guard@stop-pushing-shit', { cwd: source, env, shell: true, encoding: 'utf8' })
    : spawnSync('codex', ['plugin', 'add', 'issue-pr-guard@stop-pushing-shit'], { cwd: source, env, encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  const installed = join(isolated, 'plugins/cache/stop-pushing-shit/issue-pr-guard', version);
  assert.equal(json(join(installed, '.codex-plugin/plugin.json')).version, version);
  const invoke = event => {
    const windows = process.platform === 'win32';
    const program = windows ? 'powershell.exe' : '/bin/sh';
    const args = windows
      ? ['-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', join(installed, 'hooks/run.ps1')]
      : [join(installed, 'hooks/run.sh')];
    return JSON.parse(execFileSync(program, args, {
      env: { ...env, PLUGIN_ROOT: installed, PLUGIN_DATA: join(temporary, 'state') },
      input: JSON.stringify(event), encoding: 'utf8',
    }));
  };
  const event = {
    hook_event_name: 'PreToolUse', session_id: 'npm-package', turn_id: 'one', cwd: temporary,
    tool_name: 'github.create_repository', tool_input: { name: 'npm-fixture', private: true },
  };
  assert.equal(invoke(event).hookSpecificOutput.permissionDecision, 'deny');
  invoke({ hook_event_name: 'UserPromptSubmit', session_id: event.session_id, turn_id: 'two', prompt: '授权提交' });
  event.turn_id = 'two';
  assert.deepEqual(invoke(event), {});
  assert.equal(invoke(event).hookSpecificOutput.permissionDecision, 'deny');
});
