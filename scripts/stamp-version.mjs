// Estampa a versão (argv[2]) nos package.json de npm/ e fixa as
// optionalDependencies do pacote principal na versão exata.
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const version = process.argv[2];
if (!version) {
  console.error('uso: node scripts/stamp-version.mjs <versão>');
  process.exit(1);
}

const root = join(dirname(fileURLToPath(import.meta.url)), '..', 'npm');
const platforms = ['darwin-arm64', 'darwin-x64', 'linux-x64', 'linux-arm64'];

function patch(file, fn) {
  const pkg = JSON.parse(readFileSync(file, 'utf8'));
  fn(pkg);
  writeFileSync(file, JSON.stringify(pkg, null, 2) + '\n');
}

for (const p of platforms) {
  patch(join(root, 'platforms', p, 'package.json'), (pkg) => {
    pkg.version = version;
  });
}

patch(join(root, 'worrel', 'package.json'), (pkg) => {
  pkg.version = version;
  for (const p of platforms) {
    pkg.optionalDependencies[`@worrel/${p}`] = version;
  }
});

console.log(`versão ${version} estampada em ${platforms.length + 1} pacotes`);
