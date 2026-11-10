const filePathPattern = /^\/api\/v1\/files\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

function signedAddress(url: string, origin: string, pathAllowed: (path: string) => boolean): string | null {
  try {
    const address = new URL(url, origin);
    if (
      address.origin !== origin ||
      address.username !== '' ||
      address.password !== '' ||
      address.hash !== '' ||
      !pathAllowed(address.pathname) ||
      address.searchParams.getAll('t').length !== 1 ||
      [...address.searchParams.keys()].some((key) => key !== 't')
    ) {
      return null;
    }
    return address.href;
  } catch {
    return null;
  }
}

export function downloadAddress(attachmentId: string, url: string, origin: string): string | null {
  return signedAddress(url, origin, (path) => path === `/api/v1/files/${attachmentId}`);
}

export function studentFileAddress(url: string, origin: string): string | null {
  return signedAddress(url, origin, (path) => filePathPattern.test(path));
}
