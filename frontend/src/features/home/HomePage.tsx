import { type QueryClient, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useSearchParams } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed, focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { PageColumns } from '../../components/ui/PageColumns.tsx';
import { asApiError } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { reachable } from '../../routes/access.ts';
import { provisionallyLinked, useProvisionalLink } from '../../routes/provisionalLink.ts';
import type { MyTutor } from '../tutors/api.ts';
import { CodeEntry } from '../tutors/CodeEntry.tsx';
import { hasPendingRequest, useSentRequests } from '../tutors/SentRequests.tsx';
import { TutorList } from '../tutors/TutorList.tsx';
import { type StudentHome, type StudentHomeTutor, studentHomesKey, useStudentHomeRead } from './api.ts';
import { CheckedPanel } from './CheckedPanel.tsx';
import { NextLesson, NextSevenDays } from './HomeLessons.tsx';
import { HomeFirstLoad, HomeRefetchNotice } from './HomeLoadState.tsx';
import { TodoPanel } from './TodoPanel.tsx';
import { TutorFilter } from './TutorFilter.tsx';

function asMyTutor(tutor: StudentHomeTutor): MyTutor {
  return {
    relationship_id: tutor.relationship_id,
    state: tutor.state,
    colour: tutor.colour,
    channel_id: tutor.channel_id,
    account_closed_until: null,
    ended_at: null,
    ended_by: null,
    tutor: { name: tutor.name, avatar_id: tutor.avatar_id, subjects: tutor.subjects, bio: '' },
  };
}

function latestStudentHome(queryClient: QueryClient): StudentHome | undefined {
  let latest: { readonly home: StudentHome; readonly at: number } | undefined;
  for (const query of queryClient.getQueryCache().findAll({ queryKey: studentHomesKey })) {
    const home = queryClient.getQueryData<StudentHome>(query.queryKey);
    if (home !== undefined && (latest === undefined || query.state.dataUpdatedAt > latest.at)) {
      latest = { home, at: query.state.dataUpdatedAt };
    }
  }
  return latest?.home;
}

function FirstTutor(): ReactElement {
  const { t } = useTranslation();
  const query = useSentRequests();
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

function StudentTutors({ tutors }: { readonly tutors: readonly MyTutor[] }): ReactElement {
  const { t } = useTranslation();
  if (tutors.length === 0) {
    return <FirstTutor />;
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

interface Refusal {
  readonly entry: string;
  readonly value: string;
}

export function HomePage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const queryClient = useQueryClient();
  const tutorsHeading = useId();
  const location = useLocation();
  const [params, setParams] = useSearchParams();
  const requested = params.get('tutor');
  const [refusal, setRefusal] = useState<Refusal | null>(null);
  const refusedHere = requested !== null && refusal?.entry === location.key && refusal.value === requested;
  const known = latestStudentHome(queryClient);
  const unknownWithoutRead =
    requested !== null &&
    known !== undefined &&
    known.tutors_count <= known.tutors.length &&
    !known.tutors.some((tutor) => tutor.relationship_id === requested);
  const unavailable = refusedHere || unknownWithoutRead;
  const sent = unavailable ? null : requested;
  const query = useStudentHomeRead(sent);
  if (sent !== null && query.isError && asApiError(query.error).code === 'not_found' && !refusedHere) {
    setRefusal({ entry: location.key, value: sent });
  }
  const [lastHome, setLastHome] = useState<StudentHome | undefined>(undefined);
  if (query.data !== undefined && query.data !== lastHome) {
    setLastHome(query.data);
  }
  useEffect(() => {
    if (unavailable) {
      focusHeadingIfLost();
    }
  }, [unavailable]);
  const homeworkShown = reachable(me, '/homework', provisionallyLinked(useProvisionalLink(me)));
  const home = query.data ?? lastHome;
  const choiceFailed = query.data === undefined && query.isError && !unavailable;
  const read = {
    data: choiceFailed ? undefined : home,
    error: query.error,
    isError: query.isError,
    refetch: query.refetch,
  };
  const tutors = home === undefined ? [] : home.tutors.map(asMyTutor);
  const matched = unavailable ? null : requested;
  const scheduleAddress =
    me.state !== 'active' && tutors.length === 0
      ? null
      : matched === null
        ? '/schedule'
        : `/schedule?${new URLSearchParams({ tutor: matched }).toString()}`;
  return (
    <Page titleSize="display">
      <p className="-mt-4 text-text-muted">
        {home === undefined || unavailable || choiceFailed
          ? ' '
          : t('home.summary', { count: home.today_lessons_count })}
      </p>
      {home === undefined ? (
        <HomeFirstLoad query={read} />
      ) : (
        <PageColumns
          asideLabel={t('home.aside')}
          main={
            <>
              <HomeRefetchNotice query={read} />
              {tutors.length >= 2 ? (
                <TutorFilter
                  tutors={tutors}
                  chosen={requested}
                  onChoose={(tutor) => {
                    if (tutor !== requested) {
                      setParams(tutor === null ? {} : { tutor });
                    }
                  }}
                />
              ) : null}
              {unavailable ? (
                <div className="flex flex-col items-start gap-2">
                  <p>{t('home.tutorUnavailable')}</p>
                  <Link to="/home" className={linkClass}>
                    {t('home.allTutors')}
                  </Link>
                </div>
              ) : choiceFailed ? (
                <HomeFirstLoad query={query} />
              ) : (
                <>
                  <NextLesson me={me} item={home.next_lesson} />
                  {homeworkShown ? (
                    <>
                      <TodoPanel part={home.todo} tutor={matched} />
                      <CheckedPanel items={home.recently_checked} tutor={matched} />
                    </>
                  ) : null}
                </>
              )}
            </>
          }
          aside={
            <>
              {unavailable || choiceFailed ? null : (
                <NextSevenDays me={me} items={home.next_seven_days} scheduleAddress={scheduleAddress} />
              )}
              <section aria-labelledby={tutorsHeading} className="flex flex-col gap-3">
                <h2 id={tutorsHeading} className="text-heading">
                  {t('section.tutors')}
                </h2>
                <StudentTutors tutors={tutors} />
              </section>
            </>
          }
        />
      )}
    </Page>
  );
}
