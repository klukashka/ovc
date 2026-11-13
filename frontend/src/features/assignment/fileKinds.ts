import type { StudentFileKind } from './studentView.ts';

interface KindFormats {
  readonly mediaTypes: readonly string[];
  readonly extensions: readonly string[];
}

const kindFormats: Readonly<Record<StudentFileKind, KindFormats>> = {
  images: {
    mediaTypes: ['image/jpeg', 'image/png', 'image/webp', 'image/gif'],
    extensions: ['.jpg', '.jpeg', '.png', '.webp', '.gif'],
  },
  pdf: { mediaTypes: ['application/pdf'], extensions: ['.pdf'] },
  office: {
    mediaTypes: [
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      'application/vnd.openxmlformats-officedocument.presentationml.presentation',
      'application/vnd.oasis.opendocument.text',
      'application/vnd.oasis.opendocument.spreadsheet',
      'application/vnd.oasis.opendocument.presentation',
    ],
    extensions: ['.docx', '.xlsx', '.pptx', '.odt', '.ods', '.odp'],
  },
  audio: {
    mediaTypes: ['audio/mpeg', 'audio/mp4', 'audio/x-m4a', 'audio/ogg', 'audio/opus', 'audio/wav', 'audio/x-wav'],
    extensions: ['.mp3', '.m4a', '.ogg', '.oga', '.opus', '.wav'],
  },
};

export const answerFileLimitBytes = 25_000_000;

export const webpLimitBytes = 8 * 1024 * 1024;

export function acceptList(kinds: readonly StudentFileKind[]): string {
  return kinds.flatMap((kind) => [...kindFormats[kind].mediaTypes, ...kindFormats[kind].extensions]).join(',');
}

export function pictureAcceptList(): string {
  return kindFormats.images.mediaTypes.join(',');
}

function extensionOf(name: string): string {
  const dot = name.lastIndexOf('.');
  return dot < 0 ? '' : name.slice(dot).toLowerCase();
}

export function allowedFile(file: File, kinds: readonly StudentFileKind[]): boolean {
  const extension = extensionOf(file.name);
  return kinds.some(
    (kind) => kindFormats[kind].mediaTypes.includes(file.type) || kindFormats[kind].extensions.includes(extension),
  );
}

export function isWebp(file: File): boolean {
  return file.type === 'image/webp' || extensionOf(file.name) === '.webp';
}

export type FileRefusal = 'tooLarge' | 'webpTooLarge' | 'wrongType';

export function fileRefusal(file: File, kinds: readonly StudentFileKind[]): FileRefusal | null {
  if (!allowedFile(file, kinds)) {
    return 'wrongType';
  }
  if (isWebp(file) && file.size > webpLimitBytes) {
    return 'webpTooLarge';
  }
  return file.size > answerFileLimitBytes ? 'tooLarge' : null;
}
