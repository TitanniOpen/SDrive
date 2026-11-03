import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';

export function verifyHelpLinkBoundaries(binary, links) {
  for (const address of new Set(Object.values(links))) {
    const needle = Buffer.from(address);
    let count = 0;
    for (let at = binary.indexOf(needle); at !== -1; at = binary.indexOf(needle, at + needle.length)) {
      count++;
      if (binary[at - 1] !== 34 || binary[at + needle.length] !== 34) {
        throw new Error(`Help URL lacks JSON boundaries: ${address} at ${at}`);
      }
    }
    if (!count) throw new Error(`Help URL missing from artifact: ${address}`);
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const links = JSON.parse(readFileSync(new URL('../internal/core/help_links.json', import.meta.url)));
  if (!process.argv[2]) throw new Error('Provide the application artifact path');
  for (const path of process.argv.slice(2)) {
    verifyHelpLinkBoundaries(readFileSync(path), links);
    console.log(`PASS help URL boundaries: ${path}`);
  }
}
