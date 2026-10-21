function isControlCharacter(character: string): boolean {
  const code = character.codePointAt(0) ?? 0;
  return code <= 0x1f || code === 0x7f;
}

function isSingleSlashPath(value: string): boolean {
  return value.startsWith('/') && !value.startsWith('//') && !value.startsWith('/\\');
}

export function safeNext(value: string | null, origin: string): string | null {
  if (value === null || !isSingleSlashPath(value) || Array.from(value).some(isControlCharacter)) {
    return null;
  }
  let parsed: URL;
  try {
    parsed = new URL(value, origin);
  } catch {
    return null;
  }
  const target = `${parsed.pathname}${parsed.search}${parsed.hash}`;
  if (parsed.origin !== origin || !isSingleSlashPath(target)) {
    return null;
  }
  return target;
}
