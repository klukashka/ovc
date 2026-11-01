import { useQuery } from '@tanstack/react-query';
import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';

export function StorageUsage(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const query = useQuery({
    queryKey: ['tutor', 'settings'],
    queryFn: async ({ signal }) => {
      const { data, error, response } = await client.GET('/api/v1/tutor/settings', { signal });
      if (data === undefined) throw apiErrorFrom(response.status, error);
      return data;
    },
    retry: false,
  });
  return (
    <section className="flex flex-col gap-3" aria-label={t('teaching.storage')}>
      <h2 className="text-heading">{t('teaching.storage')}</h2>
      {query.isPending ? <DelayedLoading /> : null}
      {query.isError ? (
        <LoadFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : null}
      {query.data === undefined ? null : (
        <>
          <meter
            min={0}
            max={Math.max(1, query.data.file_quota_bytes)}
            value={query.data.file_usage_bytes}
            aria-label={t('teaching.files')}
            className="w-full"
          />
          <p>{t('teaching.storageBytes', { used: query.data.file_usage_bytes, quota: query.data.file_quota_bytes })}</p>
        </>
      )}
    </section>
  );
}
