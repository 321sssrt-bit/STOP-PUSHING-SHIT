import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { homedir } from 'node:os';
import { dirname, isAbsolute, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

export const name = 'stop-pushing-shit';
export const inject = ['tools', 'agents', 'systemPrompt'];

const root = dirname(fileURLToPath(import.meta.url));
const failure = 'STOP-PUSHING-SHIT: guard unavailable; tool execution denied.';

export function apply(ctx, config = {}) {
  const binary = config.binary ?? join(root, 'bin', `${process.platform}-${process.arch}`,
    process.platform === 'win32' ? 'issue-pr-guard.exe' : 'issue-pr-guard');
  const dataDir = config.dataDir ?? join(process.env.DSH_HOME || join(homedir(), '.dsh'),
    'data', 'stop-pushing-shit');
  const sessions = new WeakMap();

  function run(event) {
    if (!isAbsolute(binary) || !isAbsolute(dataDir)) throw new Error(failure);
    const input = JSON.stringify(event);
    if (Buffer.byteLength(input) > 16 << 20) throw new Error(failure);
    const result = spawnSync(binary, [], {
      input, encoding: 'utf8', shell: false, timeout: 3000, killSignal: 'SIGKILL',
      maxBuffer: 16 << 20, windowsHide: true,
      env: { ...process.env, PLUGIN_DATA: dataDir },
    });
    if (result.error || result.status !== 0 || result.signal) throw new Error(failure);
    const output = JSON.parse(result.stdout);
    if (!output || typeof output !== 'object' || Array.isArray(output)) throw new Error(failure);
    if (Object.keys(output).length === 0) return output;
    const hook = output.hookSpecificOutput;
    if (!hook || hook.hookEventName !== event.hook_event_name) throw new Error(failure);
    if (event.hook_event_name === 'PreToolUse') {
      if (hook.permissionDecision !== 'deny' || typeof hook.permissionDecisionReason !== 'string') {
        throw new Error(failure);
      }
    } else if (typeof hook.additionalContext !== 'string') {
      throw new Error(failure);
    }
    return output;
  }

  function eventFor(session, type, extra = {}) {
    return {
      hook_event_name: type, session_id: session.id,
      turn_id: sessions.get(session)?.turn ?? '', ...extra,
    };
  }

  function initialize(session) {
    const lastDirectory = session.snapshotEvents().findLast(e => e.type === 'working-directory/change');
    const state = { turn: '', error: null, cwd: lastDirectory?.data.cwd ?? session.header.cwd };
    sessions.set(session, state);
    try {
      run(eventFor(session, 'SessionStart'));
    } catch (error) {
      state.error = error;
    }
    return state;
  }

  // Register the denial gate before probing the executable: a broken install
  // remains restrictive even when the host contains plugin startup errors.
  ctx.tools.guard((exec) => {
    const agent = exec.agent;
    const state = agent && sessions.get(agent.session);
    if (!agent || ctx.agents.get(agent.id) !== agent || agent.status !== 'running' ||
        !state?.turn || state.error || exec.signal.aborted) return failure;
    try {
      const args = exec.arguments;
      let tool = exec.name;
      let input = args;
      let cwd = ctx.get('workingDirectory')?.get(agent.session) ?? state.cwd;
      // DSH drops the original MCP name from ToolExecution when it shortens
      // or normalizes names. Never classify that lossy public name as a read.
      if (/^mcp__.*_[0-9a-f]{12}$/.test(tool)) {
        return 'STOP-PUSHING-SHIT: DSH renamed this MCP tool; its original operation cannot be verified. Use a shorter server name and an unmodified tool name.';
      }
      if (tool === 'terminal_open' || tool === 'terminal_send') {
        return 'STOP-PUSHING-SHIT: interactive terminal input cannot be checked as a complete command. Use the standard bash or pwsh tool.';
      }
      if (tool === 'bash' || tool === 'pwsh') {
        const definition = ctx.tools.get(tool, agent);
        if (!definition?.parameters?.properties?.workdir) {
          return 'STOP-PUSHING-SHIT: persistent shells are unsupported. Use dsh-tool-bash or dsh-tool-pwsh (one command per process).';
        }
        if (!args || typeof args.command !== 'string' || !isAbsolute(cwd ?? '')) return failure;
        if (args.workdir !== undefined) {
          if (typeof args.workdir !== 'string' || args.workdir.length === 0) return failure;
          // Match each official tool: bash preserves traversal; pwsh resolves relative paths.
          cwd = isAbsolute(args.workdir) ? args.workdir
            : tool === 'pwsh' ? resolve(cwd, args.workdir) : `${cwd}${sep}${args.workdir}`;
        }
        // Retain every original argument and the actual tool name in the Go
        // target binding while using its existing shell-command classifier.
        tool = 'Bash';
        input = { command: args.command, dsh_tool: exec.name, dsh_arguments: args };
      }
      if (!isAbsolute(cwd ?? '')) return failure;
      const output = run(eventFor(agent.session, 'PreToolUse', {
        cwd, tool_name: tool, tool_input: input,
      }));
      return output.hookSpecificOutput?.permissionDecisionReason;
    } catch (error) {
      state.error = error;
      return failure;
    }
  });

  ctx.on('agent/created', ({ agent }) => {
    initialize(agent.session);
  });
  ctx.on('session/event', (session, event) => {
    if (!['turn/start', 'turn/end', 'user/message', 'working-directory/change'].includes(event.type)) return;
    let state = sessions.get(session);
    if (!state) state = initialize(session);
    try {
      if (event.type === 'working-directory/change') {
        state.cwd = event.data.cwd;
      } else if (event.type === 'turn/start') {
        state.turn = randomUUID();
        run(eventFor(session, 'Interrupt'));
        state.error = null;
      } else if (event.type === 'turn/end') {
        state.turn = '';
        run(eventFor(session, 'Interrupt'));
      } else if (event.data.source?.kind === 'user') {
        const agent = ctx.agents.get(session.id);
        const message = event.data;
        const direct = agent && ctx.agents.roots().includes(agent) &&
          session.header.origin !== 'subagent';
        const text = direct && message.role === 'user' && message.content.length === 1 &&
          message.content[0].type === 'text' ? message.content[0].text : '';
        run(eventFor(session, 'UserPromptSubmit', { prompt: text }));
      }
    } catch (error) {
      state.error = error;
    }
  });
  // No second copy of the policy text in the adapter.
  ctx.systemPrompt.section({
    name, order: 5, interpolate: false,
    text: () => {
      try {
        return run({ hook_event_name: 'SubagentStart' }).hookSpecificOutput.additionalContext;
      } catch {
        return failure;
      }
    },
  });
}
