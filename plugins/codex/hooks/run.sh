#!/bin/sh
# Missing or crashed binaries must produce a blocking exit.
guard_root=${PLUGIN_ROOT:-${CLAUDE_PLUGIN_ROOT:-}}
guard_os=$(/usr/bin/uname -s 2>/dev/null || /bin/uname -s) || exit 2
guard_arch=$(/usr/bin/uname -m 2>/dev/null || /bin/uname -m) || exit 2
case "$guard_os" in Darwin) guard_os=darwin ;; Linux) guard_os=linux ;; *) exit 2 ;; esac
case "$guard_arch" in arm64|aarch64) guard_arch=arm64 ;; x86_64|amd64) guard_arch=amd64 ;; *) exit 2 ;; esac
guard_program="$guard_root/bin/$guard_os-$guard_arch/issue-pr-guard"
# Single-platform developer packages retain the original flat binary path.
if [ ! -f "$guard_program" ]; then guard_program="$guard_root/bin/issue-pr-guard"; fi
if [ -z "$guard_root" ] || [ ! -x "$guard_program" ]; then
  echo 'Issue / PR guard: 缺少插件程序，拒绝本次工具调用。' >&2
  exit 2
fi
"$guard_program"
guard_status=$?
if [ "$guard_status" -ne 0 ]; then
  echo 'Issue / PR guard: 检查失败，拒绝本次工具调用。' >&2
  exit 2
fi
