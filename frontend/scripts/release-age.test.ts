import { describe, expect, test } from 'vitest';

import { type WorkspaceFolder, deployedReleaseAge, developmentReleaseAge, releaseAgeProblems } from './release-age.ts';

function workspaceWith(age: number): string {
  return `allowBuilds: {}\nstrictDepBuilds: true\nminimumReleaseAge: ${String(age)}\n`;
}

const plainPackageJson = '{"name": "ovc-frontend", "private": true}';

function frontendWith(workspaceYaml: string, files: Partial<WorkspaceFolder> = {}): WorkspaceFolder {
  return {
    folder: 'frontend',
    workspaceYaml,
    packageJson: plainPackageJson,
    npmrc: undefined,
    pnpmfiles: [],
    ...files,
  };
}

function bothWorkspaces(frontendAge: number, docsAge: number): WorkspaceFolder[] {
  return [
    frontendWith(workspaceWith(frontendAge)),
    {
      folder: 'tools/docs',
      workspaceYaml: workspaceWith(docsAge),
      packageJson: plainPackageJson,
      npmrc: undefined,
      pnpmfiles: [],
    },
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

  test('refuses a workspace without a whole number of minutes', () => {
    const workspaces = [frontendWith('allowBuilds: {}\n'), frontendWith(`minimumReleaseAge: '4320'\n`)];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml must set minimumReleaseAge once, as a whole number of minutes',
      'frontend/pnpm-workspace.yaml must set minimumReleaseAge once, as a whole number of minutes',
    ]);
  });

  test('refuses a repeated key, also when one spelling is quoted', () => {
    const workspaces = [frontendWith(`${workspaceWith(developmentReleaseAge)}"minimumReleaseAge": 0\n`)];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml is not one valid YAML document without repeated keys: duplicated mapping key (4:1)',
    ]);
  });

  test('reads the parsed value, not a decoy line inside a quoted string', () => {
    const decoy = `allowBuilds: {}\nnotes: "kept for reference\nminimumReleaseAge: 4320\nuntil the next review"\n'minimumReleaseAge': 0\n`;
    const workspaces = [frontendWith(decoy)];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: minimumReleaseAge is 0, but ADR-0017 allows only 4320 or 20160',
    ]);
  });

  test('refuses another spelling of the setting and a nested one', () => {
    const workspaces = [
      frontendWith(
        `${workspaceWith(developmentReleaseAge)}minimum-release-age: 0\npackageConfigs:\n  ovc:\n    minimumReleaseAge: 0\n`,
      ),
    ];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: minimum-release-age is not one of minimumReleaseAge and minimumReleaseAgeExclude (ADR-0017)',
      'frontend/pnpm-workspace.yaml: the release age may be set only at the top level, not at packageConfigs.ovc.minimumReleaseAge (ADR-0017)',
    ]);
  });

  test('refuses any exclusion while the delay is three days, an empty one included', () => {
    const workspaces = [
      frontendWith(`${workspaceWith(developmentReleaseAge)}minimumReleaseAgeExclude:\n  - dompurify@3.4.17\n`),
      frontendWith(`${workspaceWith(developmentReleaseAge)}minimumReleaseAgeExclude: []\n`),
    ];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: minimumReleaseAgeExclude is not allowed while the delay is three days (ADR-0017)',
      'frontend/pnpm-workspace.yaml: minimumReleaseAgeExclude is not allowed while the delay is three days (ADR-0017)',
    ]);
  });

  test('accepts only exact package versions as exclusions at fourteen days', () => {
    const workspaces = [
      frontendWith(
        `${workspaceWith(deployedReleaseAge)}minimumReleaseAgeExclude:\n  - dompurify@3.4.17\n  - '@scope/name@1.2.3'\n  - dompurify\n  - '*'\n`,
      ),
    ];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: the exclusion "dompurify" must name exactly one package version (ADR-0017)',
      'frontend/pnpm-workspace.yaml: the exclusion "*" must name exactly one package version (ADR-0017)',
    ]);
  });

  test('refuses a broad exclusion written as a flow list', () => {
    const workspaces = [frontendWith(`${workspaceWith(deployedReleaseAge)}minimumReleaseAgeExclude: [dompurify]\n`)];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: the exclusion "dompurify" must name exactly one package version (ADR-0017)',
    ]);
  });

  test('refuses a broad exclusion written as an unindented list', () => {
    const workspaces = [frontendWith(`${workspaceWith(deployedReleaseAge)}minimumReleaseAgeExclude:\n- '*'\n`)];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: the exclusion "*" must name exactly one package version (ADR-0017)',
    ]);
  });

  test('refuses an exclusion that is not a list', () => {
    const workspaces = [frontendWith(`${workspaceWith(deployedReleaseAge)}minimumReleaseAgeExclude: dompurify\n`)];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: minimumReleaseAgeExclude must be a list of exact package versions (ADR-0017)',
    ]);
  });

  test('refuses the setting in an .npmrc, and a key it cannot read', () => {
    const workspaces = [
      frontendWith(workspaceWith(developmentReleaseAge), {
        npmrc: '; registry settings\nminimum-release-age=0\n"minimum\\u002drelease-age"=0\n',
      }),
    ];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/.npmrc sets the release age, which ADR-0017 allows only in pnpm-workspace.yaml',
      'frontend/.npmrc has a quoted, escaped or substituted key that the release-age check cannot read',
    ]);
  });

  test('accepts an .npmrc that does not touch the release age', () => {
    const workspaces = [
      frontendWith(workspaceWith(developmentReleaseAge), { npmrc: '# no release age here\nengine-strict=true\n' }),
    ];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([]);
  });

  test('refuses the setting in the pnpm field of package.json, also behind an escape', () => {
    const workspaces = [
      frontendWith(workspaceWith(developmentReleaseAge), {
        packageJson: '{"name": "ovc-frontend", "pnpm": {"minimum\\u0052eleaseAge": 0}}',
      }),
    ];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/package.json sets the release age in its pnpm field, which ADR-0017 allows only in pnpm-workspace.yaml',
    ]);
  });

  test('refuses a pnpmfile and the settings that load configuration hooks', () => {
    const workspaces = [
      frontendWith(
        `${workspaceWith(developmentReleaseAge)}pnpmfile: hooks.cjs\nconfigDependencies:\n  hooks: 1.0.0\n`,
        {
          pnpmfiles: ['.pnpmfile.cjs'],
        },
      ),
    ];

    const problems = releaseAgeProblems(workspaces, false);

    expect(problems).toEqual([
      'frontend/pnpm-workspace.yaml: pnpmfile can change the release age outside this check, so it is not allowed (ADR-0017)',
      'frontend/pnpm-workspace.yaml: configDependencies can change the release age outside this check, so it is not allowed (ADR-0017)',
      'frontend/.pnpmfile.cjs can change the release age outside this check, so it is not allowed (ADR-0017)',
    ]);
  });
});
