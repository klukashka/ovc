import { type ApiClient, type ApiError, apiErrorFrom, asApiError } from '../../lib/api/client.ts';
import type { Session } from '../../lib/api/session.ts';
import { retryAfterSeconds } from '../../components/ui/Form.tsx';
import { fetchFolders } from '../library/api.ts';
import { LibraryUpload, type UploadState } from '../library/upload.ts';
import { attachmentsFolderOf, materialTitleOf } from './newTask.ts';

export const uploadsInFlight = 4;

export type AttachmentRefusal =
  | { readonly kind: 'tooLarge' }
  | { readonly kind: 'failed'; readonly error: ApiError; readonly retryAfter: number | null };

export type AttachmentState =
  | { readonly kind: 'waiting' }
  | { readonly kind: 'uploading'; readonly loaded: number; readonly total: number | null }
  | { readonly kind: 'saving' }
  | { readonly kind: 'ready'; readonly materialId: string }
  | { readonly kind: 'refused'; readonly refusal: AttachmentRefusal; readonly retryable: boolean };

export interface Attachment {
  readonly key: string;
  readonly file: File;
  readonly state: AttachmentState;
}

interface Entry {
  readonly key: string;
  readonly file: File;
  state: AttachmentState;
  attachmentId: string | null;
  upload: LibraryUpload | null;
  release: (() => void) | null;
}

export interface AttachmentNames {
  readonly folder: string;
  readonly fallbackTitle: string;
}

type Outcome<Value> =
  | { readonly kind: 'ok'; readonly value: Value }
  | { readonly kind: 'refused'; readonly error: ApiError; readonly retryAfter: number | null };

function refused<Value>(response: Response, error: unknown): Outcome<Value> {
  return { kind: 'refused', error: apiErrorFrom(response.status, error), retryAfter: retryAfterSeconds(response) };
}

function thrownOutcome<Value>(thrown: unknown): Outcome<Value> {
  return { kind: 'refused', error: asApiError(thrown), retryAfter: null };
}

const permanentCodes: ReadonlySet<string> = new Set([
  'payload_too_large',
  'unsupported_media_type',
  'validation_failed',
  'email_not_verified',
]);

export class TaskAttachments {
  private entries: Entry[] = [];
  private view: readonly Attachment[] = [];
  private readonly listeners = new Set<() => void>();
  private folder: Promise<Outcome<string | null>> | null = null;
  private stopped = false;
  private starting = false;
  private counter = 0;

  private readonly client: ApiClient;
  private readonly session: Session;
  private readonly names: AttachmentNames;

