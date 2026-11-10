import { type ApiClient, type ApiError, apiErrorFrom, asApiError } from '../../lib/api/client.ts';
import type { Session } from '../../lib/api/session.ts';
import { retryAfterSeconds } from '../../components/ui/Form.tsx';
import { type UploadedFile, libraryUploadLimitBytes, webpUploadLimitBytes } from './api.ts';

export type UploadState =
  | { readonly kind: 'uploading'; readonly loaded: number; readonly total: number | null }
  | { readonly kind: 'uploaded'; readonly file: UploadedFile }
  | { readonly kind: 'refused'; readonly error: ApiError; readonly retryAfter: number | null }
  | { readonly kind: 'tooLarge'; readonly limit: 'file' | 'webp' };

function isWebp(file: File): boolean {
  return file.type === 'image/webp' || file.name.toLowerCase().endsWith('.webp');
}

function sizeRefusal(file: File): UploadState | null {
  if (file.size > libraryUploadLimitBytes) {
    return { kind: 'tooLarge', limit: 'file' };
  }
  if (isWebp(file) && file.size > webpUploadLimitBytes) {
    return { kind: 'tooLarge', limit: 'webp' };
  }
  return null;
}

export class LibraryUpload {
  readonly file: File;
  private state: UploadState;
  private readonly listeners = new Set<() => void>();
  private readonly controller = new AbortController();

  constructor(client: ApiClient, session: Session, file: File) {
    this.file = file;
    const refused = sizeRefusal(file);
    this.state = refused ?? { kind: 'uploading', loaded: 0, total: file.size };
    if (refused === null) {
      void this.send(client, session);
    }
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  readonly snapshot = (): UploadState => this.state;

  cancel(): void {
    this.controller.abort();
    this.listeners.clear();
  }

  private settle(state: UploadState): void {
    if (this.controller.signal.aborted) {
      return;
    }
    this.state = state;
    for (const listener of this.listeners) {
      listener();
    }
  }

  private async send(client: ApiClient, session: Session): Promise<void> {
    const body = new FormData();
    body.append('file', this.file);
    try {
      const { data, error, response } = await client.POST('/api/v1/uploads', {
        params: { query: { context: 'library' } },
        signal: this.controller.signal,
        body: { file: '' },
        bodySerializer: () => null,
        fetch: (request) =>
          session.authorizedUpload(request, {
            body,
            progress: (loaded, total) => {
              this.settle({ kind: 'uploading', loaded, total });
            },
          }),
      });
      if (data === undefined) {
        this.settle({
          kind: 'refused',
          error: apiErrorFrom(response.status, error),
          retryAfter: retryAfterSeconds(response),
        });
        return;
      }
      this.settle({ kind: 'uploaded', file: data });
    } catch (thrown: unknown) {
      this.settle({ kind: 'refused', error: asApiError(thrown), retryAfter: null });
    }
  }
}
