import { describe, expect, test } from 'vitest';

import { UploadQueue, type UploadedFile } from './UploadQueue.ts';

class Uploader {
  readonly requests: {
    readonly file: File;
    readonly signal: AbortSignal;
    readonly progress: (loaded: number, total: number | null) => void;
    readonly resolve: (file: UploadedFile) => void;
    readonly reject: (error: Error) => void;
  }[] = [];
  readonly upload = (
    file: File,
    signal: AbortSignal,
    progress: (loaded: number, total: number | null) => void,
  ): Promise<UploadedFile> =>
    new Promise((resolve, reject) => {
      this.requests.push({ file, signal, progress, resolve, reject });
    });
  async finish(index: number): Promise<void> {
    const request = this.requests[index];
    if (request === undefined) throw new Error('missing request');
    request.resolve({
      id: `file-${String(index)}`,
      filename: request.file.name,
      media_type: 'text/plain',
      size_bytes: 1,
    });
    await Promise.resolve();
  }
}
function files(count: number): File[] {
  return Array.from({ length: count }, (_, index) => new File(['a'], `${String(index)}.txt`, { type: 'text/plain' }));
}
describe('upload queue', () => {
  test('bounds the queue at ten and starts only four transfers, promoting waiting files on completion', async () => {
    const uploader = new Uploader();
    const queue = new UploadQueue(uploader.upload, () => true);

    const accepted = queue.add(files(10));
    const refused = queue.add(files(1));
    const before = uploader.requests.length;
    uploader.requests[0]?.progress(1, 2);
    const progress = queue.snapshot()[0]?.progress;
    await uploader.finish(0);

    expect(accepted).toBe(true);
    expect(refused).toBe(false);
    expect(queue.snapshot()).toHaveLength(10);
    expect(before).toBe(4);
    expect(progress).toBe(50);
    expect(uploader.requests).toHaveLength(5);
    expect(queue.snapshot()[0]?.state).toBe('ready');
  });
  test('cancels active uploads and ignores late results and progress after ownership ends', async () => {
    const uploader = new Uploader();
    let current = true;
    const queue = new UploadQueue(uploader.upload, () => current);
    queue.add(files(6));

    current = false;
    queue.stop();
    uploader.requests[0]?.progress(2, 2);
    await uploader.finish(0);
    const accepted = queue.add(files(1));

    expect(uploader.requests.every((request) => request.signal.aborted)).toBe(true);
    expect(uploader.requests).toHaveLength(4);
    expect(queue.snapshot()).toEqual([]);
    expect(accepted).toBe(false);
  });
  test('reports an uncertain failure without replay and frees the slot for a different waiting file', async () => {
    const uploader = new Uploader();
    const queue = new UploadQueue(uploader.upload, () => true);
    queue.add(files(5));

    uploader.requests[0]?.reject(new TypeError('connection lost'));
    await Promise.resolve();

    expect(queue.snapshot()[0]?.state).toBe('failed');
    expect(uploader.requests).toHaveLength(5);
    expect(uploader.requests.filter((request) => request.file.name === '0.txt')).toHaveLength(1);
  });
});
