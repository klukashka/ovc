import type { UploadTransfer } from './transport.ts';

export function sendUpload(request: Request, upload: UploadTransfer): Promise<Response> {
  request.signal.throwIfAborted();
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const abort = (): void => {
      xhr.abort();
    };
    const cleanup = (): void => {
      request.signal.removeEventListener('abort', abort);
      xhr.onload = null;
      xhr.onerror = null;
      xhr.onabort = null;
      xhr.ontimeout = null;
      xhr.upload.onprogress = null;
    };
    const fail = (error: Error): void => {
      cleanup();
      reject(error);
    };
    xhr.open(request.method, request.url);
    for (const [name, value] of request.headers) {
      if (name.toLowerCase() !== 'content-type') xhr.setRequestHeader(name, value);
    }
    xhr.timeout = 635_000;
    xhr.upload.onprogress = (event): void => {
      if (!request.signal.aborted) upload.progress(event.loaded, event.lengthComputable ? event.total : null);
    };
    xhr.onload = (): void => {
      const headers = new Headers();
      for (const line of xhr
        .getAllResponseHeaders()
        .trim()
        .split(/[\r\n]+/)) {
        const colon = line.indexOf(':');
        if (colon > 0) headers.append(line.slice(0, colon), line.slice(colon + 1).trim());
      }
      cleanup();
      resolve(new Response(xhr.responseText, { status: xhr.status, headers }));
    };
    xhr.onerror = (): void => {
      fail(new TypeError('upload network failure'));
    };
    xhr.ontimeout = (): void => {
      fail(new TypeError('upload timeout'));
    };
    xhr.onabort = (): void => {
      fail(new DOMException('upload cancelled', 'AbortError'));
    };
    request.signal.addEventListener('abort', abort, { once: true });
    xhr.send(upload.body);
  });
}
