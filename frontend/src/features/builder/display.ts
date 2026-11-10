import type { components } from '../../lib/api/schema.ts';
import type { LibraryUpload } from '../library/upload.ts';
import type { Template } from './autosave.ts';

export type ProblemCode = components['schemas']['TemplateProblem']['code'];

export interface MaterialInfo {
  readonly attachmentId: string;
  readonly title: string;
  readonly filename: string;
  readonly deleted: boolean;
  readonly url: string | null;
  readonly expiresAt: string | null;
}

export interface ImageInfo {
  readonly filename: string;
  readonly width: number | null;
  readonly height: number | null;
  readonly url: string | null;
  readonly expiresAt: string | null;
}

export interface PictureUpload {
  readonly key: string;
  readonly blockId: string;
  readonly upload: LibraryUpload;
}

export interface DisplayEntries {
  readonly materials: ReadonlyMap<string, MaterialInfo>;
  readonly images: ReadonlyMap<string, ImageInfo>;
}

export function displayEntries(template: Template, previous: DisplayEntries): DisplayEntries {
  const materials = new Map(previous.materials);
  const images = new Map(previous.images);
  for (const block of template.blocks) {
    if (block.material !== undefined) {
      const material = block.material;
      materials.set(material.material_id, {
        attachmentId: material.attachment_id,
        title: material.title,
        filename: material.filename,
        deleted: material.deleted,
        url: material.url,
        expiresAt: material.expires_at,
      });
    }
    for (const image of block.images ?? []) {
      images.set(image.attachment_id, {
        filename: image.filename,
        width: image.width,
        height: image.height,
        url: image.url,
        expiresAt: image.expires_at,
      });
    }
  }
  return { materials, images };
}

export const noDisplayEntries: DisplayEntries = { materials: new Map(), images: new Map() };

export function earliestExpiry(entries: DisplayEntries): number | null {
  const times = [...entries.materials.values(), ...entries.images.values()]
    .map((entry) => (entry.expiresAt === null ? Number.NaN : Date.parse(entry.expiresAt)))
    .filter((time) => Number.isFinite(time));
  return times.length === 0 ? null : Math.min(...times);
}

export function problemsByBlock(template: Template): ReadonlyMap<string | null, readonly ProblemCode[]> {
  const problems = new Map<string | null, ProblemCode[]>();
  for (const problem of template.problems) {
    const codes = problems.get(problem.block_id) ?? [];
    codes.push(problem.code);
    problems.set(problem.block_id, codes);
  }
  return problems;
}
