import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import type { StudentLessonWork } from '../schedule/lessonIssues.ts';
import { assignmentAddress } from './api.ts';
import { DeadlineHint, StateLabel } from './parts.tsx';
import { assignmentTextLoaders } from './texts.ts';

function WorkRow({ work, now }: { readonly work: StudentLessonWork; readonly now: number }): ReactElement {
  return (
    <li data-row={work.submissionId} className="flex flex-col gap-1 border-t border-border py-3">
      <Link
        data-row-link=""
        to={assignmentAddress(work.submissionId)}
        dir="auto"
        className={`${linkClass} min-h-0 break-words`}
      >
        {work.title}
      </Link>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-small">
        <DeadlineHint deadline={work.deadlineAt} now={now} />
        <StateLabel status={work.status} deadline={work.deadlineAt} late={work.late} now={now} />
      </div>
    </li>
  );
}

function LessonWorkList({
  work,
  now,
}: {
  readonly work: readonly StudentLessonWork[];
  readonly now: number;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  useFocusAfterRemovedRow(
    listRef,
    work.map((entry) => entry.submissionId),
    '[data-row-link]',
    heading,
  );
  return (
    <section aria-labelledby={headingId} data-part="assignments" className="flex flex-col gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('assignment.list.label')}
      </h2>
      <ul ref={listRef} className="flex flex-col border-b border-border">
        {work.map((entry) => (
          <WorkRow key={entry.submissionId} work={entry} now={now} />
        ))}
      </ul>
    </section>
  );
}

function LessonWorkWithTexts(props: {
  readonly work: readonly StudentLessonWork[];
  readonly now: number;
}): ReactElement | null {
  const texts = useFeatureTexts('assignment', assignmentTextLoaders);
  return texts.ready ? <LessonWorkList {...props} /> : null;
}

export function LessonWork(props: {
  readonly work: readonly StudentLessonWork[];
  readonly now: number;
}): ReactElement | null {
  return props.work.length === 0 ? null : <LessonWorkWithTexts {...props} />;
}
