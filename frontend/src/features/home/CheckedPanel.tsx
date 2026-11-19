import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { asApiError } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { type AssignmentItem, assignmentAddress } from '../assignment/api.ts';
import { TutorName } from '../assignment/parts.tsx';
import { homeworkPanelTextLoaders } from './panelTexts.ts';

function CheckedRow({ item }: { readonly item: AssignmentItem }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  return (
    <li data-row={item.submission_id} className="flex flex-col gap-1 border-t border-border py-3">
      <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <Link
          data-row-link=""
          to={assignmentAddress(item.submission_id)}
          dir="auto"
          className={`${linkClass} min-h-0 break-words`}
        >
          {item.title}
        </Link>
        {item.feedback_new ? (
          <span
            role="img"
            aria-label={t('homeworkPanels.newFeedbackName')}
            data-new-feedback=""
            className="rounded-full bg-accent px-2 text-caption font-medium text-on-accent"
          >
            <span aria-hidden="true">{t('homeworkPanels.newFeedback')}</span>
          </span>
        ) : null}
      </span>
      {item.issued_before_reclaim ? (
        <span className="text-small text-text-muted">{t('homeworkPanels.cue')}</span>
      ) : null}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-small">
        <TutorName name={item.tutor.name} colour={item.tutor.colour} />
        {item.checked_at === null ? null : (
          <span className="text-text-muted">
            {t('homeworkPanels.checkedOn', { date: formatDate(item.checked_at, me.language, me.timezone) })}
          </span>
        )}
      </div>
      {item.feedback_excerpt === null ? null : (
        <p data-excerpt="" dir="auto" className="break-words">
          {item.feedback_excerpt}
        </p>
      )}
      {item.has_corrected_files ? (
        <p className="text-small text-text-muted">{t('homeworkPanels.correctedFile')}</p>
      ) : null}
    </li>
  );
}

function CheckedRows({ items }: { readonly items: readonly AssignmentItem[] }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  useFocusAfterRemovedRow(
    listRef,
    items.map((item) => item.submission_id),
    '[data-row-link]',
    heading,
  );
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('homeworkPanels.recentlyChecked')}
      </h2>
      {items.length === 0 ? <p className="text-text-muted">{t('homeworkPanels.nothingChecked')}</p> : null}
      <ul ref={listRef} className={items.length === 0 ? 'hidden' : 'flex flex-col border-b border-border'}>
        {items.map((item) => (
          <CheckedRow key={item.submission_id} item={item} />
        ))}
      </ul>
    </section>
  );
}

export function CheckedPanel({
  items,
  tutor,
}: {
  readonly items: readonly AssignmentItem[];
  readonly tutor: string | null;
}): ReactElement | null {
  const texts = useFeatureTexts('homeworkPanels', homeworkPanelTextLoaders);
  if (!texts.ready) {
    return texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />;
  }
  return <CheckedRows key={tutor ?? ''} items={items} />;
}
