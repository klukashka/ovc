export function firstFieldOf(questionId: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    `[data-question="${questionId}"] input:not([type="hidden"]), [data-question="${questionId}"] textarea, [data-question="${questionId}"] button`,
  );
}
