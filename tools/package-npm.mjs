import assert from 'node:assert/strict';
import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const output = join(repo, 'dist/npm');
const stage = mkdtempSync(join(tmpdir(), 'sps-npm-package-'));
const targets = ['darwin-arm64', 'darwin-amd64', 'linux-arm64', 'linux-amd64', 'windows-amd64'];
const copy = (from, to) => {
  mkdirSync(dirname(to), { recursive: true });
  copyFileSync(from, to);
};
mkdirSync(output, { recursive: true });
try {
  for (const host of ['codex', 'dsh']) {
    const source = join(repo, 'plugins', host);
    const pkg = join(stage, host);
    const manifest = JSON.parse(readFileSync(join(source, 'package.json'), 'utf8'));
    copy(join(source, 'package.json'), join(pkg, 'package.json'));
    for (const file of ['LICENSE', 'THIRD_PARTY_NOTICES.txt']) copy(join(repo, file), join(pkg, file));
    let binaryRoot;
    if (host === 'codex') {
      const pluginManifest = JSON.parse(readFileSync(join(source, '.codex-plugin/plugin.json'), 'utf8'));
      assert.equal(manifest.version, pluginManifest.version, 'Codex npm and plugin versions differ');
      copy(join(source, 'cli.mjs'), join(pkg, 'cli.mjs'));
      chmodSync(join(pkg, 'cli.mjs'), 0o755);
      copy(join(source, 'INSTALL.md'), join(pkg, 'README.md'));
      copy(join(source, 'marketplace.json'), join(pkg, 'marketplace/.agents/plugins/marketplace.json'));
      for (const file of ['.codex-plugin/plugin.json', 'hooks/hooks.json', 'hooks/run.sh', 'hooks/run.ps1']) {
        copy(join(source, file), join(pkg, 'marketplace/plugin', file));
      }
      chmodSync(join(pkg, 'marketplace/plugin/hooks/run.sh'), 0o755);
      binaryRoot = join(pkg, 'marketplace/plugin/bin');
    } else {
      for (const file of ['index.js', 'cordis.patch.yml', 'README.md']) copy(join(source, file), join(pkg, file));
      binaryRoot = join(pkg, 'bin');
    }
    for (const target of targets) {
      if (host === 'dsh' && target.endsWith('amd64') && target !== 'windows-amd64') continue;
      const binary = `issue-pr-guard${target === 'windows-amd64' ? '.exe' : ''}`;
      const folder = host === 'dsh' && target === 'windows-amd64' ? 'win32-x64' : target;
      const destination = join(binaryRoot, folder, binary);
      copy(join(repo, 'plugins/codex/bin', target, binary), destination);
      chmodSync(destination, 0o755);
    }
    const [packed] = JSON.parse(execFileSync('npm', ['pack', '--json', '--ignore-scripts', '--offline', '--pack-destination', output], {
      cwd: pkg, encoding: 'utf8', env: { ...process.env, npm_config_cache: join(stage, 'npm-cache') },
    }));
    console.log(`${join(output, packed.filename)} (${packed.size} bytes)`);
  }
} finally {
  rmSync(stage, { recursive: true, force: true });
}
