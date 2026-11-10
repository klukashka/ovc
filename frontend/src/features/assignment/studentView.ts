import type { components } from '../../lib/api/schema.ts';

type StudentTemplateResponse = components['schemas']['StudentTemplate'];
type StudentBlockResponse = components['schemas']['StudentTemplateBlock'];
type StudentFileResponse = components['schemas']['StudentTemplateFile'];
type StudentMaterialResponse = components['schemas']['StudentTemplateMaterial'];

export const studentFileKinds = ['images', 'pdf', 'office', 'audio'] as const;

export type StudentFileKind = (typeof studentFileKinds)[number];

export interface StudentFile {
  readonly filename: string;
  readonly mediaType: string;
  readonly sizeBytes: number;
  readonly width: number | null;
  readonly height: number | null;
  readonly url: string;
}

export interface StudentMaterial extends StudentFile {
  readonly title: string;
}

export interface StudentOption {
  readonly id: string;
  readonly text: string;
}

interface StudentQuestion {
  readonly id: string;
  readonly prompt: string;
  readonly images: readonly StudentFile[];
}

export type StudentBlock =
  | { readonly type: 'instruction'; readonly id: string; readonly text: string }
  | { readonly type: 'material'; readonly id: string; readonly material: StudentMaterial | null }
  | (StudentQuestion & {
      readonly type: 'single_choice' | 'multiple_choice';
      readonly options: readonly StudentOption[];
    })
  | (StudentQuestion & { readonly type: 'short_text' | 'short_number' })
  | (StudentQuestion & { readonly type: 'long_text'; readonly maxLength: number })
  | (StudentQuestion & {
      readonly type: 'file';
      readonly allowedKinds: readonly StudentFileKind[];
      readonly maxFiles: number;
    });

export interface StudentAssignment {
  readonly title: string;
  readonly blocks: readonly StudentBlock[];
}

export class MalformedStudentViewError extends Error {
  constructor(field: string) {
    super(`The student view arrived without a valid ${field}`);
    this.name = 'MalformedStudentViewError';
  }
}

function present<Value>(value: Value | undefined, field: string): Value {
  if (value === undefined) {
    throw new MalformedStudentViewError(field);
  }
  return value;
}

function file(entry: StudentFileResponse): StudentFile {
  return {
    filename: entry.filename,
    mediaType: entry.media_type,
    sizeBytes: entry.size_bytes,
    width: entry.width,
    height: entry.height,
    url: entry.url,
  };
}

function material(entry: StudentMaterialResponse | undefined): StudentMaterial | null {
  return entry === undefined ? null : { ...file(entry), title: entry.title };
}

function question(block: StudentBlockResponse): StudentQuestion {
  return { id: block.id, prompt: present(block.prompt, 'prompt'), images: (block.images ?? []).map(file) };
}

function isFileKind(value: string): value is StudentFileKind {
  return studentFileKinds.some((kind) => kind === value);
}

function block(entry: StudentBlockResponse): StudentBlock {
  switch (entry.type) {
    case 'instruction':
      return { type: entry.type, id: entry.id, text: present(entry.text, 'text') };
    case 'material':
      return { type: entry.type, id: entry.id, material: material(entry.material) };
    case 'single_choice':
    case 'multiple_choice':
      return {
        ...question(entry),
        type: entry.type,
        options: present(entry.options, 'options').map((option) => ({ id: option.id, text: option.text })),
      };
    case 'short_text':
    case 'short_number':
      return { ...question(entry), type: entry.type };
    case 'long_text':
      return { ...question(entry), type: entry.type, maxLength: present(entry.max_length, 'max_length') };
    case 'file':
      return {
        ...question(entry),
        type: entry.type,
        allowedKinds: present(entry.allowed_kinds, 'allowed_kinds').filter(isFileKind),
        maxFiles: present(entry.max_files, 'max_files'),
      };
  }
}

export function parseStudentView(response: StudentTemplateResponse): StudentAssignment {
  return { title: response.title, blocks: response.blocks.map(block) };
}
