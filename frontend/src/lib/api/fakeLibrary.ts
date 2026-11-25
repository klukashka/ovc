import { fakeRefusal } from './fakeSubmissions.ts';
import type { components } from './schema.ts';

type Library = components['schemas']['Library'];
type LibraryFolder = components['schemas']['LibraryFolder'];
type LibraryItem = components['schemas']['LibraryItem'];
type LibraryPathEntry = components['schemas']['LibraryPathEntry'];
type Template = components['schemas']['Template'];

export interface FakeLibrarySeed {
  readonly folderName?: string;
  readonly childFolderName?: string;
  readonly templateTitle?: string;
  readonly templateVersion?: number;
  readonly materialTitle?: string;
}

export interface SeededFakeLibrary {
  readonly folderId: string;
  readonly childFolderId: string;
  readonly templateId: string;
  readonly templateVersion: number;
  readonly materialId: string;
}

interface OwnedFolder {
  readonly tutorId: string;
  readonly folder: LibraryFolder;
}

interface OwnedMaterial {
  readonly tutorId: string;
  readonly item: LibraryItem;
}

interface OwnedTemplate {
  readonly tutorId: string;
  readonly template: Template;
}

interface ListedItem {
  readonly item: LibraryItem;
  readonly title: string;
  readonly kind: LibraryItem['kind'];
}

interface Destination {
  readonly present: boolean;
  readonly folderId: string | null;
}

type LibraryMove = 'folder' | 'material' | 'template';

const libraryPath = '/api/v1/library';
const foldersPath = '/api/v1/library/folders';
const moveRoute = /^\/api\/v1\/library\/(folders|materials|templates)\/([^/]+)$/;
const cursorPrefix = 'fake-library-cursor-';
const decimalDigits = /^\d+$/;
const defaultLimit = 30;
const maximumLimit = 100;
const folderNameLimit = 100;
const titleLimit = 200;
const folderMaxDepth = 3;

function answer(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' },
  });
}

function fieldRefusal(path: string, code: string): Response {
  return fakeRefusal(422, 'validation_failed', [{ path, code }]);
}

function normalised(text: string): string {
  return text.normalize('NFKC').trim().toLowerCase();
}

function validText(text: string, limit: number): boolean {
  const trimmed = text.trim();
  return trimmed !== '' && Array.from(trimmed).length <= limit;
}

function listOrder(left: ListedItem, right: ListedItem): number {
  return (
    left.title.localeCompare(right.title) ||
    left.kind.localeCompare(right.kind) ||
    left.item.id.localeCompare(right.item.id)
  );
}

function moveOf(kind: string | undefined): LibraryMove | null {
  if (kind === 'folders') {
    return 'folder';
  }
  if (kind === 'materials') {
    return 'material';
  }
  return kind === 'templates' ? 'template' : null;
}

