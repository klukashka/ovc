import { useQuery } from '@tanstack/react-query';
import { type ReactElement, type ReactNode, type RefObject, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { relativeHint } from '../../lib/format/relative.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { useNow } from '../assignment/parts.tsx';
import { fetchGroups, groupsKey } from '../groups/api.ts';
import { type IssueItem, issueAddress } from '../homework/api.ts';
import { AudienceLabel } from '../homework/IssueParts.tsx';
import { reviewAddress } from '../review/api.ts';
import type { TutorHome } from './api.ts';
import { shownCount } from './api.ts';
import { homeworkPanelTextLoaders } from './panelTexts.ts';
import { type HomeRefetch, useRefetchAt } from './refetchAt.ts';

type NeedsCheckingPart = NonNullable<TutorHome['needs_checking']>;
type DeadlinesPart = NonNullable<TutorHome['upcoming_deadlines']>;

const noFilter = { issue: null, group: null, student: null };

function earliestDeadline(items: readonly IssueItem[]): string | null {
  let earliest: string | null = null;
  for (const item of items) {
    if (item.deadline_at !== null && (earliest === null || Date.parse(item.deadline_at) < Date.parse(earliest))) {
      earliest = item.deadline_at;
    }
  }
  return earliest;
}

function Panel({
  title,
  count,
  rowKeys,
  children,
}: {
  readonly title: string;
  readonly count: ReactNode;
  readonly rowKeys: readonly string[];
  readonly children: (listRef: RefObject<HTMLUListElement | null>) => ReactNode;
}): ReactElement {
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  useFocusAfterRemovedRow(listRef, rowKeys, '[data-row-link]', heading);
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {title} {count}
      </h2>
      {children(listRef)}
    </section>
  );
}

function NeedsChecking({ part }: { readonly part: NeedsCheckingPart }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const { items, count } = part;
  return (
    <Panel
      title={t('homeworkPanels.needsChecking')}
      count={<span className="tabular text-text-muted">{shownCount(count)}</span>}
      rowKeys={items.map((item) => item.submission_id)}
    >
      {(listRef) => (
        <>
          {items.length === 0 ? <p className="text-text-muted">{t('homeworkPanels.nothingToCheck')}</p> : null}
          <ul ref={listRef} className={items.length === 0 ? 'hidden' : 'flex flex-col border-b border-border'}>
            {items.map((item) => (
              <li
                key={item.submission_id}
                data-row={item.submission_id}
                className="flex flex-col gap-1 border-t border-border py-3"
              >
                <Link
                  data-row-link=""
                  to={reviewAddress(item.submission_id, noFilter)}
                  dir="auto"
                  className={`${linkClass} min-h-0 break-words`}
                >
                  {item.student.display_name}
                </Link>
                <span dir="auto" className="break-words">
                  {item.title}
                </span>
                {item.issued_before_reclaim ? (
                  <span className="text-small text-text-muted">{t('homeworkPanels.cue')}</span>
                ) : null}
                <span className="flex flex-wrap items-center gap-x-3 gap-y-1 text-small">
                  {item.submitted_at === null ? null : (
                    <span className="text-text-muted">
                      {formatDateTime(item.submitted_at, me.language, me.timezone)}
                    </span>
                  )}
                  {item.late ? <StatusWord tone="warning">{t('homeworkPanels.late')}</StatusWord> : null}
                  {item.has_draft ? <StatusWord tone="info">{t('homeworkPanels.reviewInProgress')}</StatusWord> : null}
                </span>
              </li>
            ))}
          </ul>
          <p>
            <Link to="/homework/review" className={linkClass}>
              {t('homeworkPanels.openQueue')}
            </Link>
          </p>
        </>
      )}
    </Panel>
  );
}

function UpcomingDeadlines({
  part,
  homeRefetch,
}: {
  readonly part: DeadlinesPart;
  readonly homeRefetch: HomeRefetch;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const me = useSignedInAccount();
  const { items } = part;
  useRefetchAt(earliestDeadline(items), homeRefetch);
  const groups = useQuery({
    queryKey: groupsKey,
    queryFn: () => fetchGroups(client),
    enabled: items.some((item) => item.audience.group_id !== undefined),
    retry: false,
  });
  const now = useNow();
  return (
    <Panel title={t('homeworkPanels.deadlines')} count={null} rowKeys={items.map((item) => item.id)}>
      {(listRef) => (
        <>
          {items.length === 0 ? <p className="text-text-muted">{t('homeworkPanels.noDeadlines')}</p> : null}
          <ul ref={listRef} className={items.length === 0 ? 'hidden' : 'flex flex-col border-b border-border'}>
            {items.map((item) => (
              <li key={item.id} data-row={item.id} className="flex flex-col gap-1 border-t border-border py-3">
                <Link
                  data-row-link=""
                  to={issueAddress(item.id)}
                  dir="auto"
                  className={`${linkClass} min-h-0 break-words`}
                >
                  {item.title}
                </Link>
                <span className="text-small">
                  <AudienceLabel audience={item.audience} groups={groups.data ?? []} />
                </span>
                {item.deadline_at === null ? null : (
                  <span className="text-small text-text-muted">
                    {formatDateTime(item.deadline_at, me.language, me.timezone)} (
                    {relativeHint(item.deadline_at, me.language, now)})
                  </span>
                )}
                <span className="text-small">
                  {t('homeworkPanels.submittedProgress', {
                    submitted: item.progress.submitted,
                    total: item.progress.students,
                  })}
                </span>
              </li>
            ))}
          </ul>
          <p>
            <Link to="/homework/issued" className={linkClass}>
              {t('homeworkPanels.allIssued')}
            </Link>
          </p>
        </>
      )}
    </Panel>
  );
}

function PanelTexts({ children }: { readonly children: ReactNode }): ReactElement {
  const texts = useFeatureTexts('homeworkPanels', homeworkPanelTextLoaders);
  if (!texts.ready) {
    return texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />;
  }
  return <>{children}</>;
}

export function NeedsCheckingPanel({ part }: { readonly part: NeedsCheckingPart }): ReactElement {
  return (
    <PanelTexts>
      <NeedsChecking part={part} />
    </PanelTexts>
  );
}

export function UpcomingDeadlinesPanel({
  part,
  homeRefetch,
}: {
  readonly part: DeadlinesPart;
  readonly homeRefetch: HomeRefetch;
}): ReactElement {
  return (
    <PanelTexts>
      <UpcomingDeadlines part={part} homeRefetch={homeRefetch} />
    </PanelTexts>
  );
}
