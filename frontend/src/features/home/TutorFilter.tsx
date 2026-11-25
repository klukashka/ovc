import { type ReactElement, useEffect, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { SelectChevron, selectClass } from '../../components/ui/Form.tsx';
import { focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { paletteDotClass } from '../../components/ui/palette.ts';
import { SegmentedControl } from '../../components/ui/Tabs.tsx';
import type { MyTutor } from '../tutors/api.ts';

const allTutors = 'all';
const unlistedChoice = '';
const mostSegments = 4;

function asChoice(value: string): string | null {
  return value === allTutors ? null : value;
}

function TutorSelect({
  tutors,
  chosen,
  onChoose,
}: {
  readonly tutors: readonly MyTutor[];
  readonly chosen: string | null;
  readonly onChoose: (tutor: string | null) => void;
}): ReactElement {
  const { t } = useTranslation();
  const selectId = useId();
  const listed = tutors.some((tutor) => tutor.relationship_id === chosen);
  const unlisted = chosen !== null && !listed;
  const shown = chosen === null ? allTutors : listed ? chosen : unlistedChoice;
  return (
    <div className="flex flex-col gap-1 tablet:flex-row tablet:items-center tablet:gap-3">
      <label htmlFor={selectId} className="text-label text-text-muted">
        {t('home.filter')}
      </label>
      <div className="relative w-full tablet:w-64 tablet:max-w-full">
        <select
          id={selectId}
          value={shown}
          onChange={(event) => {
            onChoose(asChoice(event.target.value));
          }}
          className={selectClass}
        >
          {unlisted ? (
            <option value={unlistedChoice} disabled>
              {t('home.chooseTutor')}
            </option>
          ) : null}
          <option value={allTutors}>{t('home.allTutors')}</option>
          {tutors.map((tutor) => (
            <option key={tutor.relationship_id} value={tutor.relationship_id}>
              {tutor.tutor.name}
            </option>
          ))}
        </select>
        <SelectChevron />
      </div>
    </div>
  );
}

function TutorSegments({
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
  return (
    <div className="flex flex-col gap-2 tablet:flex-row tablet:flex-wrap tablet:items-center tablet:gap-3">
      <span id={labelId} className="text-label text-text-muted">
        {t('home.filter')}
      </span>
      <SegmentedControl
        name={{ labelledBy: labelId }}
        fill
        chosen={chosen ?? allTutors}
        onChoose={(value) => {
          onChoose(asChoice(value));
        }}
        segments={[
          { value: allTutors, label: t('home.allTutors') },
          ...tutors.map((tutor) => ({
            value: tutor.relationship_id,
            label: (
              <>
                <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[tutor.colour]}`} />
                <span dir="auto" className="min-w-0 wrap-anywhere">
                  {tutor.tutor.name}
                </span>
              </>
            ),
          })),
        ]}
      />
    </div>
  );
}

export function TutorFilter({
  tutors,
  chosen,
  onChoose,
}: {
  readonly tutors: readonly MyTutor[];
  readonly chosen: string | null;
  readonly onChoose: (tutor: string | null) => void;
}): ReactElement {
  useEffect(
    () => (): void => {
      window.requestAnimationFrame(focusHeadingIfLost);
    },
    [],
  );
  return tutors.length + 1 > mostSegments ? (
    <TutorSelect tutors={tutors} chosen={chosen} onChoose={onChoose} />
  ) : (
    <TutorSegments tutors={tutors} chosen={chosen} onChoose={onChoose} />
  );
}
