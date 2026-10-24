import type { components } from '../../lib/api/schema.ts';

export type ReclaimList = components['schemas']['ReclaimList'];
export type ReclaimChoice = components['schemas']['ReclaimChoice'];

export type Decision = 'keep' | 'drop';

export const decisions: readonly Decision[] = ['keep', 'drop'];

export function linkField(relationshipId: string): string {
  return `link:${relationshipId}`;
}

export function requestField(requestId: string): string {
  return `request:${requestId}`;
}

function decided(
  ids: readonly string[],
  field: (id: string) => string,
  decisionOf: (field: string) => string | null,
): { readonly keep: string[]; readonly drop: string[] } | null {
  const keep: string[] = [];
  const drop: string[] = [];
  for (const id of new Set(ids)) {
    const decision = decisionOf(field(id));
    if (decision === 'keep') {
      keep.push(id);
    } else if (decision === 'drop') {
      drop.push(id);
    } else {
      return null;
    }
  }
  return { keep, drop };
}

export function reclaimChoice(list: ReclaimList, decisionOf: (field: string) => string | null): ReclaimChoice | null {
  const links = decided(
    list.links.map((link) => link.relationship_id),
    linkField,
    decisionOf,
  );
  const requests = decided(
    list.requests.map((request) => request.request_id),
    requestField,
    decisionOf,
  );
  if (links === null || requests === null) {
    return null;
  }
  return {
    keep_links: links.keep,
    drop_links: links.drop,
    keep_requests: requests.keep,
    drop_requests: requests.drop,
  };
}
