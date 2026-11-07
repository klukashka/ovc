import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { asApiError } from '../../lib/api/client.ts';
import { currentTutors } from '../tutors/api.ts';
import { CodeEntry } from '../tutors/CodeEntry.tsx';
import { type SentRequestsQuery, hasPendingRequest, useSentRequests } from '../tutors/SentRequests.tsx';
import { TutorList, TutorsLoadFailed, useMyTutors } from '../tutors/TutorList.tsx';

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

export function HomePage(): ReactElement {
  const { t } = useTranslation();
  const tutorsHeading = useId();
  return (
    <Page>
      <section aria-labelledby={tutorsHeading} className="flex flex-col gap-3">
        <h2 id={tutorsHeading} className="text-heading">
          {t('section.tutors')}
        </h2>
        <StudentTutors />
      </section>
    </Page>
  );
}
