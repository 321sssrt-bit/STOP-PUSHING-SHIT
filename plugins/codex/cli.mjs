#!/usr/bin/env node
import { cpSync, existsSync, mkdirSync, readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { homedir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const { version } = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'));
const args = process.argv.slice(2);
if (args.length === 1 && args[0] === '--version') {
  console.log(version);
} else if (args.length === 1 && ['--help', '-h'].includes(args[0])) {
  console.log('Usage: codex-stop-pushing-shit [install]\nInstall the bundled Codex plugin. Then start a new chat and review /hooks.');
} else if (args.length > 1 || (args.length === 1 && args[0] !== 'install')) {
  console.error('Usage: codex-stop-pushing-shit [install|--help|--version]');
  process.exitCode = 1;
} else {
  try {
    const platform = `${process.platform === 'win32' ? 'windows' : process.platform}-${process.arch === 'x64' ? 'amd64' : process.arch}`;
    const binary = `issue-pr-guard${process.platform === 'win32' ? '.exe' : ''}`;
    const source = join(root, 'marketplace');
    if (!existsSync(join(source, 'plugin', 'bin', platform, binary))) {
      throw new Error(`No bundled guard for ${platform}. Use a supported GitHub Release package.`);
    }
    // Keep the marketplace outside npm's disposable npx cache.
    const destination = resolve(process.env.CODEX_HOME || join(homedir(), '.codex'), 'npm', 'codex-stop-pushing-shit');
    mkdirSync(destination, { recursive: true });
    cpSync(source, destination, { recursive: true });
    for (const command of [
      ['plugin', 'marketplace', 'add', '.'],
      ['plugin', 'add', 'issue-pr-guard@stop-pushing-shit'],
    ]) {
      // Windows npm exposes codex.cmd; only fixed command text enters the shell.
      const result = process.platform === 'win32'
        ? spawnSync(`codex ${command.join(' ')}`, { cwd: destination, stdio: 'inherit', shell: true })
        : spawnSync('codex', command, { cwd: destination, stdio: 'inherit' });
      if (result.error) throw result.error;
      if (result.status !== 0) throw new Error('Codex installation failed. Check the error above and the installation guide.');
    }
    console.log(`Installed STOP-PUSHING-SHIT ${version}. Start a new chat and review /hooks to trust the hooks.`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
