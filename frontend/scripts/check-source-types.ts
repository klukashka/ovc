import { resolve } from 'node:path';

import { checkSourceTypes } from './source-types.ts';

const unlinted = checkSourceTypes(resolve(import.meta.dirname, '..'));
if (unlinted.length > 0) {
  process.stderr.write(
    `src/ may hold only .ts, .tsx and .css files, which the lint rules cover:\n${unlinted.join('\n')}\n`,
  );
  process.exit(1);
}
