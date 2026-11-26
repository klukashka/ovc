import { readFileSync, realpathSync, statSync } from 'node:fs';
import { resolve } from 'node:path';

import { legalDocumentNames, legalLanguages } from '../src/legal/document.ts';
import { type LegalTextSource, legalTextProblems } from './legal-texts.ts';

const frontendRoot = resolve(import.meta.dirname, '..');
const repositoryRoot = resolve(frontendRoot, '..');

function exists(path: string): boolean {
  return statSync(path, { throwIfNoEntry: false }) !== undefined;
}

const texts: LegalTextSource[] = legalLanguages.flatMap((language) =>
  legalDocumentNames.map((name) => {
    const path = resolve(frontendRoot, 'src', 'legal', language, `${name}.md`);
    return { language, name, source: exists(path) ? readFileSync(path, 'utf8') : undefined };
  }),
);

const problems = legalTextProblems(texts, {
  deploymentExists: statSync(resolve(repositoryRoot, 'deploy'), { throwIfNoEntry: false })?.isDirectory() === true,
  today: new Date().toISOString().slice(0, 10),
  reviewRecordIdentity: (path) => {
    const absolute = resolve(repositoryRoot, path);
    return statSync(absolute, { throwIfNoEntry: false })?.isFile() === true
      ? realpathSync(absolute).toLowerCase()
      : null;
  },
});

if (problems.length > 0) {
  process.stderr.write(`The legal texts break their rules:\n${problems.join('\n')}\n`);
  process.exitCode = 1;
}
