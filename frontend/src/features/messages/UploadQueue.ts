import { asApiError, type ApiError } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

export type UploadedFile = components['schemas']['UploadedFile'];
export interface UploadItem {
  readonly key: number;
  readonly name: string;
  readonly state: 'waiting' | 'uploading' | 'ready' | 'failed';
  readonly progress: number | null;
  readonly uploaded: UploadedFile | null;
  readonly error: ApiError | null;
}
export type UploadFile = (
  file: File,
  signal: AbortSignal,
  progress: (loaded: number, total: number | null) => void,
) => Promise<UploadedFile>;

export class UploadQueue {
  private items: readonly UploadItem[] = [];
  private readonly files = new Map<number, File>();
  private readonly active = new Map<number, AbortController>();
  private readonly listeners = new Set<() => void>();
  private nextKey = 0;
  private stopped = false;

  private readonly upload: UploadFile;
  private readonly current: () => boolean;

  constructor(upload: UploadFile, current: () => boolean) {
    this.upload = upload;
    this.current = current;
  }

  readonly snapshot = (): readonly UploadItem[] => this.items;
  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  add(files: readonly File[]): boolean {
    if (this.stopped || !this.current()) return false;
    if (files.length + this.items.length > 10) return false;
    for (const file of files) {
      const key = this.nextKey++;
      this.files.set(key, file);
      this.items = [
        ...this.items,
        { key, name: file.name, state: 'waiting', progress: null, uploaded: null, error: null },
      ];
    }
    this.notify();
    this.pump();
    return true;
  }

  remove(key: number): void {
    this.active.get(key)?.abort();
    this.active.delete(key);
    this.files.delete(key);
    this.items = this.items.filter((item) => item.key !== key);
    this.notify();
    this.pump();
  }

  clear(): void {
    for (const controller of this.active.values()) controller.abort();
    this.active.clear();
    this.files.clear();
    this.items = [];
    this.notify();
  }

  stop(): void {
    this.stopped = true;
    this.clear();
  }

  private notify(): void {
    for (const listener of this.listeners) listener();
  }

  private change(key: number, changes: Partial<UploadItem>): void {
    this.items = this.items.map((item) => (item.key === key ? { ...item, ...changes } : item));
    this.notify();
  }

  private pump(): void {
    if (this.stopped || !this.current()) return;
    for (const item of this.items) {
      if (this.active.size >= 4) break;
      if (item.state !== 'waiting') continue;
      const file = this.files.get(item.key);
      if (file === undefined) throw new Error('queued upload has no file');
      const controller = new AbortController();
      this.active.set(item.key, controller);
      this.change(item.key, { state: 'uploading' });
      void this.transfer(item.key, file, controller);
    }
  }

  private async transfer(key: number, file: File, controller: AbortController): Promise<void> {
    const valid = (): boolean =>
      !this.stopped && this.current() && !controller.signal.aborted && this.active.get(key) === controller;
    try {
      const uploaded = await this.upload(file, controller.signal, (loaded, total) => {
        if (valid())
          this.change(key, {
            progress: total === null || total === 0 ? null : Math.min(100, Math.floor((loaded / total) * 100)),
          });
      });
      if (valid()) this.change(key, { state: 'ready', progress: 100, uploaded });
    } catch (error: unknown) {
      if (valid()) this.change(key, { state: 'failed', error: asApiError(error) });
    } finally {
      if (this.active.get(key) === controller) {
        this.active.delete(key);
        this.files.delete(key);
      }
      this.pump();
    }
  }
}
