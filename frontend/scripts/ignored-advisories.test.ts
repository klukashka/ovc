import { describe, expect, test } from 'vitest';

import {
  ignoredAdvisoryProblems,
  packageAuditProblems,
  parseIgnoredGhsas,
  parseIgnoredVulnerabilities,
} from './ignored-advisories.ts';

const workspace = `allowBuilds: {}
strictDepBuilds: true
minimumReleaseAge: 20160
auditConfig:
  ignoreGhsas:
    - GHSA-mrhx-c7xc-m3w5
    - GHSA-xjj6-f33q-929c
`;

const osvEntries = `[[IgnoredVulns]]
id = "GHSA-mrhx-c7xc-m3w5"
ignoreUntil = 2026-10-27T00:00:00Z
reason = "Reached only through a development dependency."

[[IgnoredVulns]]
id = "GHSA-xjj6-f33q-929c"
ignoreUntil = 2026-10-31T00:00:00Z
reason = "Reached only through the unit-test DOM."
`;

const packageJson = '{\n  "name": "fixture",\n  "scripts": { "lint": "node scripts/check-ignored-advisories.ts" }\n}\n';

const beforeEveryExpiry = new Date('2026-10-25T12:00:00Z');

describe('ignored advisory check', () => {
  test('accepts ignored advisories that each have a current OSV entry', () => {
    const problems = ignoredAdvisoryProblems(workspace, osvEntries, packageJson, beforeEveryExpiry);

    expect(problems).toEqual([]);
  });

  test('reports an ignored advisory that has no OSV entry', () => {
    const withExtraAdvisory = workspace.replace(
      '    - GHSA-xjj6-f33q-929c\n',
      '    - GHSA-xjj6-f33q-929c\n    - GHSA-4qv8-2p9j-2685\n',
    );

    const problems = ignoredAdvisoryProblems(withExtraAdvisory, osvEntries, packageJson, beforeEveryExpiry);

    expect(problems).toEqual([
      'GHSA-4qv8-2p9j-2685 is ignored by pnpm audit but has no [[IgnoredVulns]] entry in osv-scanner.toml',
    ]);
  });

  test('reports an OSV entry whose ignoreUntil has passed, from the instant it passes', () => {
    const atFirstExpiry = new Date('2026-10-27T00:00:00Z');

    const problems = ignoredAdvisoryProblems(workspace, osvEntries, packageJson, atFirstExpiry);

    expect(problems).toEqual([
      'GHSA-mrhx-c7xc-m3w5 is ignored by pnpm audit but its osv-scanner.toml entry expired on 2026-10-27T00:00:00.000Z',
    ]);
  });

  test('reads no advisories from a workspace without auditConfig', () => {
    const advisories = parseIgnoredGhsas('allowBuilds: {}\nstrictDepBuilds: true\n');

    expect(advisories).toEqual([]);
  });

  test.each([
    ['another auditConfig key', 'auditConfig:\n  ignoreCves:\n    - CVE-2026-1\n'],
    ['an empty list', 'auditConfig:\n  ignoreGhsas:\nminimumReleaseAge: 1\n'],
    ['an entry that is not a GHSA identifier', 'auditConfig:\n  ignoreGhsas:\n    - CVE-2026-1\n'],
    ['a quoted entry', "auditConfig:\n  ignoreGhsas:\n    - 'GHSA-mrhx-c7xc-m3w5'\n"],
    [
      'a blank line inside the list',
      'auditConfig:\n  ignoreGhsas:\n    - GHSA-mrhx-c7xc-m3w5\n\n    - GHSA-xjj6-f33q-929c\n',
    ],
    ['an auditConfig key with trailing spaces', 'auditConfig:  \n  ignoreGhsas:\n    - GHSA-mrhx-c7xc-m3w5\n'],
    ['a quoted auditConfig key', "'auditConfig':\n  ignoreGhsas:\n    - GHSA-mrhx-c7xc-m3w5\n"],
    ['an inline auditConfig', 'auditConfig: { ignoreGhsas: [GHSA-mrhx-c7xc-m3w5] }\n'],
    [
      'a second auditConfig block',
      'auditConfig:\n  ignoreGhsas:\n    - GHSA-mrhx-c7xc-m3w5\nauditConfig:\n  ignoreGhsas:\n    - GHSA-xjj6-f33q-929c\n',
    ],
    [
      'ignoreGhsas outside auditConfig',
      'auditConfig:\n  ignoreGhsas:\n    - GHSA-mrhx-c7xc-m3w5\nignoreGhsas:\n  - GHSA-xjj6-f33q-929c\n',
    ],
  ])('refuses a workspace with %s', (_name, text) => {
    const parse = (): string[] => parseIgnoredGhsas(text);

    expect(parse).toThrow();
  });

  test('accepts trailing blank lines after the list', () => {
    const advisories = parseIgnoredGhsas(
      'auditConfig:\n  ignoreGhsas:\n    - GHSA-mrhx-c7xc-m3w5\n\n\nallowBuilds: {}\n',
    );

    expect(advisories).toEqual(['GHSA-mrhx-c7xc-m3w5']);
  });

  test('reports a package.json that holds pnpm.auditConfig', () => {
    const withAudit = '{\n  "pnpm": { "auditConfig": { "ignoreGhsas": ["GHSA-4qv8-2p9j-2685"] } }\n}\n';

    const problems = ignoredAdvisoryProblems(workspace, osvEntries, withAudit, beforeEveryExpiry);

    expect(problems).toEqual(packageAuditProblems(withAudit));
    expect(problems).toHaveLength(1);
  });

  test.each([
    ['an entry without ignoreUntil', '[[IgnoredVulns]]\nid = "GHSA-mrhx-c7xc-m3w5"\nreason = "x"\n'],
    [
      'an unknown key',
      '[[IgnoredVulns]]\nid = "GHSA-mrhx-c7xc-m3w5"\nignoreUntil = 2026-10-27T00:00:00Z\nreason = "x"\nfrom = "y"\n',
    ],
    [
      'an invalid date',
      '[[IgnoredVulns]]\nid = "GHSA-mrhx-c7xc-m3w5"\nignoreUntil = 2026-02-30T00:00:00Z\nreason = "x"\n',
    ],
    ['a key outside an entry', 'id = "GHSA-mrhx-c7xc-m3w5"\n'],
    ['a repeated key', '[[IgnoredVulns]]\nid = "a"\nid = "b"\nignoreUntil = 2026-10-27T00:00:00Z\nreason = "x"\n'],
  ])('refuses an OSV file with %s', (_name, text) => {
    const parse = (): unknown => parseIgnoredVulnerabilities(text);

    expect(parse).toThrow();
  });
});
