const shownExtension = /^[a-z0-9]{1,4}$/iu;

export function extensionTile(filename: string): string | null {
  const dot = filename.lastIndexOf('.');
  if (dot <= 0) return null;
  const extension = filename.slice(dot + 1);
  return shownExtension.test(extension) ? extension.toUpperCase() : null;
}
