// Exercise the installed MCP binary against this container's own health endpoint.
const { spawn } = require('node:child_process');
const readline = require('node:readline');
const child = spawn('playwright-mcp', ['--headless', '--no-sandbox'], {
  stdio: ['pipe', 'pipe', 'inherit'],
});
const pending = new Map();
let nextId = 0;
const lines = readline.createInterface({ input: child.stdout });
lines.on('line', line => {
  let response;
  try { response = JSON.parse(line); } catch { return; }
  const request = pending.get(response.id);
  if (!request) return;
  pending.delete(response.id);
  clearTimeout(request.timer);
  if (response.error) request.reject(new Error(JSON.stringify(response.error)));
  else request.resolve(response.result);
});
child.on('exit', code => {
  for (const request of pending.values()) {
    clearTimeout(request.timer);
    request.reject(new Error(`MCP exited before responding (${code})`));
  }
  pending.clear();
});
function call(method, params) {
  const id = ++nextId;
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`${method} timed out`));
    }, 45000);
    pending.set(id, { resolve, reject, timer });
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
  });
}
(async () => {
  try {
    await call('initialize', {
      protocolVersion: '2024-11-05', capabilities: {},
      clientInfo: { name: 'artex-runtime-smoke', version: '1' },
    });
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }) + '\n');
    const { tools } = await call('tools/list', {});
    if (!tools.some(tool => tool.name === 'browser_navigate')) throw Error('browser_navigate unavailable');
    const result = await call('tools/call', {
      name: 'browser_navigate', arguments: { url: 'http://127.0.0.1:8787/api/health' },
    });
    if (result.isError) throw Error(`Browser navigation failed: ${JSON.stringify(result)}`);
    console.log(`browser MCP: ${tools.length} tools discovered; local navigation passed`);
  } finally {
    lines.close();
    child.stdin.end();
    child.kill('SIGTERM');
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
