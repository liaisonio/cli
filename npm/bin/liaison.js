#!/usr/bin/env node
//
// Entry point listed in package.json "bin". This file is what `npx
// @liaisonio/cli ...` and the symlinked `liaison` command both call.
//
// All it does is locate the platform-specific Go binary that was downloaded
// during `postinstall` (see scripts/install.js) and exec it, forwarding
// argv, stdio, and the exit code.
//
// We deliberately do NOT use require('child_process').execFile or .exec here
// because they buffer stdout/stderr — the CLI prints progress lines for
// `liaison login`, and we want the user to see them in real time.

'use strict';

const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');

const PLATFORM_BINARY = {
  'darwin-arm64': 'liaison',
  'darwin-x64': 'liaison',
  'linux-arm64': 'liaison',
  'linux-x64': 'liaison',
  'win32-x64': 'liaison.exe',
};

function fail(msg, code) {
  process.stderr.write(`liaison-cli: ${msg}\n`);
  process.exit(code || 1);
}

const key = `${process.platform}-${process.arch}`;
const binaryName = PLATFORM_BINARY[key];
if (!binaryName) {
  fail(
    `unsupported platform ${key}. Supported: ${Object.keys(PLATFORM_BINARY).join(', ')}`,
  );
}

const binaryPath = path.join(__dirname, '..', 'vendor', binaryName);
if (!fs.existsSync(binaryPath)) {
  fail(
    `binary not found at ${binaryPath}.\n` +
      'This usually means the postinstall download was skipped or failed.\n' +
      'Try: npm rebuild @liaisonio/cli  (or `npm install` again with --foreground-scripts)',
  );
}

const result = spawnSync(binaryPath, process.argv.slice(2), {
  stdio: 'inherit',
  windowsHide: true,
});

if (result.error) {
  fail(`failed to run binary: ${result.error.message}`);
}
process.exit(result.status === null ? 1 : result.status);
