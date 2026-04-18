#!/usr/bin/env node
//
// Postinstall hook for @liaisonio/cli.
//
// Downloads the platform-specific Go binary from liaison.cloud (primary) or
// GitHub releases (fallback), and verifies its SHA256 against SHA256SUMS. The binary is
// dropped at vendor/<liaison|liaison.exe> next to this script's parent.
//
// Skipped automatically when:
//   - LIAISON_CLI_SKIP_DOWNLOAD=1 (dev / CI scenarios that don't need the bin)
//   - The user is on an unsupported platform (we exit 0 + warn, NOT fail,
//     so npm install doesn't break for transitive deps)
//
// Honours HTTPS_PROXY / HTTP_PROXY env vars for users behind corporate proxies
// (Node's built-in https.get does NOT honour these out of the box — we handle
// it manually via a CONNECT tunnel). Retries once on transient network errors.
//
// Network failures DO fail the install — silently shipping a broken package
// is worse than a clear error the user can retry.

'use strict';

const https = require('https');
const http = require('http');
const net = require('net');
const tls = require('tls');
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const { spawnSync } = require('child_process');
const { URL } = require('url');

const pkg = require('../package.json');
const VERSION = `v${pkg.version}`;
const REPO = 'liaisonio/cli';

// Download source. By default we try liaison.cloud first (our own CDN), then
// fall back to GitHub releases. Users can override this by setting
// LIAISON_CLI_MIRROR to a custom host, e.g.:
//
//   LIAISON_CLI_MIRROR=https://my-mirror.example.com/releases npm i -g @liaisonio/cli
//
// The mirror URL must serve the same directory layout:
//   ${MIRROR}/${VERSION}/liaison-v0.2.5-linux-amd64
//   ${MIRROR}/${VERSION}/SHA256SUMS
const GITHUB_BASE = `https://github.com/${REPO}/releases/download/${VERSION}`;
const CHINA_MIRROR = `https://liaison.cloud/releases/${VERSION}`;
const RELEASE_BASE = process.env.LIAISON_CLI_MIRROR
  ? `${process.env.LIAISON_CLI_MIRROR.replace(/\/+$/, '')}/${VERSION}`
  : GITHUB_BASE;

// How long to wait for a single HTTP round-trip before giving up.
const SOCKET_TIMEOUT_MS = 30_000;
// How many times to retry a download after a transient error. Retries are
// linear-backoff with 2s then 5s gaps.
const RETRY_DELAYS_MS = [2000, 5000];

// process.platform-process.arch → release asset GOOS-GOARCH suffix.
const PLATFORMS = {
  'darwin-arm64': { os: 'darwin', arch: 'arm64', ext: '' },
  'darwin-x64': { os: 'darwin', arch: 'amd64', ext: '' },
  'linux-arm64': { os: 'linux', arch: 'arm64', ext: '' },
  'linux-x64': { os: 'linux', arch: 'amd64', ext: '' },
  'win32-x64': { os: 'windows', arch: 'amd64', ext: '.exe' },
};

function log(msg) {
  process.stdout.write(`liaison-cli: ${msg}\n`);
}

function warn(msg) {
  process.stderr.write(`liaison-cli: ${msg}\n`);
}

function die(msg) {
  warn(msg);
  process.exit(1);
}

if (process.env.LIAISON_CLI_SKIP_DOWNLOAD === '1') {
  log('LIAISON_CLI_SKIP_DOWNLOAD=1, skipping binary download');
  process.exit(0);
}

const key = `${process.platform}-${process.arch}`;
const platform = PLATFORMS[key];
if (!platform) {
  warn(
    `unsupported platform ${key}; package installed without a binary. ` +
      `Use --ignore-scripts or set LIAISON_CLI_SKIP_DOWNLOAD=1 to silence.`,
  );
  // Exit 0 so transitive dependents don't break.
  process.exit(0);
}

const filename = `liaison-${VERSION}-${platform.os}-${platform.arch}${platform.ext}`;
const url = `${RELEASE_BASE}/${filename}`;
const sumsUrl = `${RELEASE_BASE}/SHA256SUMS`;

const vendorDir = path.join(__dirname, '..', 'vendor');
const destPath = path.join(vendorDir, `liaison${platform.ext}`);

fs.mkdirSync(vendorDir, { recursive: true });

// ─── proxy support ──────────────────────────────────────────────────────────

// getProxy reads HTTPS_PROXY / HTTP_PROXY / https_proxy / http_proxy, in the
// order npm / curl do. Returns a parsed URL or null.
function getProxy() {
  const raw =
    process.env.HTTPS_PROXY ||
    process.env.https_proxy ||
    process.env.HTTP_PROXY ||
    process.env.http_proxy;
  if (!raw) return null;
  try {
    return new URL(raw);
  } catch {
    warn(`ignoring invalid proxy URL: ${raw}`);
    return null;
  }
}

