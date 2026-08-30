// Unified local dev: one clean URL that mirrors the deployment.
//
// Runs the sample API, an `ng build --watch` for each app, and the Go server —
// which serves the two built apps under one origin (public at /, vendor-admin at
// /vendor/) and the API at /v1, exactly like production. So http://localhost:5200
// behaves like the deployed site; saving a file triggers a rebuild + reload.
//
// We serve the *built* output rather than the Angular dev servers on purpose:
// the apps and the source-consumed @coldcrabby/ui share one Angular only through
// the esbuild build, so serving dist sidesteps the dev-server's duplicate-Angular
// pitfall. Ports other than 5200 are internal plumbing.
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';

const PORTS = { unified: 5200, sample: 8787 };
const UNIFIED_URL = `http://localhost:${PORTS.unified}`;
const DIST = {
  public: 'apps/public/dist/public/browser/index.html',
  vendor: 'apps/vendor-admin/dist/vendor-admin/browser/index.html',
};

const goEnv = {
  ADDR: `:${PORTS.unified}`,
  SAMPLE_API_URL: `http://localhost:${PORTS.sample}`,
};

const colors = { 'sample-api': 90, 'go-server': 36, public: 33, vendor: 35, dev: 32 };
const width = Math.max(...['sample-api', 'go-server', 'public', 'vendor', 'dev'].map((l) => l.length));

const children = [];
let shuttingDown = false;

function prefix(label) {
  return `\x1b[${colors[label] ?? 37}m[${label.padEnd(width)}]\x1b[0m `;
}
function log(label, text) {
  process.stdout.write(prefix(label) + text + '\n');
}
function pipe(label, stream, out) {
  let buffer = '';
  stream.on('data', (chunk) => {
    buffer += chunk.toString();
    const lines = buffer.split('\n');
    buffer = lines.pop() ?? '';
    for (const line of lines) out.write(prefix(label) + line + '\n');
  });
}

function start(label, command, args, extraEnv = {}) {
  const child = spawn(command, args, {
    env: { ...process.env, ...extraEnv },
    stdio: ['ignore', 'pipe', 'pipe'],
    // Own process group so shutdown can take down grandchildren too — notably
    // `go run`, which spawns the compiled server binary and would otherwise
    // leave it holding the port.
    detached: true,
  });
  pipe(label, child.stdout, process.stdout);
  pipe(label, child.stderr, process.stderr);
  child.on('exit', (code) => {
    if (shuttingDown) return;
    log(label, `exited (${code}); shutting the rest down.`);
    shutdown(code ?? 1);
  });
  children.push(child);
}

function shutdown(code) {
  if (shuttingDown) return;
  shuttingDown = true;
  for (const child of children) {
    // Negative pid signals the whole process group (see detached above).
    try {
      if (child.pid) process.kill(-child.pid, 'SIGTERM');
    } catch {
      // Already gone.
    }
  }
  setTimeout(() => process.exit(code), 500);
}

process.on('SIGINT', () => shutdown(0));
process.on('SIGTERM', () => shutdown(0));

process.stdout.write(`\n  Cold Crabby dev is starting — building the apps…\n\n`);

start('sample-api', 'node', ['tools/sample-api/server.mjs']);
start('public', 'pnpm', ['--filter', 'public', 'run', 'watch']);
start('vendor', 'pnpm', ['--filter', 'vendor-admin', 'run', 'watch']);

// The Go server disables a frontend whose build dir is missing at startup, so
// wait for the first build of both apps before launching it.
let goStarted = false;
const poll = setInterval(() => {
  if (goStarted || shuttingDown) return;
  if (existsSync(DIST.public) && existsSync(DIST.vendor)) {
    goStarted = true;
    clearInterval(poll);
    start('go-server', 'go', ['run', './cmd/server'], goEnv);
    process.stdout.write(`\n  Cold Crabby dev is up — open \x1b[1m${UNIFIED_URL}\x1b[0m\n\n`);
  }
}, 500);
