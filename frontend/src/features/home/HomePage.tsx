import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { Page } from '../../components/ui/Page.tsx';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { TutorList } from '../tutors/TutorList.tsx';
import { Readiness } from './Readiness.tsx';

export function HomePage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const tutorsHeading = useId();
  return (
    <Page>
      {me.role === 'student' ? (
        <section aria-labelledby={tutorsHeading} className="flex flex-col gap-3">
          <h2 id={tutorsHeading} className="text-heading">
            {t('section.tutors')}
          </h2>
          <TutorList withBio={false} />
        </section>
      ) : (
        <p className="text-text-muted">{t('app.placeholder')}</p>
      )}
      <Readiness />
    </Page>
  );
}
