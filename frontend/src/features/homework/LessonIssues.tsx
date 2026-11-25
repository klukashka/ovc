import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { DelayedLoading } from '../../components/ui/LoadState.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { relativeHint } from '../../lib/format/relative.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { type TutorLessonIssue, lessonIssueLimit } from '../schedule/lessonIssues.ts';
import { issueAddress } from './api.ts';
import { type IssuePreset, IssueAssignmentButton, homeworkTextLoaders } from './IssueAssignment.tsx';

const lessonIssueOpener = 'lesson-issue';
const lessonEmptyIssueOpener = 'lesson-issue-empty';

function IssueRow({ issue, now }: { readonly issue: TutorLessonIssue; readonly now: number }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  return (
    <li data-row={issue.issueId} className="flex flex-col gap-1 border-t border-border py-3">
      <Link data-row-link="" to={issueAddress(issue.issueId)} dir="auto" className={`${linkClass} min-h-0 break-words`}>
        {issue.title}
      </Link>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-small text-text-muted">
        <span>
          {issue.deadlineAt === null
            ? t('homework.list.noDeadline')
            : `${formatDateTime(issue.deadlineAt, me.language, me.timezone)} (${relativeHint(issue.deadlineAt, me.language, now)})`}
        </span>
        <span>{t('homework.progress.submitted', { submitted: issue.submitted, total: issue.students })}</span>
      </div>
    </li>
  );
}

function LessonIssueList({
  issues,
  now,
  preset,
}: {
  readonly issues: readonly TutorLessonIssue[];
  readonly now: number;
  readonly preset: IssuePreset | null;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const limitId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  useFocusAfterRemovedRow(
    listRef,
    issues.map((issue) => issue.issueId),
    '[data-row-link]',
    heading,
  );
  const limited = issues.length >= lessonIssueLimit;
  return (
    <section aria-labelledby={headingId} data-part="assignments" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
          {t('homework.sections.assignments')}
        </h2>
        {preset === null ? null : (
          <IssueAssignmentButton
            opener={lessonIssueOpener}
            preset={preset}
            unavailableReason={limited ? limitId : null}
          />
        )}
      </div>
      {preset !== null && limited ? (
        <p id={limitId} className="text-small text-text-muted">
          {t('homework.refusals.lessonLimit')}
        </p>
      ) : null}
      {issues.length === 0 ? (
        <div className="flex flex-col items-start gap-3">
          <p className="text-text-muted">{t('homework.empty.lesson')}</p>
          {preset === null ? null : (
            <IssueAssignmentButton opener={lessonEmptyIssueOpener} preset={preset}>
              {t('homework.empty.issueForLesson')}
            </IssueAssignmentButton>
          )}
        </div>
      ) : null}
      <ul ref={listRef} className={issues.length === 0 ? 'hidden' : 'flex flex-col border-b border-border'}>
        {issues.map((issue) => (
          <IssueRow key={issue.issueId} issue={issue} now={now} />
        ))}
      </ul>
    </section>
  );
}

export function LessonIssues(props: {
  readonly issues: readonly TutorLessonIssue[];
  readonly now: number;
  readonly preset: IssuePreset | null;
}): ReactElement | null {
  const texts = useFeatureTexts('homework', homeworkTextLoaders);
  if (texts.ready) {
    return <LessonIssueList {...props} />;
  }
  return texts.pending ? <DelayedLoading /> : null;
}
