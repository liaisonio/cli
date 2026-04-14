#!/usr/bin/env node
//
// Postinstall hook for @liaison-cloud/cli.
//
// Downloads the platform-specific Go binary from the matching GitHub release
// and verifies its SHA256 against the published SHA256SUMS file. The binary is
// dropped at vendor/<liaison|liaison.exe> next to this script's parent.
//
// Skipped automatically when:
//   - LIAISON_CLI_SKIP_DOWNLOAD=1 (dev / CI scenarios that don't need the bin)
//   - The user is on an unsupported platform (we exit 0 + warn, NOT fail,
//     so npm install doesn't break for transitive deps)
//
// Network failures DO fail the install — silently shipping a broken package
// is worse than a clear error the user can retry.

'use strict';

const https = require('https');
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

const pkg = require('../package.json');
const VERSION = `v${pkg.version}`;
const REPO = 'liaison-cloud/cli';
const RELEASE_BASE = `https://github.com/${REPO}/releases/download/${VERSION}`;

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

// Followed-redirect HTTP GET that streams to a file.
function downloadToFile(targetUrl, outPath) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(outPath);
    function get(u, depth) {
      if (depth > 5) {
        reject(new Error(`too many redirects fetching ${targetUrl}`));
        return;
      }
      https
        .get(u, { headers: { 'User-Agent': 'liaison-cli-installer' } }, (res) => {
          if (
            res.statusCode &&
            res.statusCode >= 300 &&
            res.statusCode < 400 &&
            res.headers.location
          ) {
            res.resume();
            get(res.headers.location, depth + 1);
            return;
          }
          if (res.statusCode !== 200) {
            reject(new Error(`HTTP ${res.statusCode} for ${u}`));
            res.resume();
            return;
          }
          res.pipe(file);
          file.on('finish', () => file.close(() => resolve()));
          file.on('error', reject);
        })
        .on('error', reject);
    }
    get(targetUrl, 0);
  });
}

function downloadToString(targetUrl) {
  return new Promise((resolve, reject) => {
    function get(u, depth) {
      if (depth > 5) {
        reject(new Error(`too many redirects fetching ${targetUrl}`));
        return;
      }
      https
        .get(u, { headers: { 'User-Agent': 'liaison-cli-installer' } }, (res) => {
          if (
            res.statusCode &&
            res.statusCode >= 300 &&
            res.statusCode < 400 &&
            res.headers.location
          ) {
            res.resume();
            get(res.headers.location, depth + 1);
            return;
          }
          if (res.statusCode !== 200) {
            reject(new Error(`HTTP ${res.statusCode} for ${u}`));
            res.resume();
            return;
          }
          let body = '';
          res.setEncoding('utf8');
          res.on('data', (chunk) => (body += chunk));
          res.on('end', () => resolve(body));
        })
        .on('error', reject);
    }
    get(targetUrl, 0);
  });
}

function sha256(filepath) {
  const hash = crypto.createHash('sha256');
  hash.update(fs.readFileSync(filepath));
  return hash.digest('hex');
}

async function main() {
  log(`fetching ${url}`);
  await downloadToFile(url, destPath);
  fs.chmodSync(destPath, 0o755);

  log('verifying SHA256');
  const sums = await downloadToString(sumsUrl);
  const line = sums
    .split('\n')
    .map((l) => l.trim())
    .find((l) => l.endsWith(filename));
  if (!line) {
    fs.unlinkSync(destPath);
    die(`no SHA256 entry for ${filename} in SHA256SUMS — release may be corrupt`);
  }
  const expected = line.split(/\s+/)[0];
  const actual = sha256(destPath);
  if (expected !== actual) {
    fs.unlinkSync(destPath);
    die(`SHA256 mismatch for ${filename}: expected ${expected}, got ${actual}`);
  }
  log(`installed ${filename} (sha256 ok)`);
}

main().catch((err) => {
  die(`download failed: ${err.message}`);
});
