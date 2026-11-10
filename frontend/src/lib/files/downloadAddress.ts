export function downloadAddress(attachmentId: string, url: string, origin: string): string | null {
  try {
    const address = new URL(url, origin);
    if (
      address.origin !== origin ||
      address.username !== '' ||
      address.password !== '' ||
      address.hash !== '' ||
      address.pathname !== `/api/v1/files/${attachmentId}` ||
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
