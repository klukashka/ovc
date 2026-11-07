import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import { type ReactElement, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { FormAlert, FormNotice, quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { useSettledOnce } from '../../components/ui/settledOnce.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { type CopyResult, copyText } from '../../lib/clipboard.ts';
import { type TutorCode as TutorCodeValue, fetchTutorCode, tutorCodeKey } from '../settings/tutorCode.ts';
import { JoinRequests, useJoinRequestsSettled } from '../students/JoinRequests.tsx';

type CopyNotice = { readonly result: CopyResult; readonly count: number } | null;

function useTutorCode(): UseQueryResult<TutorCodeValue> {
  const client = useApiClient();
  return useQuery({
    queryKey: tutorCodeKey,
    queryFn: () => fetchTutorCode(client),
    retry: false,
  });
}

function TutorCode(): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const [notice, setNotice] = useState<CopyNotice>(null);
  const query = useTutorCode();

  function copy(code: string): void {
    function show(result: CopyResult): void {
      setNotice((previous) => ({ result, count: (previous?.count ?? 0) + 1 }));
    }
    copyText(navigator.clipboard, code).then(show, () => {
      show('failed');
    });
  }

  function body(): ReactElement {
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
    const code = query.data;
    return (
      <>
        <p className="font-mono text-heading tracking-wider">{code.code}</p>
        <p className="text-small text-text-muted">{t(code.active ? 'joinRequests.codeHint' : 'teaching.inactive')}</p>
        {notice === null ? null : notice.result === 'copied' ? (
          <FormNotice key={notice.count} takeFocus>
            {t('teaching.copied')}
          </FormNotice>
        ) : (
          <FormAlert key={notice.count}>{t('teaching.copyFailed')}</FormAlert>
        )}
        <p>
          <button
            type="button"
            onClick={() => {
              copy(code.code);
            }}
            className={quietButtonClass}
          >
            {t('teaching.copy')}
          </button>
        </p>
      </>
    );
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-2 border-t border-border pt-4">
      <h3 id={headingId} className="text-strong">
        {t('joinRequests.code')}
      </h3>
      {body()}
    </section>
  );
}

export function TodayPage(): ReactElement {
  const requestsSettled = useJoinRequestsSettled();
  const code = useTutorCode();
  const settled = useSettledOnce(requestsSettled && !code.isPending);
  return (
    <Page>
      <JoinRequests hideWhenEmpty={false}>
        {requestsSettled && !settled ? <DelayedLoading /> : null}
        <div hidden={!settled} className="contents">
          <TutorCode />
        </div>
      </JoinRequests>
    </Page>
  );
}
