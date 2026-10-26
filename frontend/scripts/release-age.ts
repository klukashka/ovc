import { load } from 'js-yaml';

const minutesPerDay = 24 * 60;

export const developmentReleaseAge = 3 * minutesPerDay;
export const deployedReleaseAge = 14 * minutesPerDay;

const releaseAgeKey = 'minimumReleaseAge';
const exclusionKey = 'minimumReleaseAgeExclude';
const workspaceKeys: readonly string[] = [releaseAgeKey, exclusionKey];
const settingStem = 'minimumreleaseage';
const exactExclusionPattern = /^(@[a-z0-9._-]+\/)?[a-z0-9._-]+@\d+\.\d+\.\d+$/;
const unreadableNpmrcKeyPattern = /["'\\$]/;
const npmrcCommentPattern = /^[;#]/;
const configurationHookKeys: readonly string[] = ['pnpmfile', 'globalPnpmfile', 'configDependencies'];

export interface WorkspaceFolder {
  readonly folder: string;
  readonly workspaceYaml: string;
  readonly packageJson: string;
  readonly npmrc: string | undefined;
  readonly pnpmfiles: readonly string[];
}

type Fields = Readonly<Record<string, unknown>>;

interface WorkspaceAge {
  readonly workspace: string;
  readonly age: number;
}

interface Inspection {
  readonly problems: readonly string[];
  readonly age: WorkspaceAge | undefined;
}

function isFields(value: unknown): value is Fields {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function mentionsTheSetting(text: string): boolean {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '')
    .includes(settingStem);
}

function nestedSettingKeys(value: unknown, path: string): string[] {
  if (Array.isArray(value)) {
    return value.flatMap((item, index) => nestedSettingKeys(item, `${path}[${String(index)}]`));
  }
  if (!isFields(value)) {
    return [];
  }
  return Object.entries(value).flatMap(([key, item]) => [
    ...(mentionsTheSetting(key) ? [`${path}.${key}`] : []),
    ...nestedSettingKeys(item, `${path}.${key}`),
  ]);
}

function parseWorkspace(workspace: string, yaml: string): Fields | string {
  let parsed: unknown;
  try {
    parsed = load(yaml);
  } catch (error) {
    const reason = error instanceof Error ? (error.message.split('\n')[0] ?? '') : 'unknown error';
    return `${workspace} is not one valid YAML document without repeated keys: ${reason}`;
  }
  return isFields(parsed) ? parsed : `${workspace} is not a YAML mapping`;
}

function settingSpellingProblems(workspace: string, settings: Fields): string[] {
  const misspelled = Object.keys(settings)
    .filter((key) => mentionsTheSetting(key) && !workspaceKeys.includes(key))
    .map((key) => `${workspace}: ${key} is not one of ${workspaceKeys.join(' and ')} (ADR-0017)`);
  const nested = Object.entries(settings)
    .flatMap(([key, value]) => nestedSettingKeys(value, key))
    .map((path) => `${workspace}: the release age may be set only at the top level, not at ${path} (ADR-0017)`);
  const hooks = Object.keys(settings)
    .filter((key) => configurationHookKeys.includes(key))
    .map(
      (key) => `${workspace}: ${key} can change the release age outside this check, so it is not allowed (ADR-0017)`,
    );
  return [...misspelled, ...nested, ...hooks];
}

function pnpmfileProblems(folder: string, pnpmfiles: readonly string[]): string[] {
  return pnpmfiles.map(
    (name) => `${folder}/${name} can change the release age outside this check, so it is not allowed (ADR-0017)`,
  );
}

function exclusionProblems(workspace: string, settings: Fields, age: number): string[] {
  if (!(exclusionKey in settings)) {
    return [];
  }
  if (age === developmentReleaseAge) {
    return [`${workspace}: ${exclusionKey} is not allowed while the delay is three days (ADR-0017)`];
  }
  const exclusions = settings[exclusionKey];
  if (!Array.isArray(exclusions)) {
    return [`${workspace}: ${exclusionKey} must be a list of exact package versions (ADR-0017)`];
  }
  return exclusions
    .filter((entry) => typeof entry !== 'string' || !exactExclusionPattern.test(entry))
    .map(
      (entry) =>
        `${workspace}: the exclusion ${JSON.stringify(entry)} must name exactly one package version (ADR-0017)`,
    );
}

function inspectWorkspace(workspace: string, yaml: string): Inspection {
  const settings = parseWorkspace(workspace, yaml);
  if (typeof settings === 'string') {
    return { problems: [settings], age: undefined };
  }
  const spelling = settingSpellingProblems(workspace, settings);
  const age = settings[releaseAgeKey];
  if (typeof age !== 'number' || !Number.isSafeInteger(age)) {
    return {
      problems: [...spelling, `${workspace} must set ${releaseAgeKey} once, as a whole number of minutes`],
      age: undefined,
    };
  }
  return { problems: [...spelling, ...exclusionProblems(workspace, settings, age)], age: { workspace, age } };
}

function npmrcProblems(folder: string, npmrc: string | undefined): string[] {
  if (npmrc === undefined) {
    return [];
  }
  const keys = npmrc
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line !== '' && !npmrcCommentPattern.test(line))
    .map((line) => line.split('=', 1)[0] ?? '');
  const problems: string[] = [];
  if (keys.some((key) => mentionsTheSetting(key))) {
    problems.push(`${folder}/.npmrc sets the release age, which ADR-0017 allows only in pnpm-workspace.yaml`);
  }
  if (keys.some((key) => unreadableNpmrcKeyPattern.test(key))) {
    problems.push(`${folder}/.npmrc has a quoted, escaped or substituted key that the release-age check cannot read`);
  }
  return problems;
}

function packageJsonProblems(folder: string, packageJson: string): string[] {
  const path = `${folder}/package.json`;
  let manifest: unknown;
  try {
    manifest = JSON.parse(packageJson);
  } catch {
    return [`${path} is not valid JSON`];
  }
  if (!isFields(manifest)) {
    return [`${path} is not a JSON object`];
  }
  const pnpmField = manifest.pnpm;
  return pnpmField !== undefined && mentionsTheSetting(JSON.stringify(pnpmField))
    ? [`${path} sets the release age in its pnpm field, which ADR-0017 allows only in pnpm-workspace.yaml`]
    : [];
}

function ageProblems(ages: readonly WorkspaceAge[], deploymentExists: boolean): string[] {
  const problems = ages
    .filter((entry) => entry.age !== developmentReleaseAge && entry.age !== deployedReleaseAge)
    .map(
      (entry) =>
        `${entry.workspace}: ${releaseAgeKey} is ${String(entry.age)}, but ADR-0017 allows only ${String(developmentReleaseAge)} or ${String(deployedReleaseAge)}`,
    );
  if (new Set(ages.map((entry) => entry.age)).size > 1) {
    problems.push(
      `the workspaces disagree on ${releaseAgeKey}: ${ages.map((entry) => `${entry.workspace} ${String(entry.age)}`).join(', ')}`,
    );
  }
  if (deploymentExists) {
    problems.push(
      ...ages
        .filter((entry) => entry.age < deployedReleaseAge)
        .map(
          (entry) =>
            `${entry.workspace}: deployment configuration exists, so ${releaseAgeKey} must be ${String(deployedReleaseAge)} (ADR-0017)`,
        ),
    );
  }
  return problems;
}

export function releaseAgeProblems(folders: readonly WorkspaceFolder[], deploymentExists: boolean): string[] {
  const inspections = folders.map((entry) => ({
    entry,
    inspection: inspectWorkspace(`${entry.folder}/pnpm-workspace.yaml`, entry.workspaceYaml),
  }));
  const ages = inspections.flatMap(({ inspection }) => (inspection.age === undefined ? [] : [inspection.age]));
  return [
    ...ageProblems(ages, deploymentExists),
    ...inspections.flatMap(({ entry, inspection }) => [
      ...inspection.problems,
      ...npmrcProblems(entry.folder, entry.npmrc),
      ...pnpmfileProblems(entry.folder, entry.pnpmfiles),
      ...packageJsonProblems(entry.folder, entry.packageJson),
    ]),
  ];
}
