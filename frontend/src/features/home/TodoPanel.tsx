import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { EmptyState } from '../../components/ui/EmptyState.tsx';
import { linkClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { SectionHead } from '../../components/ui/SectionHead.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { asApiError } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { relativeHint } from '../../lib/format/relative.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { type AssignmentItem, assignmentAddress } from '../assignment/api.ts';
import { TutorName, useNow } from '../assignment/parts.tsx';
import { stateTones, studentStateWord } from '../assignment/states.ts';
import { type StudentHome, shownCount } from './api.ts';
import { homeworkPanelTextLoaders } from './panelTexts.ts';

type PanelWord = 'todo' | 'dueSoon' | 'overdue' | 'inProgress' | 'returned';

function panelWord(item: AssignmentItem, now: number): PanelWord {
  const word = studentStateWord(item.status, item.deadline_at, item.late, now);
  switch (word) {
    case 'dueSoon':
    case 'overdue':
    case 'inProgress':
    case 'returned':
      return word;
    default:
      return 'todo';
  }
}

function TodoRow({ item, now }: { readonly item: AssignmentItem; readonly now: number }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const word = panelWord(item, now);
  const address = assignmentAddress(item.submission_id);
  return (
    <li
      data-row={item.submission_id}
      className="flex flex-wrap items-start justify-between gap-3 border-t border-border py-3"
    >
      <div className="flex min-w-0 flex-col gap-1">
        <Link data-row-link="" to={address} dir="auto" className={`${linkClass} min-h-0 break-words`}>
          {item.title}
        </Link>
        {item.issued_before_reclaim ? (
          <span className="text-small text-text-muted">{t('homeworkPanels.cue')}</span>
        ) : null}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-small">
          <TutorName name={item.tutor.name} colour={item.tutor.colour} />
          <StatusWord tone={stateTones[word]}>{t(`homeworkPanels.states.${word}`)}</StatusWord>
          <span className="text-text-muted">
            {item.deadline_at === null
              ? t('homeworkPanels.noDeadline')
              : `${formatDateTime(item.deadline_at, me.language, me.timezone)} (${relativeHint(item.deadline_at, me.language, now)})`}
          </span>
        </div>
      </div>
      <Link to={address} className={linkClass}>
        {item.status === 'not_started' ? t('homeworkPanels.start') : t('homeworkPanels.continue')}
        <span className="sr-only">
          {' '}
          <span dir="auto">{item.title}</span>
        </span>
      </Link>
    </li>
  );
}

function TodoRows({ part }: { readonly part: StudentHome['todo'] }): ReactElement {
  const { t } = useTranslation();
  const now = useNow();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const { items, count } = part;
  useFocusAfterRemovedRow(
    listRef,
    items.map((item) => item.submission_id),
    '[data-row-link]',
    heading,
  );
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <SectionHead
        headingId={headingId}
        heading={heading}
        title={t('homeworkPanels.todo')}
        action={
          count > 0 ? (
            <Link to="/homework" className={linkClass}>
              {t('homeworkPanels.allHomework', { count: shownCount(count) })}
            </Link>
          ) : null
        }
      />
      {items.length === 0 ? (
        <EmptyState
          level={3}
          title={t('homeworkPanels.nothingToDo')}
          sentence={t('homeworkPanels.nothingToDoSentence')}
        />
      ) : null}
      <ul ref={listRef} className={items.length === 0 ? 'hidden' : 'flex flex-col border-b border-border'}>
        {items.map((item) => (
          <TodoRow key={item.submission_id} item={item} now={now} />
        ))}
      </ul>
    </section>
  );
}

export function TodoPanel({
  part,
  tutor,
}: {
  readonly part: StudentHome['todo'];
  readonly tutor: string | null;
}): ReactElement | null {
  const texts = useFeatureTexts('homeworkPanels', homeworkPanelTextLoaders);
  if (!texts.ready) {
    return texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />;
  }
  return <TodoRows key={tutor ?? ''} part={part} />;
}
