import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { LoadFailed } from '../../components/ui/LoadState.tsx';
import { asApiError } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { reachable } from '../../routes/access.ts';
import type { ProvisionalLink } from '../../routes/provisionalLink.ts';
import { homeworkPanelTextLoaders } from './panelTexts.ts';

export function UnconfirmedSections({
  me,
  link,
}: {
  readonly me: Me;
  readonly link: ProvisionalLink;
}): ReactElement | null {
  const { t } = useTranslation();
  const headingId = useId();
  const texts = useFeatureTexts('homeworkPanels', homeworkPanelTextLoaders);
  if (link.kind !== 'known' || reachable(me, '/notifications', link.linked)) {
    return null;
  }
  if (!texts.ready) {
    return texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : null;
  }
  return (
    <section aria-labelledby={headingId} className="flex max-w-running-text flex-col gap-2">
      <h2 id={headingId} className="text-heading">
        {t('homeworkPanels.unconfirmed.heading')}
      </h2>
      <p>{link.linked ? t('homeworkPanels.unconfirmed.withLink') : t('homeworkPanels.unconfirmed.withoutLink')}</p>
    </section>
  );
}
