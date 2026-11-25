import { type ActionState, refusedBy } from '../../components/ui/action.ts';
import { settledRequest } from '../../components/ui/settled.ts';
import type { ApiClient } from '../../lib/api/client.ts';
import type { LibraryTarget } from './LibraryDialogs.tsx';

export interface Moved {
  readonly target: LibraryTarget;
  readonly from: string | null;
  readonly to: string | null;
  readonly version: number | null;
}

export type Placement =
  | { readonly kind: 'placed'; readonly folder: string | null; readonly version: number | null }
  | { readonly kind: 'gone' }
  | { readonly kind: 'unknown' };

export function sendMove(
  client: ApiClient,
  target: LibraryTarget,
  destination: string | null,
  version: number | null,
): Promise<number | null | ActionState> {
  return settledRequest(async (): Promise<number | null | ActionState> => {
    if (target.kind === 'folder') {
      const answer = await client.PATCH('/api/v1/library/folders/{id}', {
        params: { path: { id: target.folder.id } },
        body: { parent_id: destination },
      });
      return answer.data === undefined ? refusedBy(answer.response, answer.error) : null;
    }
    if (target.item.kind === 'material') {
      const answer = await client.PATCH('/api/v1/library/materials/{id}', {
        params: { path: { id: target.item.id } },
        body: { folder_id: destination },
      });
      return answer.data === undefined ? refusedBy(answer.response, answer.error) : null;
    }
    const answer = await client.PATCH('/api/v1/library/templates/{id}', {
      params: { path: { id: target.item.id } },
      body: { folder_id: destination, version: version ?? 0 },
    });
    return answer.data === undefined ? refusedBy(answer.response, answer.error) : answer.data.version;
  });
}

export async function placementOf(client: ApiClient, target: LibraryTarget): Promise<Placement> {
  try {
    if (target.kind === 'folder') {
      const answer = await client.GET('/api/v1/library/folders');
      if (answer.data === undefined) {
        return { kind: 'unknown' };
      }
      const found = answer.data.folders.find((folder) => folder.id === target.folder.id);
      return found === undefined ? { kind: 'gone' } : { kind: 'placed', folder: found.parent_id, version: null };
    }
    if (target.item.kind === 'material') {
      return { kind: 'unknown' };
    }
    const answer = await client.GET('/api/v1/library/templates/{id}', {
      params: { path: { id: target.item.id } },
    });
    if (answer.data === undefined) {
      return answer.response.status === 404 ? { kind: 'gone' } : { kind: 'unknown' };
    }
    return { kind: 'placed', folder: answer.data.folder_id, version: answer.data.version };
  } catch {
    return { kind: 'unknown' };
  }
}
