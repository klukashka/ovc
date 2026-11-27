import type { Locator } from '@playwright/test';

export async function visibleLabelLines(control: Locator): Promise<number> {
  return control.evaluate((element) => {
    const label = Array.from(element.childNodes).find(
      (node) => node.nodeType === Node.TEXT_NODE && (node.textContent ?? '').trim() !== '',
    );
    if (label === undefined) {
      return 0;
    }
    const range = document.createRange();
    range.selectNodeContents(label);
    return new Set(Array.from(range.getClientRects(), (rect) => Math.round(rect.top))).size;
  });
}
