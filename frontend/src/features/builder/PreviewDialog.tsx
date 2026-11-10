import { type ReactElement, type RefObject, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { ModalDialog } from '../../components/ui/Dialog.tsx';
import { quietButtonClass } from '../../components/ui/Form.tsx';
import { StudentBlocks } from '../assignment/StudentBlocks.tsx';
import type { StudentAssignment } from '../assignment/studentView.ts';

type Frame = 'phone' | 'computer';

const frameClass: Record<Frame, string> = {
  phone: 'mx-auto w-full max-w-[360px]',
  computer: 'w-full',
};

const frameButtonClass =
  'inline-flex min-h-11 items-center justify-center rounded-md border border-border-strong px-4 text-strong text-text aria-pressed:bg-accent aria-pressed:text-on-accent';

export function PreviewDialog({
  assignment,
  returnFocus,
  onClose,
}: {
  readonly assignment: StudentAssignment | null;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const closeButton = useRef<HTMLButtonElement>(null);
  const [frame, setFrame] = useState<Frame>('computer');
  return (
    <ModalDialog
      open={assignment !== null}
      busy={false}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={closeButton}
      wide
      onClose={onClose}
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 id={headingId} className="font-serif text-title">
            {t('builder.preview')}
          </h2>
          <div className="flex flex-wrap items-center gap-2">
            <div role="group" aria-label={t('builder.previewWidth')} className="flex gap-1">
              {(['phone', 'computer'] as const).map((option) => (
                <button
                  key={option}
                  type="button"
                  aria-pressed={frame === option}
                  onClick={() => {
                    setFrame(option);
                  }}
                  className={frameButtonClass}
                >
                  {t(`builder.${option}`)}
                </button>
              ))}
            </div>
            <button ref={closeButton} type="button" onClick={onClose} className={quietButtonClass}>
              {t('builder.close')}
            </button>
          </div>
        </div>
        <div data-preview-frame={frame} className={`${frameClass[frame]} rounded-lg border border-border p-4`}>
          {assignment === null ? null : <StudentBlocks assignment={assignment} origin={window.location.origin} />}
        </div>
      </div>
    </ModalDialog>
  );
}
