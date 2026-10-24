import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { DelayedLoading } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type MyTutor, currentTutors, pastTutors } from './api.ts';
import { CodeEntry } from './CodeEntry.tsx';
import { SentRequests } from './SentRequests.tsx';
import { TutorList, TutorsLoadFailed, useMyTutors } from './TutorList.tsx';

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
      <TutorList tutors={tutors} label={t('tutors.past')} withBio={false} />
    </section>
  );
}

export function MyTutorsPage(): ReactElement {
  const { t } = useTranslation();
  const addHeading = useId();
  const query = useMyTutors();

  function current(): ReactElement {
    if (query.isPending) {
      return <DelayedLoading />;
    }
    if (query.isError) {
      return <TutorsLoadFailed query={query} />;
    }
    const tutors = currentTutors(query.data);
    return tutors.length === 0 ? (
      <p className="text-text-muted">{t('tutors.empty')}</p>
    ) : (
      <TutorList tutors={tutors} label={t('tutors.list')} withBio />
    );
  }

  return (
    <Page>
      {current()}
      <section aria-labelledby={addHeading} className="flex flex-col gap-3">
        <h2 id={addHeading} className="text-heading">
          {t('tutors.add')}
        </h2>
        <CodeEntry label={t('tutors.code')} waiting={false} replaceWhenSent={false} />
      </section>
      <SentRequests />
      <PastTutors tutors={query.isSuccess ? pastTutors(query.data) : []} />
    </Page>
  );
}
