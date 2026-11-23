import { type ReactElement, useEffect, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { paletteDotClass } from '../../components/ui/palette.ts';
import { SegmentedControl } from '../../components/ui/Tabs.tsx';
import type { MyTutor } from '../tutors/api.ts';

const allTutors = 'all';

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
    <div className="flex flex-col gap-2 tablet:flex-row tablet:flex-wrap tablet:items-center tablet:gap-3">
      <span id={labelId} className="text-label text-text-muted">
        {t('home.filter')}
      </span>
      <SegmentedControl
        name={{ labelledBy: labelId }}
        fill
        chosen={chosen ?? allTutors}
        onChoose={(value) => {
          onChoose(value === allTutors ? null : value);
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