// tunnelThroughProxy opens a TCP connection to the proxy, issues an HTTP
// CONNECT to the target, and upgrades the socket to TLS. Returns a Promise
// resolving to an established TLSSocket that can be handed to https.request
// as `createConnection`. Zero external dependencies.
function tunnelThroughProxy(proxy, targetHost, targetPort) {
  return new Promise((resolve, reject) => {
    const proxyPort = proxy.port || (proxy.protocol === 'https:' ? 443 : 80);
    const authHeader = proxy.username
      ? `Proxy-Authorization: Basic ${Buffer.from(
          `${decodeURIComponent(proxy.username)}:${decodeURIComponent(proxy.password || '')}`,
        ).toString('base64')}\r\n`
      : '';

    const proxySocket = net.connect(proxyPort, proxy.hostname, () => {
      proxySocket.write(
        `CONNECT ${targetHost}:${targetPort} HTTP/1.1\r\n` +
          `Host: ${targetHost}:${targetPort}\r\n` +
          authHeader +
          `\r\n`,
      );
    });

    let buf = Buffer.alloc(0);
    const onData = (chunk) => {
      buf = Buffer.concat([buf, chunk]);
      const end = buf.indexOf('\r\n\r\n');
      if (end === -1) return;
      const head = buf.slice(0, end).toString('utf8');
      const status = head.split(' ')[1];
      proxySocket.removeListener('data', onData);
      if (status !== '200') {
        reject(new Error(`proxy CONNECT failed: ${head.split('\r\n')[0]}`));
        proxySocket.end();
        return;
      }
      // Upgrade to TLS — the CONNECT tunnel is now a raw TCP pipe to target.
      const tlsSocket = tls.connect({
        socket: proxySocket,
        servername: targetHost,
      });
      tlsSocket.once('secureConnect', () => resolve(tlsSocket));
      tlsSocket.once('error', reject);
    };
    proxySocket.on('data', onData);
    proxySocket.once('error', reject);
  });
}

// httpsGetRaw makes a single HTTPS GET request. If an HTTPS_PROXY is set, it
// tunnels through it via CONNECT; otherwise it uses stock https.request.
// Returns a Promise<IncomingMessage>. Caller is responsible for consuming or
// discarding the body.
function httpsGetRaw(targetUrl) {
  return new Promise((resolve, reject) => {
    const u = new URL(targetUrl);
    const proxy = getProxy();

    const reqOptions = {
      host: u.hostname,
      port: u.port || 443,
      path: u.pathname + u.search,
      method: 'GET',
      headers: {
        'User-Agent': 'liaison-cli-installer',
        Host: u.host,
      },
      timeout: SOCKET_TIMEOUT_MS,
    };

    const runRequest = (agentSocket) => {
      if (agentSocket) {
        reqOptions.createConnection = () => agentSocket;
      }
      const req = https.request(reqOptions, resolve);
      req.on('error', reject);
      req.on('timeout', () => {
        req.destroy(new Error(`timeout after ${SOCKET_TIMEOUT_MS}ms`));
      });
      req.end();
    };

    if (proxy) {
      tunnelThroughProxy(proxy, u.hostname, u.port || 443)
        .then(runRequest)
        .catch(reject);
    } else {
      runRequest(null);
    }
  });
}

// Follow HTTP redirects up to maxRedirects, returning the final IncomingMessage
// that actually holds the body. Non-2xx status codes at the end of the chain
// are returned as-is; caller decides whether to treat them as an error.
async function httpsGet(targetUrl, maxRedirects = 5) {
  let current = targetUrl;
  for (let i = 0; i <= maxRedirects; i++) {
    const res = await httpsGetRaw(current);
    if (
      res.statusCode >= 300 &&
      res.statusCode < 400 &&
      res.headers.location
    ) {
      res.resume();
      current = res.headers.location;
      continue;
    }
    return res;
  }
  throw new Error(`too many redirects fetching ${targetUrl}`);
}

// withRetry wraps an async function so it's tried a few times with backoff
// on network errors. Does NOT retry on 4xx/5xx from the server — those are
// deterministic and a retry will just hit the same error.
async function withRetry(label, fn) {
  let lastErr;
  for (let attempt = 0; attempt <= RETRY_DELAYS_MS.length; attempt++) {
    try {
      return await fn();
    } catch (err) {
      lastErr = err;
      if (attempt < RETRY_DELAYS_MS.length) {
        const delay = RETRY_DELAYS_MS[attempt];
        warn(`${label}: ${err.message}, retrying in ${delay}ms...`);
        await new Promise((r) => setTimeout(r, delay));
        continue;
      }
    }
  }
  throw lastErr;
}

