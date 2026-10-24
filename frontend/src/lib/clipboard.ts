export type CopyResult = 'copied' | 'failed';

export async function copyText(clipboard: Clipboard | undefined, text: string): Promise<CopyResult> {
  if (clipboard === undefined) {
    return 'failed';
  }
  try {
    await clipboard.writeText(text);
    return 'copied';
  } catch {
    return 'failed';
  }
}
