const loopbackHostname = /^(localhost|\[::1\]|127(\.\d{1,3}){3})$/i;

export function namesTheFileItself(address: string, pageOrigin: string): boolean {
  return URL.canParse(address, pageOrigin) && new URL(address, pageOrigin).origin === pageOrigin;
}

export function startDownload(address: string): void {
  const link = document.createElement('a');
  link.href = address;
  if (namesTheFileItself(address, window.location.origin)) {
    link.download = '';
  }
  link.rel = 'noopener noreferrer';
  document.body.append(link);
  link.click();
  link.remove();
}

function isStorageOrigin(address: URL): boolean {
  return address.protocol === 'https:' || (address.protocol === 'http:' && loopbackHostname.test(address.hostname));
}

export function storageDownloadAddress(url: string): string | null {
  if (!URL.canParse(url)) {
    return null;
  }
  const address = new URL(url);
  if (!isStorageOrigin(address) || address.username !== '' || address.password !== '' || address.hash !== '') {
    return null;
  }
  return address.href;
}
