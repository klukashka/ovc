export function revealRow(row: HTMLElement): void {
  const pane = row.closest<HTMLElement>('[data-history-pane]');
  if (pane !== null) pane.scrollTop = Math.max(0, row.offsetTop - (pane.clientHeight - row.offsetHeight) / 2);
  row.focus();
}
