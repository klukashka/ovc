import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { ActionMenu } from '../../components/ui/ActionMenu.tsx';
import { Avatar } from '../../components/ui/Avatar.tsx';
import {
  DataTable,
  type TableColumn,
  endCellClass,
  narrowOnlyClass,
  primaryCellClass,
  secondaryCellClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormActions,
  FormAlert,
  FormNotice,
  dangerFilledButtonClass,
  quietButtonClass,
} from '../../components/ui/Form.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { type ApiClient, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { type CopyResult, copyText } from '../../lib/clipboard.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { StorageUsage } from './StorageUsage.tsx';
import { SettingsNav } from './SettingsNav.tsx';
import { SettingsSection } from './SettingsRow.tsx';
import { fetchTutorCode, tutorCodeKey } from './tutorCode.ts';

type BlockedRequesterPage = components['schemas']['BlockedRequesterPage'];
type BlockedRequester = components['schemas']['BlockedRequester'];

const blockedKey = ['tutor', 'blocked-requesters'] as const;

async function fetchBlockedPage(client: ApiClient, cursor: string | null): Promise<BlockedRequesterPage> {
  const { data, error, response } = await client.GET('/api/v1/tutor/blocked-requesters', {
    params: { query: cursor === null ? {} : { cursor } },
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
  const action = useAction();
  const [notice, setNotice] = useState<CodeNotice>('none');
  const [confirmingCode, setConfirmingCode] = useState<string | null>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const keepCode = useRef<HTMLButtonElement>(null);
  const dialogHeadingId = useId();
  const consequenceId = useId();
  const query = useQuery({
    queryKey: tutorCodeKey,
    queryFn: () => fetchTutorCode(client),
    retry: false,
  });
  const shownCode = query.data?.active === true ? query.data.code : null;
  if (confirmingCode !== null && confirmingCode !== shownCode && !action.busy) {
    setConfirmingCode(null);
  }

  function closeConfirmation(): void {
    action.reset();
    setConfirmingCode(null);
  }

  function regenerate(): void {
    setNotice('none');
    action.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/tutor/code/regenerate');
      if (data === undefined) {
        return refusedBy(response, error);
      }
      setConfirmingCode(null);
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
        {confirmingCode === null ? (
          <Refusal state={action.state} dailyLimit="code" confirmation="regenerateCode" />
        ) : null}
        <ModalDialog
          open={confirmingCode !== null}
          busy={action.busy}
          labelledBy={dialogHeadingId}
          describedBy={consequenceId}
          returnFocus={menuButton}
          initialFocus={keepCode}
          onClose={closeConfirmation}
        >
          <div className="flex flex-col gap-4">
            <h2 id={dialogHeadingId} className="text-heading">
              {t('teaching.regenerateTitle')}
            </h2>
            <p id={consequenceId}>{t('teaching.regenerateConsequence')}</p>
            <Refusal state={action.state} dailyLimit="code" confirmation="regenerateCode" />
            <FormActions>
              <ActionButton busy={action.busy} onClick={regenerate} className={dangerFilledButtonClass}>
                {t('teaching.regenerate')}
              </ActionButton>
              <ActionButton
                ref={keepCode}
                busy={false}
                unavailable={action.busy}
                onClick={closeConfirmation}
                className={quietButtonClass}
              >
                {t('teaching.cancel')}
              </ActionButton>
            </FormActions>
          </div>
        </ModalDialog>
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
            <ActionMenu
              label={t('teaching.codeActions')}
              busy={action.busy && confirmingCode === null}
              unavailable={action.busy}
              buttonRef={menuButton}
              items={[
                {
                  label: t('teaching.regenerate'),
                  danger: true,
                  onChoose: (): void => {
                    setNotice('none');
                    action.reset();
                    setConfirmingCode(code.code);
                  },
                },
              ]}
            >
              <MoreIcon />
            </ActionMenu>
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

  return <SettingsSection heading={t('teaching.code')}>{body()}</SettingsSection>;
}

function BlockedRow({
  student,
  onUnblock,
  busy,
  unavailable,
}: {
  readonly student: BlockedRequester;
  readonly onUnblock: () => void;
  readonly busy: boolean;
  readonly unavailable: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const name = student.name ?? t('teaching.deletedAccount');
  const blockedDate = formatDate(student.blocked_at, me.language, me.timezone);
  const blockedAt = t('teaching.blockedAt', { date: blockedDate });
  const unblockLabel =
    student.name === null
      ? t('teaching.unblockDeleted', { date: blockedDate })
      : t('teaching.unblockNamed', { name: student.name });
  return (
    <tr data-row={student.student_id} className={tableRowClass}>
      <th scope="row" className={primaryCellClass}>
        <span className="flex min-w-0 items-center gap-3">
          <Avatar name={name} avatarId={student.avatar_id} />
          <span className="flex min-w-0 flex-col">
            <span dir="auto" className={student.name === null ? 'text-text-muted' : 'text-strong'}>
              {name}
            </span>
            <span className={`tabular text-small text-text-muted ${narrowOnlyClass}`}>{blockedAt}</span>
          </span>
        </span>
      </th>
      <td className={secondaryCellClass}>
        <span className="tabular text-small text-text-muted">{blockedAt}</span>
      </td>
      <td className={endCellClass}>
        <ActionButton
          busy={busy}
          unavailable={unavailable}
          label={unblockLabel}
          onClick={onUnblock}
          className={quietButtonClass}
        >
          {t('teaching.unblock')}
        </ActionButton>
      </td>
    </tr>
  );
}

function BlockedStudentsSection(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const action = useAction();
  const [unblocking, setUnblocking] = useState<string | null>(null);
  const [refusals, setRefusals] = useState(0);
  const query = useInfiniteQuery({
    queryKey: blockedKey,
    queryFn: ({ pageParam }) => fetchBlockedPage(client, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
  const students = query.data?.pages.flatMap((page) => page.items) ?? [];
  const blockedColumns: readonly TableColumn[] = [
    { key: 'student', label: t('teaching.columns.student'), width: 'w-2/3 tablet:w-1/2' },
    { key: 'blocked', label: t('teaching.columns.blocked'), secondary: true, width: 'w-1/4' },
    { key: 'actions', label: t('teaching.columns.actions'), visuallyHidden: true, width: 'w-1/3 tablet:w-1/4' },
  ];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLTableSectionElement>(
    students.length,
    ':scope > tr button',
    query.isFetching,
  );

  function unblock(studentId: string): void {
    if (action.busy) {
      return;
    }
    setUnblocking(studentId);
    action.run(async (): Promise<ActionState> => {
      const { error, response } = await client.DELETE('/api/v1/tutor/blocked-requesters/{studentId}', {
        params: { path: { studentId } },
      });
      const settled = response.ok ? done : refusedBy(response, error);
      if (settled.kind === 'refused') {
        setRefusals((count) => count + 1);
      }
      if (settled.kind === 'done' || showsStaleData(settled)) {
        await queryClient.invalidateQueries({ queryKey: blockedKey });
      }
      return settled;
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
        {showsStaleData(action.state) ? (
          <FormAlert key={refusals}>{t('teaching.unblockChanged')}</FormAlert>
        ) : (
          <Refusal state={action.state} />
        )}
        {action.state.kind === 'done' ? <FormNotice takeFocus>{t('teaching.unblocked')}</FormNotice> : null}
        {students.length === 0 ? (
          <p className="text-text-muted">{t('teaching.blockedEmpty')}</p>
        ) : (
          <DataTable label={t('teaching.blocked')} columns={blockedColumns} bodyRef={listRef}>
            {students.map((student) => (
              <BlockedRow
                key={student.student_id}
                student={student}
                busy={action.busy && unblocking === student.student_id}
                unavailable={action.busy}
                onUnblock={() => {
                  unblock(student.student_id);
                }}
              />
            ))}
          </DataTable>
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
                  void loadNextPage(query);
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
    <SettingsSection heading={t('teaching.blocked')} description={t('teaching.blockedHint')}>
      {body()}
    </SettingsSection>
  );
}

export function TeachingSettingsPage(): ReactElement {
  return (
    <Page>
      <SettingsNav />
      <div className="flex flex-col border-b border-border">
        <TutorCodeSection />
        <StorageUsage />
        <BlockedStudentsSection />
      </div>
    </Page>
  );
}
