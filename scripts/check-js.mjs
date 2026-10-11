import {readdirSync} from 'node:fs';
import {join} from 'node:path';
import {spawnSync} from 'node:child_process';

for (const directory of ['internal/httpapi/assets', 'scripts', 'sdk']) {
  for (const file of readdirSync(directory).sort()) {
    if (!/\.(?:js|mjs)$/.test(file)) continue;
    const path = join(directory, file);
    const result = spawnSync(process.execPath, ['--check', path], {stdio: 'inherit'});
    if (result.error) throw result.error;
    if (result.status !== 0) process.exit(result.status ?? 1);
  }
}