  constructor(client: ApiClient, session: Session, names: AttachmentNames) {
    this.client = client;
    this.session = session;
    this.names = names;
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  readonly snapshot = (): readonly Attachment[] => this.view;

  add(files: readonly File[]): void {
    for (const file of files) {
      this.counter += 1;
      this.entries.push({
        key: `attachment-${String(this.counter)}`,
        file,
        state: { kind: 'waiting' },
        attachmentId: null,
        upload: null,
        release: null,
      });
    }
    this.changed();
    this.startWaiting();
  }

  retry(key: string): void {
    const entry = this.entries.find((candidate) => candidate.key === key);
    if (entry?.state.kind !== 'refused' || !entry.state.retryable) {
      return;
    }
    entry.state = { kind: 'waiting' };
    this.changed();
    this.startWaiting();
  }

  remove(key: string): void {
    const entry = this.entries.find((candidate) => candidate.key === key);
    if (entry === undefined) {
      return;
    }
    this.stopUpload(entry);
    this.entries = this.entries.filter((candidate) => candidate !== entry);
    this.changed();
    this.startWaiting();
  }

  resume(): void {
    this.stopped = false;
    this.startWaiting();
  }

  stop(): void {
    this.stopped = true;
    for (const entry of this.entries) {
      this.stopUpload(entry);
      if (entry.state.kind === 'uploading' || entry.state.kind === 'saving') {
        entry.state = { kind: 'waiting' };
      }
    }
  }

  private stopUpload(entry: Entry): void {
    entry.release?.();
    entry.release = null;
    entry.upload?.cancel();
    entry.upload = null;
  }

  private changed(): void {
    this.view = this.entries.map((entry) => ({
      key: entry.key,
      file: entry.file,
      state: entry.state,
    }));
    for (const listener of this.listeners) {
      listener();
    }
  }

  private live(entry: Entry): boolean {
    return !this.stopped && this.entries.includes(entry);
  }

  private settle(entry: Entry, state: AttachmentState): void {
    if (!this.live(entry)) {
      return;
    }
    entry.state = state;
    this.changed();
    if (state.kind !== 'uploading' && state.kind !== 'saving') {
      this.startWaiting();
    }
  }

  private refuse(entry: Entry, outcome: Extract<Outcome<unknown>, { kind: 'refused' }>, retryable = true): void {
    this.settle(entry, {
      kind: 'refused',
      refusal: { kind: 'failed', error: outcome.error, retryAfter: outcome.retryAfter },
      retryable: retryable && !permanentCodes.has(outcome.error.code ?? ''),
    });
  }

  private startWaiting(): void {
    if (this.stopped || this.starting) {
      return;
    }
    this.starting = true;
    try {
      for (;;) {
        const busy = this.entries.filter(
          (entry) => entry.state.kind === 'uploading' || entry.state.kind === 'saving',
        ).length;
        const next = this.entries.find((entry) => entry.state.kind === 'waiting');
        if (busy >= uploadsInFlight || next === undefined) {
          return;
        }
        this.begin(next);
      }
    } finally {
      this.starting = false;
    }
  }

  private begin(entry: Entry): void {
    if (entry.attachmentId !== null) {
      void this.saveMaterial(entry, entry.attachmentId);
      return;
    }
    const upload = new LibraryUpload(this.client, this.session, entry.file);
    entry.upload = upload;
    const follow = (): void => {
      this.followUpload(entry, upload.snapshot());
    };
    entry.release = upload.subscribe(follow);
    follow();
  }

  private followUpload(entry: Entry, state: UploadState): void {
    switch (state.kind) {
      case 'uploading':
        this.settle(entry, { kind: 'uploading', loaded: state.loaded, total: state.total });
        return;
      case 'tooLarge':
      case 'wrongType':
        this.stopUpload(entry);
        this.settle(entry, { kind: 'refused', refusal: { kind: 'tooLarge' }, retryable: false });
        return;
      case 'refused':
        this.stopUpload(entry);
        this.refuse(entry, { kind: 'refused', error: state.error, retryAfter: state.retryAfter });
        return;
      case 'uploaded':
        this.stopUpload(entry);
        entry.attachmentId = state.file.id;
        void this.saveMaterial(entry, state.file.id);
    }
  }

  private async saveMaterial(entry: Entry, attachmentId: string): Promise<void> {
    this.settle(entry, { kind: 'saving' });
    const folder = await this.attachmentsFolder();
    if (!this.live(entry)) {
      return;
    }
    if (folder.kind === 'refused') {
      this.refuse(entry, folder);
      return;
    }
    const first = await this.createMaterial(
      attachmentId,
      materialTitleOf(entry.file.name, this.names.fallbackTitle),
      folder.value,
    );
    const titleRefused = first.kind === 'refused' && first.error.fieldCode('title') !== undefined;
    const outcome = titleRefused
      ? await this.createMaterial(attachmentId, this.names.fallbackTitle, folder.value)
      : first;
    if (outcome.kind === 'refused' && outcome.error.code === 'not_found') {
      this.folder = null;
    }
    if (outcome.kind === 'refused') {
      this.refuse(entry, outcome, !(titleRefused && outcome.error.fieldCode('title') !== undefined));
      return;
    }
    this.settle(entry, { kind: 'ready', materialId: outcome.value });
  }

  private async createMaterial(attachmentId: string, title: string, folderId: string | null): Promise<Outcome<string>> {
    try {
      const { data, error, response } = await this.client.POST('/api/v1/library/materials', {
        body: { attachment_id: attachmentId, title, folder_id: folderId },
      });
      return data === undefined ? refused(response, error) : { kind: 'ok', value: data.id };
    } catch (thrown: unknown) {
      return thrownOutcome(thrown);
    }
  }

  private attachmentsFolder(): Promise<Outcome<string | null>> {
    if (this.folder === null) {
      const resolving = this.resolveFolder();
      this.folder = resolving;
      void resolving.then((outcome) => {
        if (outcome.kind === 'refused' && this.folder === resolving) {
          this.folder = null;
        }
      });
    }
    return this.folder;
  }

  private async listedFolder(): Promise<Outcome<string | null>> {
    try {
      const folders = await fetchFolders(this.client, new AbortController().signal);
      return { kind: 'ok', value: attachmentsFolderOf(folders, this.names.folder) };
    } catch (thrown: unknown) {
      return thrownOutcome(thrown);
    }
  }

  private async resolveFolder(): Promise<Outcome<string | null>> {
    const listed = await this.listedFolder();
    if (listed.kind === 'refused' || listed.value !== null) {
      return listed;
    }
    try {
      const { data, error, response } = await this.client.POST('/api/v1/library/folders', {
        body: { name: this.names.folder, parent_id: null },
      });
      if (data !== undefined) {
        return { kind: 'ok', value: data.id };
      }
      const refusal = refused<string | null>(response, error);
      if (refusal.kind === 'refused' && refusal.error.fieldCode('name') === 'taken') {
        return await this.listedFolder();
      }
      if (refusal.kind === 'refused' && refusal.error.fieldCode('body') === 'too_many') {
        return { kind: 'ok', value: null };
      }
      return response.status === 0 || response.status >= 500 ? await this.listedAfterUnknown(refusal) : refusal;
    } catch (thrown: unknown) {
      return await this.listedAfterUnknown(thrownOutcome(thrown));
    }
  }

  private async listedAfterUnknown(refusal: Outcome<string | null>): Promise<Outcome<string | null>> {
    const listed = await this.listedFolder();
    return listed.kind === 'ok' && listed.value !== null ? listed : refusal;
  }
}

export function attachmentsPending(attachments: readonly Attachment[]): boolean {
  return attachments.some((attachment) => attachment.state.kind !== 'ready');
}

export function readyMaterialIds(attachments: readonly Attachment[]): readonly string[] {
  return attachments.flatMap((attachment) => (attachment.state.kind === 'ready' ? [attachment.state.materialId] : []));
}
