import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, chmodSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const [platform, executable] = process.argv.slice(2);
if (!['darwin-arm64', 'linux-arm64', 'windows-amd64'].includes(platform) || !executable) {
  throw new Error('Usage: node tools/package-dsh.mjs <darwin-arm64|linux-arm64|windows-amd64> <freshly-built-guard>');
}
const binaryPlatform = platform === 'windows-amd64' ? 'win32-x64' : platform;
const binaryName = platform === 'windows-amd64' ? 'issue-pr-guard.exe' : 'issue-pr-guard';
const stage = mkdtempSync(join(tmpdir(), 'sps-dsh-package-'));
const pkg = join(stage, 'package');
mkdirSync(join(pkg, 'bin', binaryPlatform), { recursive: true });
for (const file of ['index.js', 'package.json', 'cordis.patch.yml', 'README.md']) {
  copyFileSync(join(repo, 'plugins/dsh', file), join(pkg, file));
}
for (const file of ['LICENSE', 'THIRD_PARTY_NOTICES.txt']) {
  copyFileSync(join(repo, file), join(pkg, file));
}
copyFileSync(resolve(executable), join(pkg, 'bin', binaryPlatform, binaryName));
chmodSync(join(pkg, 'bin', binaryPlatform, binaryName), 0o755);
const { version } = JSON.parse(readFileSync(join(pkg, 'package.json'), 'utf8'));
const output = join(repo, 'dist/dsh');
mkdirSync(output, { recursive: true });
const archive = join(output, `dsh-stop-pushing-shit-${version}-${platform}.tgz`);
const metadata = process.platform === 'darwin'
  ? ['--no-mac-metadata', '--no-xattrs', '--no-acls', '--no-fflags', '--uid=0', '--gid=0', '--uname=root', '--gname=root']
  : ['--owner=0', '--group=0', '--numeric-owner'];
execFileSync('tar', ['--format=ustar', ...metadata, '-czf', archive, '-C', stage, 'package'], {
  env: { ...process.env, COPYFILE_DISABLE: '1' },
});
console.log(archive);
