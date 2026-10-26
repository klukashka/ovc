const minutesPerDay = 24 * 60;

export const developmentReleaseAge = 3 * minutesPerDay;
export const deployedReleaseAge = 14 * minutesPerDay;

const releaseAgeLinePattern = /^minimumReleaseAge: (\d+)$/;

const exclusionKey = 'minimumReleaseAgeExclude';
const exactExclusionPattern = /^ {2}- '?(@[a-z0-9._-]+\/)?[a-z0-9._-]+@\d+\.\d+\.\d+'?$/;

function lines(workspaceYaml: string): string[] {
  return workspaceYaml.replace(/\n$/, '').split('\n');
}

export function parseReleaseAge(workspace: string, workspaceYaml: string): number {
  const values = lines(workspaceYaml).flatMap((line) => {
    const match = releaseAgeLinePattern.exec(line);
    return match === null ? [] : [Number(match[1])];
  });
  if (values.length !== 1 || values[0] === undefined) {
    throw new Error(`${workspace} must set minimumReleaseAge exactly once, as a whole number of minutes`);
  }
  return values[0];
}

function exclusionEntries(workspaceYaml: string): string[] | undefined {
  const all = lines(workspaceYaml);
  const start = all.findIndex((line) => line.includes(exclusionKey));
  if (start === -1) {
    return undefined;
  }
  const entries: string[] = [];
  for (let index = start + 1; index < all.length && (all[index] ?? '').startsWith('  '); index += 1) {
    entries.push(all[index] ?? '');
  }
  return entries;
}

function exclusionProblems(workspace: string, workspaceYaml: string, age: number): string[] {
  const entries = exclusionEntries(workspaceYaml);
  if (entries === undefined) {
    return [];
  }
  if (age === developmentReleaseAge) {
    return [`${workspace}: minimumReleaseAgeExclude is not allowed while the delay is three days (ADR-0017)`];
  }
  return entries
    .filter((entry) => !exactExclusionPattern.test(entry))
    .map((entry) => `${workspace}: the exclusion "${entry.trim()}" must name exactly one package version (ADR-0017)`);
}

export interface WorkspaceReleaseAge {
  readonly workspace: string;
  readonly yaml: string;
}

export function releaseAgeProblems(workspaces: readonly WorkspaceReleaseAge[], deploymentExists: boolean): string[] {
  const ages = workspaces.map((entry) => ({
    workspace: entry.workspace,
    age: parseReleaseAge(entry.workspace, entry.yaml),
    yaml: entry.yaml,
  }));
  const problems = ages
    .filter((entry) => entry.age !== developmentReleaseAge && entry.age !== deployedReleaseAge)
    .map(
      (entry) =>
        `${entry.workspace}: minimumReleaseAge is ${String(entry.age)}, but ADR-0017 allows only ${String(developmentReleaseAge)} or ${String(deployedReleaseAge)}`,
    );
  if (new Set(ages.map((entry) => entry.age)).size > 1) {
    problems.push(
      `the workspaces disagree on minimumReleaseAge: ${ages.map((entry) => `${entry.workspace} ${String(entry.age)}`).join(', ')}`,
    );
  }
  problems.push(...ages.flatMap((entry) => exclusionProblems(entry.workspace, entry.yaml, entry.age)));
  if (deploymentExists) {
    problems.push(
      ...ages
        .filter((entry) => entry.age < deployedReleaseAge)
        .map(
          (entry) =>
            `${entry.workspace}: deployment configuration exists, so minimumReleaseAge must be ${String(deployedReleaseAge)} (ADR-0017)`,
        ),
    );
  }
  return problems;
}
