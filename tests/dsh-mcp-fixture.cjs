const { createInterface } = require('node:readline');
const { appendFileSync } = require('node:fs');

createInterface({ input: process.stdin }).on('line', (line) => {
  const request = JSON.parse(line);
  if (request.id === undefined) return;
  let result = {};
  if (request.method === 'initialize') {
    result = {
      protocolVersion: '2024-11-05', capabilities: { tools: {} },
      serverInfo: { name: 'sps-local-fixture', version: '1' },
    };
  } else if (request.method === 'tools/list') {
    result = { tools: ['create_issue', 'create_repository_from_template'].map(name => ({
      name, description: 'Local fixture that appends a temporary execution marker.',
      inputSchema: { type: 'object', properties: {} },
    })) };
  } else if (request.method === 'tools/call') {
    appendFileSync(process.argv[2], JSON.stringify(request.params) + '\n');
    result = { content: [{ type: 'text', text: 'local MCP fixture executed' }] };
  }
  process.stdout.write(JSON.stringify({ jsonrpc: '2.0', id: request.id, result }) + '\n');
});
