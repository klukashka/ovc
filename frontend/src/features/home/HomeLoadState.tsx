import type { UseQueryResult } from '@tanstack/react-query';
import type { ReactElement } from 'react';

import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { asApiError } from '../../lib/api/client.ts';

export type HomeRead<Data> = Pick<UseQueryResult<Data>, 'data' | 'error' | 'isError' | 'refetch'>;

export function HomeFirstLoad<Data>({ query }: { readonly query: HomeRead<Data> }): ReactElement {
  if (!query.isError) {
    return <DelayedLoading />;
  }
  return (
    <LoadFailed
      error={asApiError(query.error)}
      onRetry={() => {
        void query.refetch();
      }}
    />
  );
}

export function HomeRefetchNotice<Data>({ query }: { readonly query: HomeRead<Data> }): ReactElement | null {
  if (query.data === undefined || !query.isError) {
    return null;
  }
  return (
    <RefetchFailed
      error={asApiError(query.error)}
      onRetry={() => {
        void query.refetch();
      }}
    />
  );
}
