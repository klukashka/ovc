import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

export const mainContentId = 'main-content';

export function SkipLink(): ReactElement {
  const { t } = useTranslation();
  return (
    <a
      href={`#${mainContentId}`}
      onClick={(event) => {
        event.preventDefault();
        document.getElementById(mainContentId)?.focus();
      }}
      className="sr-only rounded-md bg-surface px-4 py-2 text-strong text-text focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:inline-flex focus:min-h-11 focus:items-center"
    >
      {t('frame.skipToContent')}
    </a>
  );
}
