import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useSearchParams } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { PageColumns } from '../../components/ui/PageColumns.tsx';
import { asApiError } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { localDateOf } from '../../lib/time/localDates.ts';
import { currentTutors } from '../tutors/api.ts';
import { CodeEntry } from '../tutors/CodeEntry.tsx';
import { type SentRequestsQuery, hasPendingRequest, useSentRequests } from '../tutors/SentRequests.tsx';
import { TutorList, TutorsLoadFailed, useMyTutors } from '../tutors/TutorList.tsx';
import { NextLesson, NextSevenDays } from './HomeLessons.tsx';
import { TutorFilter } from './TutorFilter.tsx';
import { useUpcomingLessons } from './upcoming.ts';

function FirstTutor({ query }: { readonly query: SentRequestsQuery }): ReactElement {
  const { t } = useTranslation();
  if (query.isPending) {
    return <DelayedLoading />;
  }
  if (query.isError) {
    return (
      <LoadFailed
        error={asApiError(query.error)}
        onRetry={() => {
          void query.refetch();
        }}
      />
    );
  }
  return (
    <>
      <p className="text-text-muted">{t('tutors.empty')}</p>
      <CodeEntry label={t('tutors.firstCode')} waiting={hasPendingRequest(query)} replaceWhenSent />
    </>
  );
}

function StudentTutors(): ReactElement {
  const { t } = useTranslation();
  const query = useMyTutors();
  const sentRequests = useSentRequests();
  if (query.isPending) {
    return <DelayedLoading />;
  }
  if (query.isError) {
    return <TutorsLoadFailed query={query} />;
  }
  const tutors = currentTutors(query.data);
  if (tutors.length === 0) {
    return <FirstTutor query={sentRequests} />;
  }
  return (
    <>
      <TutorList tutors={tutors} label={t('tutors.list')} withBio={false} />
      <p>
        <Link to="/tutors" className={linkClass}>
          {t('tutors.enterCode')}
        </Link>
      </p>
    </>
  );
}

function refusedTutor(error: unknown): boolean {
  const refusal = asApiError(error);
  return refusal.code === 'not_found' || refusal.code === 'validation_failed';
}

export function HomePage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const tutorsHeading = useId();
  const [params, setParams] = useSearchParams();
  const requested = params.get('tutor');
  const tutorsQuery = useMyTutors();
  const current = tutorsQuery.data === undefined ? [] : currentTutors(tutorsQuery.data);
  const known = requested === null || current.some((tutor) => tutor.relationship_id === requested);
  const unknownTutor = requested !== null && tutorsQuery.data !== undefined && !known;
  const { today, query } = useUpcomingLessons(
    me,
    requested,
    requested === null || (tutorsQuery.data !== undefined && known),
  );
  const unavailable = unknownTutor || (requested !== null && query.isError && refusedTutor(query.error));
  const todayCount =
    unavailable || !query.isSuccess
      ? null
      : query.data.items.filter(
          (item) => item.status !== 'cancelled' && localDateOf(Date.parse(item.starts_at), me.timezone) === today,
        ).length;
  const matched = current.find((tutor) => tutor.relationship_id === requested)?.relationship_id;
  const tutorsFailed = requested !== null && tutorsQuery.data === undefined && tutorsQuery.isError;
  const scheduleAddress =
    me.state !== 'active' && current.length === 0
      ? null
      : matched === undefined
        ? '/schedule'
        : `/schedule?${new URLSearchParams({ tutor: matched }).toString()}`;
  return (
    <Page titleSize="display">
      <p className="-mt-4 text-text-muted">
        {todayCount === null ? '\u00a0' : t('home.summary', { count: todayCount })}
      </p>
      <PageColumns
        asideLabel={t('home.aside')}
        main={
          <>
            {current.length >= 2 ? (
              <TutorFilter
                tutors={current}
                chosen={requested}
                onChoose={(tutor) => {
                  if (tutor !== requested) {
                    setParams(tutor === null ? {} : { tutor });
                  }
                }}
              />
            ) : null}
            {tutorsFailed ? (
              <p className="text-text-muted">{t('home.lessonsWaitForTutors')}</p>
            ) : unavailable ? (
              <div className="flex flex-col items-start gap-2">
                <p>{t('home.tutorUnavailable')}</p>
                <Link to="/home" className={linkClass}>
                  {t('home.allTutors')}
                </Link>
              </div>
            ) : (
              <NextLesson me={me} query={query} />
            )}
          </>
        }
        aside={
          <>
            {unavailable || tutorsFailed ? null : (
              <NextSevenDays me={me} query={query} scheduleAddress={scheduleAddress} />
            )}
            <section aria-labelledby={tutorsHeading} className="flex flex-col gap-3">
              <h2 id={tutorsHeading} className="text-heading">
                {t('section.tutors')}
              </h2>
              <StudentTutors />
            </section>
          </>
        }
      />
    </Page>
  );
}
