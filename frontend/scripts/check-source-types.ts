import { resolve } from 'node:path';

import { checkSourceTypes } from './source-types.ts';

const unlinted = checkSourceTypes(resolve(import.meta.dirname, '..'));
if (unlinted.length > 0) {
  process.stderr.write(
    `src/ may hold only .ts, .tsx and .css files and public/ only linted .js files and static assets:\n${unlinted.join('\n')}\n`,
  );
  process.exit(1);
}
