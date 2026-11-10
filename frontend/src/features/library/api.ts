import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

export type Library = components['schemas']['Library'];
export type LibraryFolder = components['schemas']['LibraryFolder'];
export type LibraryItem = components['schemas']['LibraryItem'];
export type LibraryPathEntry = components['schemas']['LibraryPathEntry'];
export type UploadedFile = components['schemas']['UploadedFile'];

export const libraryKey = ['library'] as const;

export const libraryFoldersKey = [...libraryKey, 'folders'] as const;

export const librarySummaryKey = [...libraryKey, 'summary'] as const;

export function libraryListKey(folder: string | null, search: string | null): readonly unknown[] {
  return [...libraryKey, 'list', folder, search];
}

export const titleLimit = 200;

export const folderNameLimit = 100;

export const libraryPageSize = 30;

export interface LibraryView {
  readonly folder: string | null;
  readonly search: string | null;
}

export async function fetchLibraryPage(
  client: ApiClient,
  view: LibraryView,
  cursor: string | null,
  signal: AbortSignal,
): Promise<Library> {
  const { data, error, response } = await client.GET('/api/v1/library', {
    params: {
      query: {
        ...(view.search === null ? {} : { q: view.search }),
        ...(view.search === null && view.folder !== null ? { folder: view.folder } : {}),
        ...(cursor === null ? {} : { cursor }),
      },
    },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export async function fetchTemplateCount(client: ApiClient, signal: AbortSignal): Promise<number> {
  const { data, error, response } = await client.GET('/api/v1/library', {
    params: { query: { limit: 1 } },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data.template_count;
}

export async function fetchFolders(client: ApiClient, signal: AbortSignal): Promise<readonly LibraryFolder[]> {
  const { data, error, response } = await client.GET('/api/v1/library/folders', { signal });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data.folders;
}

export const searchLimit = 200;

export const libraryUploadLimitBytes = 100_000_000;

export const webpUploadLimitBytes = 8_388_608;

export function searchTerm(typed: string): string | null {
  const trimmed = typed.trim();
  return trimmed.normalize('NFKC').trim() === '' ? null : cutToCodePoints(trimmed, searchLimit);
}

export function withoutExtension(filename: string): string {
  const dot = filename.lastIndexOf('.');
  return dot > 0 ? filename.slice(0, dot) : filename;
}

export function cutToCodePoints(text: string, limit: number): string {
  return Array.from(text).slice(0, limit).join('');
}

export interface FolderNode {
  readonly folder: LibraryFolder;
  readonly depth: number;
  readonly path: readonly LibraryFolder[];
}

export function folderTree(folders: readonly LibraryFolder[]): readonly FolderNode[] {
  const children = new Map<string | null, LibraryFolder[]>();
  for (const folder of folders) {
    const siblings = children.get(folder.parent_id) ?? [];
    siblings.push(folder);
    children.set(folder.parent_id, siblings);
  }
  const nodes: FolderNode[] = [];
  const visit = (parent: string | null, path: readonly LibraryFolder[]): void => {
    for (const folder of children.get(parent) ?? []) {
      const ownPath = [...path, folder];
      nodes.push({ folder, depth: ownPath.length, path: ownPath });
      visit(folder.id, ownPath);
    }
  };
  visit(null, []);
  return nodes;
}

export function subtreeOf(nodes: readonly FolderNode[], folderId: string): ReadonlySet<string> {
  return new Set(
    nodes.filter((node) => node.path.some((ancestor) => ancestor.id === folderId)).map((node) => node.folder.id),
  );
}
