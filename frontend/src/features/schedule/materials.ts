import type { components } from '../../lib/api/schema.ts';
import { downloadAddress } from '../../lib/files/downloadAddress.ts';
import { type LibraryItem, libraryKey } from '../library/api.ts';
import type { LessonDetail } from './api.ts';

export type LessonMaterial = components['schemas']['LessonMaterial'];

export const lessonMaterialLimit = 20;

const addressRenewalMargin = 30_000;

export interface PickedMaterial {
  readonly id: string;
  readonly title: string;
  readonly filename: string;
}

export function pickedOf(materials: readonly LessonMaterial[]): readonly PickedMaterial[] {
  return materials.flatMap((material) =>
    material.material_id === undefined
      ? []
      : [{ id: material.material_id, title: material.title, filename: material.filename }],
  );
}

export function pickedFromLibrary(item: LibraryItem): PickedMaterial {
  return { id: item.id, title: item.title, filename: item.filename ?? '' };
}

export function idsOf(materials: readonly PickedMaterial[]): string[] {
  return materials.map((material) => material.id);
}

export function sameMaterials(left: readonly PickedMaterial[], right: readonly PickedMaterial[]): boolean {
  return left.length === right.length && left.every((material, index) => material.id === right[index]?.id);
}

export function materialAddress(material: LessonMaterial, origin: string): string | null {
  return downloadAddress(material.attachment_id, material.url, origin);
}

export function opensAsPicture(material: LessonMaterial): boolean {
  return (
    material.width !== null &&
    material.height !== null &&
    material.width > 0 &&
    material.height > 0 &&
    material.media_type.startsWith('image/')
  );
}

export function addressRenewalDelay(lesson: LessonDetail | undefined, now: number): number | false {
  const expiries = (lesson?.materials ?? []).map((material) => Date.parse(material.expires_at));
  if (expiries.length === 0) {
    return false;
  }
  return Math.max(addressRenewalMargin, Math.min(...expiries) - now - addressRenewalMargin);
}

export function libraryPickerKey(folder: string | null, search: string | null): readonly unknown[] {
  return [...libraryKey, 'picker', folder, search];
}
