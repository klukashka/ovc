import { type ReactElement, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { quietButtonClass } from '../../components/ui/Form.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { useSignedInAccount } from '../../lib/api/session.ts';
import type { Audience } from './AudiencePicker.tsx';
import { type LessonPrefill, NewLessonDialog, nextFullHour } from './NewLessonDialog.tsx';

export function ScheduleLessonAction({
  audience,
  onScheduled,
}: {
  readonly audience: Audience;
  readonly onScheduled: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const opener = useRef<HTMLButtonElement>(null);
  const [prefill, setPrefill] = useState<LessonPrefill | null>(null);
  const [announcement, setAnnouncement] = useState<string | null>(null);
  const [closedPrefill] = useState<LessonPrefill>(() => nextFullHour(Date.now(), me.timezone));
  return (
    <div>
      <button
        ref={opener}
        type="button"
        onClick={() => {
          setAnnouncement(null);
          setPrefill({ ...nextFullHour(Date.now(), me.timezone), audience });
        }}
        className={quietButtonClass}
      >
        {t('lessons.schedule')}
      </button>
      <LiveNotice message={announcement} />
      <NewLessonDialog
        open={prefill !== null}
        prefill={prefill ?? closedPrefill}
        returnFocus={opener}
        onClose={() => {
          setPrefill(null);
        }}
        onScheduled={() => {
          setPrefill(null);
          setAnnouncement(t('schedule.scheduled'));
          onScheduled();
        }}
      />
    </div>
  );
}