function isRecord(value: unknown): value is object {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function fieldOf(body: object, name: string): unknown {
  return Object.entries(body).find(([key]) => key === name)?.[1];
}

function destinationOf(body: object, name: string): Destination | null {
  if (!(name in body)) {
    return { present: false, folderId: null };
  }
  const value = fieldOf(body, name);
  if (value === null) {
    return { present: true, folderId: null };
  }
  return typeof value === 'string' ? { present: true, folderId: value } : null;
}

async function jsonBody(request: Request): Promise<{ readonly parsed: boolean; readonly body: unknown }> {
  const text = await request.text();
  try {
    const body: unknown = JSON.parse(text);
    return { parsed: true, body };
  } catch {
    return { parsed: false, body: null };
  }
}

function templateItemOf(template: Template, path: LibraryPathEntry[] | null): LibraryItem {
  return {
    kind: 'template',
    id: template.id,
    title: template.title,
    folder_id: template.folder_id,
    updated_at: template.updated_at,
    question_count: template.question_count,
    max_points: template.max_points,
    ready: template.ready,
    version: template.version,
    path,
    url: null,
    expires_at: null,
    width: null,
    height: null,
  };
}

export class FakeLibraryServer {
  private readonly folders = new Map<string, OwnedFolder>();
  private readonly materials = new Map<string, OwnedMaterial>();
  private readonly templates = new Map<string, OwnedTemplate>();
  private created = 0;
  private lostMoves = 0;
  private failingMoves = 0;

  loseNextMove(): void {
    this.lostMoves += 1;
  }

  failNextMove(): void {
    this.failingMoves += 1;
  }

  addTemplate(tutorId: string, template: Template): void {
    this.templates.set(template.id, { tutorId, template });
  }

  template(tutorId: string, id: string): Template | null {
    const owned = this.templates.get(id);
    return owned?.tutorId === tutorId ? owned.template : null;
  }

  liveTemplateCount(tutorId: string): number {
    return [...this.templates.values()].filter((owned) => owned.tutorId === tutorId).length;
  }

  remove(id: string): void {
    this.folders.delete(id);
    this.materials.delete(id);
    this.templates.delete(id);
  }

  seed(tutorId: string, seed: FakeLibrarySeed, now: number): SeededFakeLibrary {
    const updatedAt = new Date(now).toISOString();
    const folderId = this.nextId();
    this.folders.set(folderId, {
      tutorId,
      folder: { id: folderId, name: seed.folderName ?? 'Grammar', parent_id: null },
    });
    const childFolderId = this.nextId();
    this.folders.set(childFolderId, {
      tutorId,
      folder: { id: childFolderId, name: seed.childFolderName ?? 'Irregular verbs', parent_id: folderId },
    });
    const templateId = this.nextId();
    const templateVersion = seed.templateVersion ?? 1;
    this.addTemplate(tutorId, {
      id: templateId,
      title: seed.templateTitle ?? 'Past simple',
      folder_id: null,
      blocks: [],
      version: templateVersion,
      ready: false,
      problems: [],
      question_count: 0,
      max_points: 0,
      created_at: updatedAt,
      updated_at: updatedAt,
    });
    const materialId = this.nextId();
    const materialTitle = seed.materialTitle ?? 'Verb table';
    this.materials.set(materialId, {
      tutorId,
      item: {
        kind: 'material',
        id: materialId,
        title: materialTitle,
        folder_id: null,
        updated_at: updatedAt,
        attachment_id: this.nextId(),
        filename: `${materialTitle}.pdf`,
        media_type: 'application/pdf',
        size_bytes: 1_024,
        path: null,
        url: null,
        expires_at: null,
        width: null,
        height: null,
      },
    });
    return { folderId, childFolderId, templateId, templateVersion, materialId };
  }

  handles(method: string, path: string): boolean {
    return (
      (method === 'GET' && (path === libraryPath || path === foldersPath)) ||
      (method === 'PATCH' && moveRoute.test(path))
    );
  }

  async handle(request: Request, tutorId: string, now: number, extraTemplateCount: number): Promise<Response> {
    const address = new URL(request.url);
    if (address.pathname === libraryPath) {
      return this.listing(tutorId, address.searchParams, extraTemplateCount);
    }
    if (address.pathname === foldersPath) {
      return answer(200, { folders: this.foldersOf(tutorId) });
    }
    const [, kind, encodedId = ''] = moveRoute.exec(address.pathname) ?? [];
    const move = moveOf(kind);
    if (move === null) {
      return fakeRefusal(404, 'not_found');
    }
    if (this.failingMoves > 0) {
      this.failingMoves -= 1;
      return fakeRefusal(503, 'service_unavailable');
    }
    const lost = this.lostMoves > 0;
    if (lost) {
      this.lostMoves -= 1;
    }
    const moved = await this.move(move, request, tutorId, decodeURIComponent(encodedId), now);
    if (lost) {
      throw new TypeError('the connection was lost before the library move answered');
    }
    return moved;
  }

  private async move(move: LibraryMove, request: Request, tutorId: string, id: string, now: number): Promise<Response> {
    const { parsed, body } = await jsonBody(request);
    if (!parsed) {
      return fakeRefusal(400, 'bad_request');
    }
    if (!isRecord(body)) {
      return fieldRefusal('body', 'invalid');
    }
    if (move === 'folder') {
      return this.moveFolder(tutorId, id, body);
    }
    return move === 'material' ? this.moveMaterial(tutorId, id, body, now) : this.moveTemplate(tutorId, id, body, now);
  }

  private moveFolder(tutorId: string, id: string, body: object): Response {
    const current = this.ownFolder(tutorId, id);
    const destination = destinationOf(body, 'parent_id');
    const name = fieldOf(body, 'name');
    if (destination === null || (name !== undefined && typeof name !== 'string')) {
      return fieldRefusal('body', 'invalid');
    }
    if (!destination.present && name === undefined) {
      return fieldRefusal('body', 'invalid');
    }
    if (current === null || !this.reachable(tutorId, destination)) {
      return fakeRefusal(404, 'not_found');
    }
    if (typeof name === 'string' && !validText(name, folderNameLimit)) {
      return fieldRefusal('body.name', 'invalid');
    }
    const parentId = destination.present ? destination.folderId : current.parent_id;
    if (parentId !== current.parent_id && !this.canMove(id, parentId)) {
      return fieldRefusal('body.parent_id', 'invalid');
    }
    const next: LibraryFolder = { id, name: typeof name === 'string' ? name : current.name, parent_id: parentId };
    const taken = this.foldersOf(tutorId).some(
      (folder) =>
        folder.id !== id && folder.parent_id === next.parent_id && normalised(folder.name) === normalised(next.name),
    );
    if (taken) {
      return fieldRefusal('body.name', 'taken');
    }
    this.folders.set(id, { tutorId, folder: next });
    return answer(200, next);
  }

  private moveMaterial(tutorId: string, id: string, body: object, now: number): Response {
    const owned = this.materials.get(id);
    const destination = destinationOf(body, 'folder_id');
    const title = fieldOf(body, 'title');
    if (destination === null || (title !== undefined && typeof title !== 'string')) {
      return fieldRefusal('body', 'invalid');
    }
    if (!destination.present && title === undefined) {
      return fieldRefusal('body', 'invalid');
    }
    if (owned?.tutorId !== tutorId || !this.reachable(tutorId, destination)) {
      return fakeRefusal(404, 'not_found');
    }
    if (typeof title === 'string' && !validText(title, titleLimit)) {
      return fieldRefusal('body.title', 'invalid');
    }
    const folderId = destination.present ? destination.folderId : owned.item.folder_id;
    const nextTitle = typeof title === 'string' ? title : owned.item.title;
    const changed = folderId !== owned.item.folder_id || nextTitle !== owned.item.title;
    const item: LibraryItem = changed
      ? { ...owned.item, folder_id: folderId, title: nextTitle, updated_at: new Date(now).toISOString() }
      : owned.item;
    this.materials.set(id, { tutorId, item });
    return answer(200, item);
  }

  private moveTemplate(tutorId: string, id: string, body: object, now: number): Response {
    const owned = this.template(tutorId, id);
    const destination = destinationOf(body, 'folder_id');
    const version = fieldOf(body, 'version');
    if (destination?.present !== true || typeof version !== 'number' || !Number.isInteger(version)) {
      return fieldRefusal('body', 'invalid');
    }
    if (owned === null || !this.reachable(tutorId, destination)) {
      return fakeRefusal(404, 'not_found');
    }
    if (owned.version !== version) {
      return fakeRefusal(409, 'conflict');
    }
    const template: Template =
      destination.folderId === owned.folder_id
        ? owned
        : {
            ...owned,
            folder_id: destination.folderId,
            version: owned.version + 1,
            updated_at: new Date(now).toISOString(),
          };
    this.addTemplate(tutorId, template);
    return answer(200, template);
  }

  private listing(tutorId: string, query: URLSearchParams, extraTemplateCount: number): Response {
    const folderId = query.get('folder') ?? '';
    const search = query.get('q') ?? '';
    const limitText = query.get('limit');
    const limit = limitText === null ? defaultLimit : decimalDigits.test(limitText) ? Number(limitText) : Number.NaN;
    const cursor = query.get('cursor');
    const offsetText = cursor === null ? '0' : cursor.startsWith(cursorPrefix) ? cursor.slice(cursorPrefix.length) : '';
    if (!(limit >= 1 && limit <= maximumLimit)) {
      return fieldRefusal('query.limit', 'invalid');
    }
    if (!decimalDigits.test(offsetText)) {
      return fieldRefusal('query.cursor', 'invalid');
    }
    if (folderId !== '' && search !== '') {
      return fieldRefusal('query.folder', 'not_allowed');
    }
    const folder = folderId === '' ? null : this.ownFolder(tutorId, folderId);
    if (folderId !== '' && folder === null) {
      return fakeRefusal(404, 'not_found');
    }
    const term = normalised(search);
    if (search !== '' && term === '') {
      return fieldRefusal('query.q', 'required');
    }
    const searching = search !== '';
    const listed = this.itemsOf(tutorId)
      .filter((entry) => (searching ? entry.title.includes(term) : entry.item.folder_id === (folder?.id ?? null)))
      .sort(listOrder)
      .map((entry) => (searching ? { ...entry.item, path: this.pathOf(tutorId, entry.item.folder_id) } : entry.item));
    const offset = Number(offsetText);
    const end = offset + limit;
    const library: Library = {
      folder: searching ? null : folder,
      path: searching ? [] : this.pathOf(tutorId, folder?.id ?? null),
      folders: searching ? [] : this.foldersOf(tutorId).filter((child) => child.parent_id === (folder?.id ?? null)),
      items: listed.slice(offset, end),
      next_cursor: end < listed.length ? `${cursorPrefix}${String(end)}` : null,
      template_count: this.liveTemplateCount(tutorId) + extraTemplateCount,
    };
    return answer(200, library);
  }

  private itemsOf(tutorId: string): ListedItem[] {
    const materials = [...this.materials.values()]
      .filter((owned) => owned.tutorId === tutorId)
      .map((owned): ListedItem => ({ item: owned.item, title: normalised(owned.item.title), kind: 'material' }));
    const templates = [...this.templates.values()]
      .filter((owned) => owned.tutorId === tutorId)
      .map((owned): ListedItem => ({
        item: templateItemOf(owned.template, null),
        title: normalised(owned.template.title),
        kind: 'template',
      }));
    return [...materials, ...templates];
  }

  private foldersOf(tutorId: string): LibraryFolder[] {
    return [...this.folders.values()]
      .filter((owned) => owned.tutorId === tutorId)
      .map((owned) => owned.folder)
      .sort((left, right) => left.name.localeCompare(right.name) || left.id.localeCompare(right.id));
  }

  private ownFolder(tutorId: string, id: string): LibraryFolder | null {
    const owned = this.folders.get(id);
    return owned?.tutorId === tutorId ? owned.folder : null;
  }

  private reachable(tutorId: string, destination: Destination): boolean {
    return destination.folderId === null || this.ownFolder(tutorId, destination.folderId) !== null;
  }

  private pathOf(tutorId: string, folderId: string | null): LibraryPathEntry[] {
    const path: LibraryPathEntry[] = [];
    for (
      let current = folderId === null ? null : this.ownFolder(tutorId, folderId);
      current !== null && path.length <= this.folders.size;
      current = current.parent_id === null ? null : this.ownFolder(tutorId, current.parent_id)
    ) {
      path.unshift({ id: current.id, name: current.name });
    }
    return path;
  }

  private levelOf(folderId: string): number {
    let level = 0;
    for (
      let current: string | null = folderId;
      current !== null && level <= this.folders.size;
      current = this.folders.get(current)?.folder.parent_id ?? null
    ) {
      level += 1;
    }
    return level;
  }

  private heightOf(folderId: string): number {
    const children = [...this.folders.values()].filter((owned) => owned.folder.parent_id === folderId);
    return 1 + Math.max(0, ...children.map((child) => this.heightOf(child.folder.id)));
  }

  private canMove(folderId: string, parentId: string | null): boolean {
    if (parentId === null) {
      return this.heightOf(folderId) <= folderMaxDepth;
    }
    let current: string | null = parentId;
    while (current !== null) {
      if (current === folderId) {
        return false;
      }
      current = this.folders.get(current)?.folder.parent_id ?? null;
    }
    return this.levelOf(parentId) + this.heightOf(folderId) <= folderMaxDepth;
  }

  private nextId(): string {
    this.created += 1;
    return `00000000-0000-4000-c000-${String(this.created).padStart(12, '0')}`;
  }
}
