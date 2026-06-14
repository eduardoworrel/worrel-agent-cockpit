#!/usr/bin/env node
'use strict';
const { spawnSync } = require('child_process');

// mapeia "<platform>-<arch>" do Node -> pacote npm da plataforma
const PKG = {
  'darwin-arm64': '@worrel/darwin-arm64',
  'darwin-x64': '@worrel/darwin-x64',
  'linux-x64': '@worrel/linux-x64',
  'linux-arm64': '@worrel/linux-arm64',
};

function binaryPath() {
  const pkg = PKG[`${process.platform}-${process.arch}`];
  if (!pkg) return null;
  const exe = process.platform === 'win32' ? 'worrel.exe' : 'worrel';
  try {
    return require.resolve(`${pkg}/bin/${exe}`);
  } catch (_) {
    return null;
  }
}

const bin = binaryPath();
if (!bin) {
  console.error(
    `worrel: plataforma não suportada (${process.platform}-${process.arch}).\n` +
    `Suportadas: ${Object.keys(PKG).join(', ')}.`
  );
  process.exit(1);
}

// spawnSync com stdio herdado encaminha sinais (Ctrl-C) ao binário naturalmente.
const res = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });
if (res.error) {
  console.error(`worrel: falha ao executar o binário: ${res.error.message}`);
  process.exit(1);
}
process.exit(res.status === null ? 1 : res.status);
