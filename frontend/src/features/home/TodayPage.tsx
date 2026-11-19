import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useLocalToday } from '../../app/clock.ts';
import { FormAlert, FormNotice, quietButtonClass } from '../../components/ui/Form.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { PageColumns } from '../../components/ui/PageColumns.tsx';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { type CopyResult, copyText } from '../../lib/clipboard.ts';
import type { TutorCode as TutorCodeValue } from '../settings/tutorCode.ts';
import { formatLongDay } from '../schedule/calendarFormat.ts';
import { HomeJoinRequestList } from '../students/JoinRequests.tsx';
import { tutorHomeKey, useTutorHome } from './api.ts';
import { FirstRunChecklist } from './FirstRunChecklist.tsx';
import { HomeFirstLoad, HomeRefetchNotice } from './HomeLoadState.tsx';
import { TodayLessons } from './TodayLessons.tsx';
import { NeedsCheckingPanel, UpcomingDeadlinesPanel } from './TutorHomeworkPanels.tsx';
import { UnreadMessages } from './UnreadMessages.tsx';

type CopyNotice = { readonly result: CopyResult; readonly count: number } | null;

function TutorCode({ code }: { readonly code: TutorCodeValue }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const [notice, setNotice] = useState<CopyNotice>(null);

  function copy(value: string): void {
    function show(result: CopyResult): void {
      setNotice((previous) => ({ result, count: (previous?.count ?? 0) + 1 }));
    }
    copyText(navigator.clipboard, value).then(show, () => {
      show('failed');
    });
  }

  function body(): ReactElement {
    return (
      <>
        <p className="font-mono text-heading tracking-wider">{code.code}</p>
        <p className="text-small text-text-muted">{t(code.active ? 'joinRequests.codeHint' : 'teaching.inactive')}</p>
        {notice === null ? null : notice.result === 'copied' ? (
          <FormNotice key={notice.count} takeFocus>
            {t('teaching.copied')}
          </FormNotice>
        ) : (
          <FormAlert key={notice.count}>{t('teaching.copyFailed')}</FormAlert>
        )}
        <p>
          <button
            type="button"
            onClick={() => {
              copy(code.code);
            }}
            className={quietButtonClass}
          >
            {t('teaching.copy')}
          </button>
        </p>
      </>
    );
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-2 border-t border-border pt-4">
      <h3 id={headingId} className="text-strong">
        {t('joinRequests.code')}
      </h3>
      {body()}
    </section>
  );
}

export function TodayPage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const today = useLocalToday(me.timezone);
  const query = useTutorHome();
  const queryClient = useQueryClient();
  const lessonsHeading = useRef<HTMLHeadingElement>(null);
  const [dismissals, setDismissals] = useState(0);
  useEffect(() => {
    if (dismissals > 0) {
      lessonsHeading.current?.focus();
    }
  }, [dismissals]);
  const home = query.data;
  const homeRefetch = {
    refetch: query.refetch,
    loadedAt: query.dataUpdatedAt,
    failedAt: query.errorUpdatedAt,
  };
  return (
    <Page titleSize="display">
      <p className="-mt-4 text-text-muted">{formatLongDay(today, me.language)}</p>
      {home === undefined ? (
        <HomeFirstLoad query={query} />
      ) : (
        <PageColumns
          asideLabel={t('today.aside')}
          main={
            <>
              <HomeRefetchNotice query={query} />
              <FirstRunChecklist
                me={me}
                checklist={home.checklist}
                onDismissed={() => {
                  setDismissals((count) => count + 1);
                }}
              />
              <TodayLessons me={me} lessons={home.today_lessons} headingRef={lessonsHeading} />
              {home.needs_checking === null ? null : <NeedsCheckingPanel part={home.needs_checking} />}
            </>
          }
          aside={
            <>
              <HomeJoinRequestList
                part={home.join_requests}
                loadedAt={queryClient.getQueryState(tutorHomeKey)?.dataUpdateCount ?? 0}
              >
                <TutorCode code={home.tutor_code} />
              </HomeJoinRequestList>
              {home.upcoming_deadlines === null ? null : (
                <UpcomingDeadlinesPanel part={home.upcoming_deadlines} homeRefetch={homeRefetch} />
              )}
              <UnreadMessages part={home.unread_channels} />
            </>
          }
        />
      )}
    </Page>
  );
}
