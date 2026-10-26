import { describe, expect, test } from 'vitest';

import { deployedReleaseAge, developmentReleaseAge, releaseAgeProblems } from './release-age.ts';

function workspaceWith(age: number): string {
  return `allowBuilds: {}\nstrictDepBuilds: true\nminimumReleaseAge: ${String(age)}\n`;
}

function bothWorkspaces(frontendAge: number, docsAge: number): { workspace: string; yaml: string }[] {
  return [
    { workspace: 'frontend/pnpm-workspace.yaml', yaml: workspaceWith(frontendAge) },
    { workspace: 'tools/docs/pnpm-workspace.yaml', yaml: workspaceWith(docsAge) },
  ];
}

describe('dependency release age of ADR-0017', () => {
  test('accepts three days in both workspaces while nothing is deployed', () => {
    const workspaces = bothWorkspaces(developmentReleaseAge, developmentReleaseAge);

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([]);
  });

  test('accepts fourteen days in both workspaces once deployment configuration exists', () => {
    const workspaces = bothWorkspaces(deployedReleaseAge, deployedReleaseAge);

    const problems = releaseAgeProblems(workspaces, true);

    expect(problems).toEqual([]);
  });

  test('refuses three days once deployment configuration exists', () => {
    const workspaces = bothWorkspaces(developmentReleaseAge, developmentReleaseAge);

    const problems = releaseAgeProblems(workspaces, true);

    expect(problems).toHaveLength(2);
  });

  test('refuses workspaces that disagree', () => {
    const workspaces = bothWorkspaces(developmentReleaseAge, deployedReleaseAge);

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'the workspaces disagree on minimumReleaseAge: frontend/pnpm-workspace.yaml 4320, tools/docs/pnpm-workspace.yaml 20160',
    ]);
  });

  test('refuses any other value, zero included', () => {
    const workspaces = bothWorkspaces(0, 0);

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toHaveLength(2);
  });

  test('refuses a workspace without exactly one minimumReleaseAge', () => {
    const workspaces = [{ workspace: 'frontend/pnpm-workspace.yaml', yaml: 'allowBuilds: {}\n' }];

    const check = (): string[] => releaseAgeProblems(workspaces, false);

    expect(check).toThrow(/frontend\/pnpm-workspace.yaml must set minimumReleaseAge exactly once/);
  });

  test('refuses any exclusion while the delay is three days', () => {
    const workspaces = [
      {
        workspace: 'frontend/pnpm-workspace.yaml',
        yaml: `${workspaceWith(developmentReleaseAge)}minimumReleaseAgeExclude:\n  - dompurify@3.4.17\n`,
      },
    ];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: minimumReleaseAgeExclude is not allowed while the delay is three days (ADR-0017)',
    ]);
  });

  test('accepts only exact package versions as exclusions at fourteen days', () => {
    const workspaces = [
      {
        workspace: 'frontend/pnpm-workspace.yaml',
        yaml: `${workspaceWith(deployedReleaseAge)}minimumReleaseAgeExclude:\n  - dompurify@3.4.17\n  - '@scope/name@1.2.3'\n  - dompurify\n  - '*'\n`,
      },
    ];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: the exclusion "- dompurify" must name exactly one package version (ADR-0017)',
      `frontend/pnpm-workspace.yaml: the exclusion "- '*'" must name exactly one package version (ADR-0017)`,
    ]);
  });
});
