import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { FormAlert, FormNotice, quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { type ApiClient, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { type CopyResult, copyText } from '../../lib/clipboard.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { SettingsNav } from './SettingsNav.tsx';
import { fetchTutorCode, tutorCodeKey } from './tutorCode.ts';

type BlockedRequesterPage = components['schemas']['BlockedRequesterPage'];
type BlockedRequester = components['schemas']['BlockedRequester'];

const blockedKey = ['tutor', 'blocked-requesters'] as const;

async function fetchBlockedPage(
  client: ApiClient,
  cursor: string | null,
  signal: AbortSignal,
): Promise<BlockedRequesterPage> {
  const { data, error, response } = await client.GET('/api/v1/tutor/blocked-requesters', {
    params: { query: cursor === null ? {} : { cursor } },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

type CodeNotice = 'none' | 'copied' | 'copyFailed' | 'regenerated';

function CodeNoticeMessage({ notice }: { readonly notice: CodeNotice }): ReactElement | null {
  const { t } = useTranslation();
  switch (notice) {
    case 'none':
      return null;
    case 'copyFailed':
      return <FormAlert>{t('teaching.copyFailed')}</FormAlert>;
    case 'copied':
    case 'regenerated':
      return <FormNotice takeFocus>{t(`teaching.${notice}`)}</FormNotice>;
  }
}

function TutorCodeSection(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const headingId = useId();
  const action = useAction();
  const [notice, setNotice] = useState<CodeNotice>('none');
  const query = useQuery({
    queryKey: tutorCodeKey,
    queryFn: ({ signal }) => fetchTutorCode(client, signal),
    retry: false,
  });

  function regenerate(): void {
    setNotice('none');
    action.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/tutor/code/regenerate');
      if (data === undefined) {
        return refusedBy(response, error);
      }
      queryClient.setQueryData(tutorCodeKey, data);
      setNotice('regenerated');
      return done;
    });
  }

  function copy(code: string): void {
    action.reset();
    copyText(navigator.clipboard, code).then(
      (result: CopyResult) => {
        setNotice(result === 'copied' ? 'copied' : 'copyFailed');
      },
      () => {
        setNotice('copyFailed');
      },
    );
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
        <p className="font-mono text-title tracking-wider">{code.code}</p>
        <p className="text-small text-text-muted">{t('teaching.codeHint')}</p>
        <Refusal state={action.state} dailyLimit="code" confirmation="regenerateCode" />
        <CodeNoticeMessage notice={notice} />
        <div className="flex flex-wrap gap-3">
          <button
            type="button"
            onClick={() => {
              copy(code.code);
            }}
            className={quietButtonClass}
          >
            {t('teaching.copy')}
          </button>
          {code.active ? (
            <button
              type="button"
              disabled={action.busy}
              aria-busy={action.busy}
              onClick={regenerate}
              className={quietButtonClass}
            >
              {t('teaching.regenerate')}
            </button>
          ) : null}
        </div>
        {code.active ? (
          <p className="text-small text-text-muted">{t('teaching.regenerateHint')}</p>
        ) : (
          <>
            <p className="text-small text-text-muted">{t('teaching.inactive')}</p>
            <ConfirmationPrompt reason="regenerateCode" />
          </>
        )}
      </>
    );
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('teaching.code')}
      </h2>
      {body()}
    </section>
  );
}

function BlockedRow({
  student,
  onUnblock,
  busy,
}: {
  readonly student: BlockedRequester;
  readonly onUnblock: () => void;
  readonly busy: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  return (
    <li className="flex flex-wrap items-center gap-3 border-t border-border py-3">
      <Avatar name={student.name} avatarId={student.avatar_id} />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-strong">{student.name}</span>
        <span className="tabular text-small text-text-muted">
          {t('teaching.blockedAt', { date: formatDate(student.blocked_at, me.language, me.timezone) })}
        </span>
      </span>
      <button
        type="button"
        disabled={busy}
        aria-label={t('teaching.unblockNamed', { name: student.name })}
        onClick={onUnblock}
        className={quietButtonClass}
      >
        {t('teaching.unblock')}
      </button>
    </li>
  );
}

function BlockedStudentsSection(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const headingId = useId();
  const action = useAction();
  const query = useInfiniteQuery({
    queryKey: blockedKey,
    queryFn: ({ pageParam, signal }) => fetchBlockedPage(client, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
  const students = query.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(students.length, ':scope > li button');

  function unblock(studentId: string): void {
    action.run(async (): Promise<ActionState> => {
      const { error, response } = await client.DELETE('/api/v1/tutor/blocked-requesters/{studentId}', {
        params: { path: { studentId } },
      });
      if (!response.ok && response.status !== 404) {
        return refusedBy(response, error);
      }
      await queryClient.invalidateQueries({ queryKey: blockedKey });
      return done;
    });
  }

  function body(): ReactElement {
    if (query.isPending) {
      return <DelayedLoading />;
    }
    if (query.isError) {
      const error = asApiError(query.error);
      return error.code === 'email_not_verified' ? (
        <ConfirmationPrompt reason="blockedStudents" />
      ) : (
        <LoadFailed
          error={error}
          onRetry={() => {
            void query.refetch();
          }}
        />
      );
    }
    return (
      <>
        <Refusal state={action.state} />
        {action.state.kind === 'done' ? <FormNotice takeFocus>{t('teaching.unblocked')}</FormNotice> : null}
        {students.length === 0 ? (
          <p className="text-text-muted">{t('teaching.blockedEmpty')}</p>
        ) : (
          <ul ref={listRef} className="border-b border-border">
            {students.map((student) => (
              <BlockedRow
                key={student.student_id}
                student={student}
                busy={action.busy}
                onUnblock={() => {
                  unblock(student.student_id);
                }}
              />
            ))}
          </ul>
        )}
        {query.hasNextPage ? (
          <p>
            <button
              type="button"
              aria-disabled={query.isFetchingNextPage}
              aria-busy={query.isFetchingNextPage}
              onClick={() => {
                if (!query.isFetchingNextPage) {
                  loadingMore();
                  void query.fetchNextPage();
                }
              }}
              className={quietButtonClass}
            >
              {t('teaching.showMore')}
            </button>
          </p>
        ) : null}
      </>
    );
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('teaching.blocked')}
      </h2>
      <p className="text-small text-text-muted">{t('teaching.blockedHint')}</p>
      {body()}
    </section>
  );
}

export function TeachingSettingsPage(): ReactElement {
  return (
    <Page>
      <SettingsNav />
      <TutorCodeSection />
      <BlockedStudentsSection />
    </Page>
  );
}
