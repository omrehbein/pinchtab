import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export function validateVersion(input, persisted) {
  const version = input.startsWith('v') ? input.slice(1) : input;
  const stable = /^(0|[1-9][0-9]*)[.](0|[1-9][0-9]*)[.](0|[1-9][0-9]*)$/;
  if (!stable.test(version) || version !== persisted.trim()) {
    throw new Error('Release version must be stable MAJOR.MINOR.PATCH and match VERSION');
  }
  return version;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  console.log(validateVersion(process.argv[2] ?? '', readFileSync('VERSION', 'utf8')));
}
