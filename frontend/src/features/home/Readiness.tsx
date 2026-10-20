import { useQuery } from '@tanstack/react-query';
import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { fetchReadiness, useApiClient } from '../../lib/api/client.ts';

type ReadinessState = 'checking' | 'ready' | 'unavailable';

const dotColour: Readonly<Record<ReadinessState, string>> = {
  checking: 'bg-text-muted',
  ready: 'bg-success',
  unavailable: 'bg-danger',
};

const textColour: Readonly<Record<ReadinessState, string>> = {
  checking: 'text-text-muted',
  ready: 'text-success',
  unavailable: 'text-danger',
};

export function Readiness(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const query = useQuery({
    queryKey: ['health', 'ready'],
    queryFn: ({ signal }) => fetchReadiness(client, signal),
    retry: false,
    refetchInterval: 30_000,
  });
  const state: ReadinessState = query.isSuccess ? 'ready' : query.isError ? 'unavailable' : 'checking';

  return (
    <section aria-labelledby="readiness-label" className="flex flex-col gap-2 border-t border-border pt-4">
      <h2 id="readiness-label" className="text-label text-text-muted">
        {t('readiness.label')}
      </h2>
      <p role="status" data-readiness={state} className={`flex items-center gap-2 text-strong ${textColour[state]}`}>
        <span aria-hidden="true" className={`size-2 rounded-full ${dotColour[state]}`} />
        {t(`readiness.${state}`)}
      </p>
    </section>
  );
}
