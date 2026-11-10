export function safeLinkTarget(destination: string): string | null {
  const url = URL.parse(destination);
  if (url === null) {
    return null;
  }
  if (url.protocol !== 'https:' || url.hostname === '' || url.username !== '' || url.password !== '') {
    return null;
  }
  return url.href;
}
