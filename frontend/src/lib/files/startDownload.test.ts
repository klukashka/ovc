import { describe, expect, test } from 'vitest';

import { namesTheFileItself, storageDownloadAddress } from './startDownload.ts';

describe('the storage download address rule', () => {
  test.each([
    'https://storage.example/exports/a.zip?X-Amz-Signature=s',
    'http://127.0.0.1:58333/exports/a.zip?X-Amz-Signature=s',
    'http://localhost:58333/exports/a.zip?X-Amz-Signature=s',
    'http://[::1]:58333/exports/a.zip?X-Amz-Signature=s',
  ])('accepts an https origin or http on a loopback host: %s', (url) => {
    const address = url;

    const accepted = storageDownloadAddress(address);

    expect(accepted).toBe(new URL(address).href);
  });

  test.each([
    'http://storage.example/exports/a.zip?X-Amz-Signature=s',
    'http://10.0.0.5/exports/a.zip?X-Amz-Signature=s',
    'http://127.0.0.1.storage.example/exports/a.zip?X-Amz-Signature=s',
    'https://user:secret@storage.example/exports/a.zip',
    'https://storage.example/exports/a.zip#fragment',
    'javascript:alert(1)',
    'data:application/zip;base64,UEsDBA==',
    'not an address',
  ])('refuses a cleartext remote host, credentials, a fragment or another scheme: %s', (url) => {
    const address = url;

    const accepted = storageDownloadAddress(address);

    expect(accepted).toBeNull();
  });
});

describe('the download attribute rule', () => {
  const pageOrigin = 'https://ovc.example';

  test.each(['/api/v1/files/f1/download?signature=s', 'https://ovc.example/api/v1/files/f1/download?signature=s'])(
    'keeps the download attribute for an address of the page origin: %s',
    (address) => {
      const target = address;

      const keeps = namesTheFileItself(target, pageOrigin);

      expect(keeps).toBe(true);
    },
  );

  test.each([
    'https://storage.example/exports/a.zip?X-Amz-Signature=s',
    'http://ovc.example/api/v1/files/f1/download',
    'https://ovc.example:8443/api/v1/files/f1/download',
    'https://cdn.ovc.example/exports/a.zip',
  ])('leaves the attribute off for another origin, where the signed response names the file: %s', (address) => {
    const target = address;

    const keeps = namesTheFileItself(target, pageOrigin);

    expect(keeps).toBe(false);
  });
});
