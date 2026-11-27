import { useEffect } from 'react';

export function carriesFiles(event: { readonly dataTransfer: DataTransfer | null }): boolean {
  return event.dataTransfer?.types.includes('Files') ?? false;
}

export function holdsFolder(transfer: DataTransfer): boolean {
  return Array.from(transfer.items).some((item) => item.webkitGetAsEntry()?.isDirectory === true);
}

function outsideDropZone(event: globalThis.DragEvent): boolean {
  const target = event.target;
  const element = target instanceof Element ? target : target instanceof Node ? target.parentElement : null;
  return element?.closest('[data-drop-zone]') === null || element === null;
}

export function useFileDropGuard(): void {
  useEffect(() => {
    const guard = (event: globalThis.DragEvent): void => {
      if (!carriesFiles(event)) {
        return;
      }
      event.preventDefault();
      if (event.type === 'dragover' && event.dataTransfer !== null && outsideDropZone(event)) {
        event.dataTransfer.dropEffect = 'none';
      }
    };
    window.addEventListener('dragover', guard);
    window.addEventListener('drop', guard);
    return (): void => {
      window.removeEventListener('dragover', guard);
      window.removeEventListener('drop', guard);
    };
  }, []);
}
