export const calendarKey = ['calendar'] as const;

export const lessonsKey = ['lesson'] as const;

export function lessonKey(id: string): readonly unknown[] {
  return [...lessonsKey, id];
}
