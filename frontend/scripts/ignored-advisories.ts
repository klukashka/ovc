export interface IgnoredVulnerability {
  readonly id: string;
  readonly ignoreUntil: Date;
}

const advisoryPattern = /^GHSA(-[23456789cfghjmpqrvwx]{4}){3}$/;
const ignoreGhsaLinePattern = /^ {4}- (\S+)$/;
const stringKeyPattern = /^(id|reason) = "([^"\\]*)"$/;
const dateKeyPattern = /^ignoreUntil = (\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)$/;

function lines(text: string): string[] {
  return text.replace(/\n$/, '').split('\n');
}

const auditKeyPattern = /auditConfig|ignoreGhsas/;

function refuseAuditKeysOutside(all: readonly string[], blockStart: number, blockEnd: number): void {
  all.forEach((line, index) => {
    if ((index < blockStart || index >= blockEnd) && auditKeyPattern.test(line)) {
      throw new Error(
        `line ${String(index + 1)} names auditConfig or ignoreGhsas outside the one auditConfig.ignoreGhsas list the check reads`,
      );
    }
  });
}

export function parseIgnoredGhsas(workspaceYaml: string): string[] {
  const all = lines(workspaceYaml);
  const start = all.indexOf('auditConfig:');
  if (start === -1) {
    refuseAuditKeysOutside(all, 0, 0);
    return [];
  }
  if (all[start + 1] !== '  ignoreGhsas:') {
    throw new Error('auditConfig must hold only an ignoreGhsas list');
  }
  const advisories: string[] = [];
  let end = start + 2;
  let blankSeen = false;
  for (; end < all.length; end += 1) {
    const line = all[end] ?? '';
    if (line === '') {
      blankSeen = true;
      continue;
    }
    if (!line.startsWith(' ')) {
      break;
    }
    if (blankSeen) {
      throw new Error(`auditConfig.ignoreGhsas holds a blank line before line ${String(end + 1)}`);
    }
    const advisory = ignoreGhsaLinePattern.exec(line)?.[1];
    if (advisory === undefined || !advisoryPattern.test(advisory)) {
      throw new Error(`auditConfig.ignoreGhsas holds a line that is not a GHSA identifier: ${line}`);
    }
    advisories.push(advisory);
  }
  if (advisories.length === 0) {
    throw new Error('auditConfig.ignoreGhsas is empty; remove auditConfig instead');
  }
  refuseAuditKeysOutside(all, start, end);
  return advisories;
}

export function packageAuditProblems(packageJson: string): string[] {
  return auditKeyPattern.test(packageJson)
    ? ['package.json names auditConfig or ignoreGhsas; pnpm audit ignores belong only in pnpm-workspace.yaml']
    : [];
}

interface PartialEntry {
  id?: string;
  ignoreUntil?: Date;
  reason?: string;
}

function completeEntry(entry: PartialEntry, line: number): IgnoredVulnerability {
  if (entry.id === undefined || entry.ignoreUntil === undefined || entry.reason === undefined) {
    throw new Error(`the [[IgnoredVulns]] entry ending before line ${String(line)} needs id, ignoreUntil and reason`);
  }
  return { id: entry.id, ignoreUntil: entry.ignoreUntil };
}

function parseDate(text: string, line: number): Date {
  const date = new Date(text);
  if (Number.isNaN(date.getTime()) || date.toISOString().replace('.000Z', 'Z') !== text) {
    throw new Error(`line ${String(line)} holds an invalid ignoreUntil date`);
  }
  return date;
}

export function parseIgnoredVulnerabilities(osvToml: string): IgnoredVulnerability[] {
  const entries: IgnoredVulnerability[] = [];
  let current: PartialEntry | undefined;
  lines(osvToml).forEach((line, index) => {
    const lineNumber = index + 1;
    if (line === '') {
      return;
    }
    if (line === '[[IgnoredVulns]]') {
      if (current !== undefined) {
        entries.push(completeEntry(current, lineNumber));
      }
      current = {};
      return;
    }
    if (current === undefined) {
      throw new Error(`line ${String(lineNumber)} is outside an [[IgnoredVulns]] entry`);
    }
    const stringKey = stringKeyPattern.exec(line);
    const dateKey = dateKeyPattern.exec(line);
    if (stringKey?.[1] === 'id' && current.id === undefined) {
      current.id = stringKey[2] ?? '';
    } else if (stringKey?.[1] === 'reason' && current.reason === undefined) {
      current.reason = stringKey[2] ?? '';
    } else if (dateKey?.[1] !== undefined && current.ignoreUntil === undefined) {
      current.ignoreUntil = parseDate(dateKey[1], lineNumber);
    } else {
      throw new Error(`line ${String(lineNumber)} is not an expected id, ignoreUntil or reason key`);
    }
  });
  if (current !== undefined) {
    entries.push(completeEntry(current, lines(osvToml).length + 1));
  }
  return entries;
}

export function ignoredAdvisoryProblems(
  workspaceYaml: string,
  osvToml: string,
  packageJson: string,
  now: Date,
): string[] {
  const vulnerabilities = parseIgnoredVulnerabilities(osvToml);
  const workspaceProblems = parseIgnoredGhsas(workspaceYaml).flatMap((advisory) => {
    const entry = vulnerabilities.find((vulnerability) => vulnerability.id === advisory);
    if (entry === undefined) {
      return [`${advisory} is ignored by pnpm audit but has no [[IgnoredVulns]] entry in osv-scanner.toml`];
    }
    if (entry.ignoreUntil.getTime() <= now.getTime()) {
      return [
        `${advisory} is ignored by pnpm audit but its osv-scanner.toml entry expired on ${entry.ignoreUntil.toISOString()}`,
      ];
    }
    return [];
  });
  return [...packageAuditProblems(packageJson), ...workspaceProblems];
}
