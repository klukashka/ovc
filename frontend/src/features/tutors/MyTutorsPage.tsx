import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { EmptyState } from '../../components/ui/EmptyState.tsx';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { PageColumns } from '../../components/ui/PageColumns.tsx';
import { asApiError } from '../../lib/api/client.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { type MyTutor, currentTutors, myTutorsKey, pastTutors } from './api.ts';
import { CodeEntry } from './CodeEntry.tsx';
import { SentRequests } from './SentRequests.tsx';
import { type StopOutcome, StopStudyingDialog } from './StopStudying.tsx';
import { myTutorsTextLoaders } from './texts.ts';
import { CurrentTutorTable, PastTutorTable, TutorsLoadFailed, useMyTutors } from './TutorList.tsx';

const stopNotice = {
  ended: 'tutors.stopped',
  removed: 'tutors.linkRemoved',
  changed: 'tutors.changed',
} as const;

function vanishedTutor(chosen: MyTutor | null, tutors: readonly MyTutor[] | undefined): StopOutcome | null {
  if (chosen === null || tutors === undefined) {
    return null;
  }
  const listed = tutors.find((tutor) => tutor.relationship_id === chosen.relationship_id);
  if (listed === undefined) {
    return 'removed';
  }
  return listed.state === 'ended' ? 'changed' : null;
}

function PastTutors({ tutors }: { readonly tutors: readonly MyTutor[] }): ReactElement | null {
  const { t } = useTranslation();
  const headingId = useId();
  if (tutors.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('tutors.past')}
      </h2>
      <PastTutorTable tutors={tutors} label={t('tutors.past')} />
    </section>
  );
}

export function MyTutorsPage(): ReactElement {
  const { t } = useTranslation();
  const texts = useFeatureTexts('myTutors', myTutorsTextLoaders);
  const addHeading = useId();
  const query = useMyTutors();
  const queryClient = useQueryClient();
  const [chosen, setChosen] = useState<MyTutor | null>(null);
  const [checking, setChecking] = useState(false);
  const stopping =
    chosen === null
      ? null
      : (currentTutors(query.data ?? []).find((tutor) => tutor.relationship_id === chosen.relationship_id) ?? chosen);
  const [outcome, setOutcome] = useState<{ readonly kind: StopOutcome; readonly count: number } | null>(null);
  const vanished = vanishedTutor(chosen, checking ? undefined : query.data);
  const opener = useRef<HTMLElement | null>(null);

  function current(): ReactElement {
    if (query.isPending) {
      return <DelayedLoading />;
    }
    if (query.isError) {
      return <TutorsLoadFailed query={query} />;
    }
    if (!texts.ready) {
      return texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />;
    }
    const tutors = currentTutors(query.data);
    return tutors.length === 0 ? (
      <EmptyState title={t('tutors.empty')} sentence={t('myTutors.emptyNext')} />
    ) : (
      <CurrentTutorTable
        tutors={tutors}
        label={t('tutors.list')}
        onStopStudying={(tutor) => {
          opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
          setOutcome(null);
          setChosen(tutor);
          setChecking(true);
          void queryClient.refetchQueries({ queryKey: myTutorsKey }).then(() => {
            setChecking(false);
            const fresh = currentTutors(queryClient.getQueryData<readonly MyTutor[]>(myTutorsKey) ?? []);
            if (!fresh.some((candidate) => candidate.relationship_id === tutor.relationship_id)) {
              setChosen(null);
              setOutcome((previous) => ({ kind: 'changed', count: (previous?.count ?? 0) + 1 }));
            }
          });
        }}
      />
    );
  }

  return (
    <Page>
      {outcome === null ? null : <FocusedNotice key={outcome.count} message={t(stopNotice[outcome.kind])} />}
      <PageColumns
        asideLabel={t('tutors.asideLabel')}
        main={
          <>
            {current()}
            <PastTutors tutors={query.isSuccess ? pastTutors(query.data) : []} />
          </>
        }
        aside={
          <>
            <section aria-labelledby={addHeading} className="flex flex-col gap-3">
              <h2 id={addHeading} className="text-heading">
                {t('tutors.add')}
              </h2>
              <CodeEntry label={t('tutors.code')} waiting={false} replaceWhenSent={false} />
            </section>
            <SentRequests />
          </>
        }
      />
      <StopStudyingDialog
        tutor={stopping}
        vanished={vanished}
        checking={checking}
        returnFocus={opener}
        onClose={() => {
          setChosen(null);
        }}
        onSettled={(kind) => {
          setChosen(null);
          setOutcome((previous) => ({ kind, count: (previous?.count ?? 0) + 1 }));
        }}
      />
    </Page>
  );
}
