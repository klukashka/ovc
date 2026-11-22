import { type ReactElement, type RefObject, useId, useLayoutEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { phoneLayoutQuery, useMediaQuery } from '../../app/context.ts';
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

function PreviewContent({
  assignment,
  headingId,
  heading,
  closeButton,
  phone,
  onClose,
}: {
  readonly assignment: StudentAssignment;
  readonly headingId: string;
  readonly heading: RefObject<HTMLHeadingElement | null>;
  readonly closeButton: RefObject<HTMLButtonElement | null>;
  readonly phone: boolean;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const [frame, setFrame] = useState<Frame>(phone ? 'phone' : 'computer');
  const shownLayout = useRef(phone);
  useLayoutEffect(() => {
    if (shownLayout.current === phone) {
      return;
    }
    shownLayout.current = phone;
    if (document.activeElement === null || document.activeElement === document.body) {
      closeButton.current?.focus();
    }
  }, [phone, closeButton]);
  const close = (
    <button ref={closeButton} type="button" onClick={onClose} className={quietButtonClass}>
      {t('builder.close')}
    </button>
  );
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 ref={heading} id={headingId} tabIndex={-1} className="font-serif text-title outline-none">
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
          {phone ? null : close}
        </div>
      </div>
      <div data-preview-frame={frame} className={`${frameClass[frame]} rounded-lg border border-border p-4`}>
        <StudentBlocks assignment={assignment} origin={window.location.origin} />
      </div>
      {phone ? <div className="flex flex-col">{close}</div> : null}
    </div>
  );
}

export function PreviewDialog({
  assignment,
  returnFocus,
  onClose,
}: {
  readonly assignment: StudentAssignment | null;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
}): ReactElement {
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const phone = useMediaQuery(phoneLayoutQuery);
  return (
    <ModalDialog
      open={assignment !== null}
      busy={false}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={phone ? heading : closeButton}
      wide
      onClose={onClose}
    >
      {assignment === null ? null : (
        <PreviewContent
          assignment={assignment}
          headingId={headingId}
          heading={heading}
          closeButton={closeButton}
          phone={phone}
          onClose={onClose}
        />
      )}
    </ModalDialog>
  );
}