function downloadToFile(targetUrl, outPath) {
  return withRetry(`download ${path.basename(outPath)}`, async () => {
    const res = await httpsGet(targetUrl);
    if (res.statusCode !== 200) {
      res.resume();
      throw new Error(`HTTP ${res.statusCode} for ${targetUrl}`);
    }
    await new Promise((resolve, reject) => {
      const file = fs.createWriteStream(outPath);
      res.pipe(file);
      file.on('finish', () => file.close(() => resolve()));
      file.on('error', reject);
      res.on('error', reject);
    });
  });
}

function downloadToString(targetUrl) {
  return withRetry(`fetch ${path.basename(new URL(targetUrl).pathname)}`, async () => {
    const res = await httpsGet(targetUrl);
    if (res.statusCode !== 200) {
      res.resume();
      throw new Error(`HTTP ${res.statusCode} for ${targetUrl}`);
    }
    let body = '';
    res.setEncoding('utf8');
    for await (const chunk of res) body += chunk;
    return body;
  });
}

function sha256(filepath) {
  const hash = crypto.createHash('sha256');
  hash.update(fs.readFileSync(filepath));
  return hash.digest('hex');
}

// fetchFromBase tries to download the binary + SHA256SUMS from a given base URL.
// Returns true on success, false on failure (so the caller can try the next mirror).
async function fetchFromBase(base, label) {
  const binaryUrl = `${base}/${filename}`;
  const checksumUrl = `${base}/SHA256SUMS`;

  log(`[${label}] fetching ${binaryUrl}`);
  await downloadToFile(binaryUrl, destPath);
  fs.chmodSync(destPath, 0o755);

  log(`[${label}] verifying SHA256`);
  const sums = await downloadToString(checksumUrl);
  const line = sums
    .split('\n')
    .map((l) => l.trim())
    .find((l) => l.endsWith(filename));
  if (!line) {
    fs.unlinkSync(destPath);
    throw new Error(`no SHA256 entry for ${filename} in SHA256SUMS`);
  }
  const expected = line.split(/\s+/)[0];
  const actual = sha256(destPath);
  if (expected !== actual) {
    fs.unlinkSync(destPath);
    throw new Error(`SHA256 mismatch: expected ${expected}, got ${actual}`);
  }
  log(`installed ${filename} (sha256 ok)`);
}

// installSkills invokes `liaison skills install -g` with the just-downloaded
// binary so the agent SKILL.md files land at ~/.claude/skills. This is a
// best-effort step — if it fails (no $HOME, permission denied, whatever) we
// warn and move on; the user can re-run it manually with
// `liaison skills install -g`. Opt out entirely with
// LIAISON_CLI_SKIP_SKILLS=1.
function installSkills() {
  if (process.env.LIAISON_CLI_SKIP_SKILLS === '1') {
    log('LIAISON_CLI_SKIP_SKILLS=1, skipping agent skill install');
    return;
  }
  try {
    const res = spawnSync(destPath, ['skills', 'install', '-g'], {
      stdio: ['ignore', 'pipe', 'pipe'],
      timeout: 15_000,
    });
    if (res.error || res.status !== 0) {
      const msg = res.error ? res.error.message : `exit ${res.status}`;
      warn(
        `agent skill install failed (${msg}) — re-run \`liaison skills install -g\` to retry`,
      );
      return;
    }
    // Strip the trailing newline from the CLI's own output and prefix with our tag.
    const out = (res.stdout || '').toString().trim();
    if (out) log(out.split('\n').join('\n  '));
  } catch (err) {
    warn(`agent skill install errored (${err.message}) — re-run manually`);
  }
}

async function main() {
  const proxy = getProxy();
  if (proxy) {
    log(`using proxy ${proxy.protocol}//${proxy.hostname}:${proxy.port || ''}`);
  }

  // If the user set an explicit mirror, use only that.
  if (process.env.LIAISON_CLI_MIRROR) {
    await fetchFromBase(RELEASE_BASE, 'mirror');
  } else {
    // Try our own server first (fast for China and most regions), fall back to
    // GitHub releases if liaison.cloud is unreachable.
    try {
      await fetchFromBase(CHINA_MIRROR, 'liaison.cloud');
    } catch (err) {
      warn(`liaison.cloud download failed: ${err.message}`);
      log('trying GitHub releases...');
      await fetchFromBase(GITHUB_BASE, 'github');
    }
  }

  // Now that the binary is in place, drop the agent skill files alongside it.
  installSkills();
}

// Suppress the unused `http` import warning — kept for future use if we
// ever need HTTP-not-HTTPS downloads (CI staging).
void http;

main().catch((err) => {
  die(`download failed: ${err.message}. You can retry with \`npm rebuild @liaisonio/cli\`, or set HTTPS_PROXY if you're behind a corporate proxy.`);
});
