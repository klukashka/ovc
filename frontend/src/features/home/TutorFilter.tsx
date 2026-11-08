import { type ReactElement, useEffect, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { paletteDotClass } from '../../components/ui/palette.ts';
import type { MyTutor } from '../tutors/api.ts';

const segmentClass =
  'inline-flex min-h-11 items-center gap-2 rounded-sm border border-transparent px-3 text-small text-text-muted hover:text-text aria-pressed:border-border-strong aria-pressed:bg-field aria-pressed:font-medium aria-pressed:text-text desktop:pointer-fine:min-h-10';

export function TutorFilter({
  tutors,
  chosen,
  onChoose,
}: {
  readonly tutors: readonly MyTutor[];
  readonly chosen: string | null;
  readonly onChoose: (tutor: string | null) => void;
}): ReactElement {
  const { t } = useTranslation();
  const labelId = useId();
  useEffect(
    () => (): void => {
      window.requestAnimationFrame(focusHeadingIfLost);
    },
    [],
  );
  return (
    <div className="flex flex-wrap items-center gap-3">
      <span id={labelId} className="text-label text-text-muted">
        {t('home.filter')}
      </span>
      <div role="group" aria-labelledby={labelId} className="flex flex-wrap gap-1 rounded-md bg-hover p-1">
        <button
          type="button"
          aria-pressed={chosen === null}
          onClick={() => {
            onChoose(null);
          }}
          className={segmentClass}
        >
          {t('home.allTutors')}
        </button>
        {tutors.map((tutor) => (
          <button
            key={tutor.relationship_id}
            type="button"
            aria-pressed={chosen === tutor.relationship_id}
            onClick={() => {
              onChoose(tutor.relationship_id);
            }}
            className={segmentClass}
          >
            <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[tutor.colour]}`} />
            <span dir="auto">{tutor.tutor.name}</span>
          </button>
        ))}
      </div>
    </div>
  );
}
