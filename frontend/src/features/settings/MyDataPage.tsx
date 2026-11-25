import { CancelledError, type QueryObserverResult, useQuery, useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import {
  type ReactElement,
  type RefObject,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react';
import { useTranslation } from 'react-i18next';

import { useScheduler } from '../../app/clock.ts';
import { useVisibility } from '../../app/visibility.ts';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import {
  ActionButton,
  FormActions,
  FormAlert,
  dangerButtonClass,
  errorMessage,
  primaryButtonClass,
  quietButtonClass,
  retryAfterSeconds,
} from '../../components/ui/Form.tsx';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed, focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSignedInAccount } from '../../lib/api/session.ts';
import { startDownload, storageDownloadAddress } from '../../lib/files/startDownload.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import {
  type CurrentExport,
  type Export,
  currentExportKey,
  exportInProgress,
  exportRefetchMilliseconds,
  fetchCurrentExport,
} from './exportApi.ts';
import { myDataTextLoaders } from './myDataTexts.ts';
import { RequestExportDialog } from './RequestExportDialog.tsx';
import { SettingsNav } from './SettingsNav.tsx';
import { SettingsSection } from './SettingsRow.tsx';
import { formatStorage } from './storageAmount.ts';

type ExportAction = 'cancel' | 'delete' | 'download';

type ActionOutcome =
  | { readonly kind: 'idle' }
  | { readonly kind: 'busy'; readonly action: ExportAction }
  | { readonly kind: 'refused'; readonly message: string; readonly shown: string | null };

type ActionResult = { readonly kind: 'idle' } | { readonly kind: 'refused'; readonly message: string };

interface Notice {
  readonly id: number;
  readonly message: string;
}

const idle = { kind: 'idle' } as const;

function useProgressRefetch(query: QueryObserverResult<CurrentExport>, refetch: () => void): void {
  const scheduler = useScheduler();
  const visibility = useVisibility();
  const visible = useSyncExternalStore(visibility.subscribe, visibility.visible);
  const following = exportInProgress(query.data?.export ?? null);
  const { dataUpdatedAt, errorUpdatedAt } = query;

  useEffect(() => {
    if (!following || !visible) {
      return undefined;
    }
    return scheduler.schedule(exportRefetchMilliseconds, () => {
      if (visibility.visible()) {
        refetch();
      }
    });
  }, [following, visible, dataUpdatedAt, errorUpdatedAt, scheduler, visibility, refetch]);
}

function useRequestAllowed(nextAllowedAt: string | null): boolean {
  const scheduler = useScheduler();
  const [, setPassed] = useState(0);
  const due = nextAllowedAt === null ? null : Date.parse(nextAllowedAt);
  const allowed = due === null || due <= scheduler.wallClock();

  useEffect(() => {
    if (due === null || allowed) {
      return undefined;
    }
    return scheduler.schedule(due - scheduler.wallClock(), () => {
      setPassed((count) => count + 1);
    });
  }, [due, allowed, scheduler]);

  return allowed;
}

function shownStateOf(current: CurrentExport | undefined): string | null {
  if (current === undefined) {
    return null;
  }
  return current.export === null ? 'none' : `${current.export.id}:${current.export.state}`;
}

function useFocusOnShownStateChange(shown: string | null): void {
  const previous = useRef(shown);

  useEffect(() => {
    const before = previous.current;
    previous.current = shown;
    if (before === null || before === shown) {
      return;
    }
    focusHeadingIfLost();
  }, [shown]);
}

function RequestControl({
  me,
  nextAllowedAt,
  button,
  onRequest,
}: {
  readonly me: Me;
  readonly nextAllowedAt: string | null;
  readonly button: RefObject<HTMLButtonElement | null>;
  readonly onRequest: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const allowed = useRequestAllowed(nextAllowedAt);
  const reasonId = useId();
  const reason =
    allowed || nextAllowedAt === null
      ? null
      : t('myData.requestAfter', { time: formatDateTime(nextAllowedAt, me.language, me.timezone) });
  return (
    <div className="flex flex-col items-start gap-2">
      <button
        ref={button}
        type="button"
        aria-disabled={reason === null ? undefined : 'true'}
        aria-describedby={reason === null ? undefined : reasonId}
        onClick={(event) => {
          if (reason !== null) {
            event.preventDefault();
            return;
          }
          onRequest();
        }}
        className={primaryButtonClass}
      >
        {t('myData.request')}
      </button>
      {reason === null ? null : (
        <p id={reasonId} className="text-small text-text-muted">
          {reason}
        </p>
      )}
    </div>
  );
}

function ExportState({
  me,
  current,
  outcome,
  requestButton,
  onRequest,
  onRemove,
  onDownload,
}: {
  readonly me: Me;
  readonly current: CurrentExport;
  readonly outcome: ActionOutcome;
  readonly requestButton: RefObject<HTMLButtonElement | null>;
  readonly onRequest: () => void;
  readonly onRemove: (action: 'cancel' | 'delete') => void;
  readonly onDownload: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const shown: Export | null = current.export;
  const busy = outcome.kind === 'busy';
  const busyWith = (action: ExportAction): boolean => outcome.kind === 'busy' && outcome.action === action;
  const request = (
    <RequestControl me={me} nextAllowedAt={current.next_allowed_at} button={requestButton} onRequest={onRequest} />
  );

  if (shown === null) {
    return (
      <>
        <p>{t('myData.none')}</p>
        {request}
      </>
    );
  }
  switch (shown.state) {
    case 'queued':
    case 'building':
      return (
        <>
          <p>{t(shown.state === 'queued' ? 'myData.queued' : 'myData.building')}</p>
          {shown.progress === null || shown.progress.total === 0 ? null : (
            <div className="flex w-full max-w-sm flex-col gap-1">
              <progress
                max={shown.progress.total}
                value={shown.progress.done}
                aria-label={t('myData.building')}
                className="w-full"
              />
              <p className="text-small text-text-muted">
                {t('myData.progress', { done: shown.progress.done, total: shown.progress.total })}
              </p>
            </div>
          )}
          <ActionButton
            busy={busyWith('cancel')}
            unavailable={busy}
            onClick={() => {
              onRemove('cancel');
            }}
            className={quietButtonClass}
          >
            {t('myData.cancelExport')}
          </ActionButton>
        </>
      );
    case 'ready':
      return (
        <>
          <p>{t('myData.ready')}</p>
          {shown.size_bytes === null ? null : (
            <p>{t('myData.size', { size: formatStorage(shown.size_bytes, me.language) })}</p>
          )}
          {shown.expires_at === null ? null : (
            <p>{t('myData.expires', { time: formatDateTime(shown.expires_at, me.language, me.timezone) })}</p>
          )}
          <FormActions>
            <ActionButton
              busy={busyWith('download')}
              unavailable={busy}
              onClick={onDownload}
              className={primaryButtonClass}
            >
              {t('myData.download')}
            </ActionButton>
            <ActionButton
              busy={busyWith('delete')}
              unavailable={busy}
              onClick={() => {
                onRemove('delete');
              }}
              className={dangerButtonClass}
            >
              {t('myData.deleteExport')}
            </ActionButton>
          </FormActions>
        </>
      );
    case 'failed':
      return (
        <>
          <p>{t(`myData.failed.${shown.failure ?? 'error'}`)}</p>
          {request}
        </>
      );
    case 'expired':
      return (
        <>
          <p>{t('myData.expired')}</p>
          {request}
        </>
      );
  }
}

function refusalMessage(t: TFunction, error: ApiError, retryAfter: number | null): string {
  return error.code === 'service_unavailable' ? t('myData.busy') : errorMessage(t, error, retryAfter);
}

function ExportSection({ me }: { readonly me: Me }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const requestButton = useRef<HTMLButtonElement>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [outcome, setOutcome] = useState<ActionOutcome>(idle);
  const [notice, setNotice] = useState<Notice | null>(null);
  const query = useQuery({
    queryKey: currentExportKey,
    queryFn: () => fetchCurrentExport(client),
    retry: false,
  });
  const refetch = useCallback((): void => {
    void queryClient.invalidateQueries({ queryKey: currentExportKey });
  }, [queryClient]);
  useProgressRefetch(query, refetch);
  const shown = shownStateOf(query.data);
  const mounted = useRef(true);
  useFocusOnShownStateChange(shown);

  function stillMounted(): boolean {
    return mounted.current;
  }

  useEffect(() => {
    mounted.current = true;
    return (): void => {
      mounted.current = false;
      void queryClient.cancelQueries({ queryKey: currentExportKey });
    };
  }, [queryClient]);

  function announce(message: string): void {
    setNotice((previous) => ({ id: (previous?.id ?? 0) + 1, message }));
  }

  function act(action: ExportAction, work: () => Promise<ActionResult>): void {
    const shownAtStart = shown;
    setNotice(null);
    setOutcome({ kind: 'busy', action });
    work()
      .then((result) => {
        if (mounted.current) {
          setOutcome(result.kind === 'refused' ? { ...result, shown: shownAtStart } : result);
        }
      })
      .catch((thrown: unknown) => {
        console.error('export_action_failed', thrown instanceof Error ? thrown.name : typeof thrown);
        if (mounted.current) {
          setOutcome({ kind: 'refused', message: errorMessage(t, asApiError(thrown), null), shown: shownAtStart });
        }
      });
  }

  async function readAgainAndAnnounce(message: string): Promise<void> {
    await queryClient.cancelQueries({ queryKey: currentExportKey });
    if (!stillMounted()) {
      return;
    }
    await queryClient
      .query({ queryKey: currentExportKey, queryFn: () => fetchCurrentExport(client), staleTime: 0, retry: false })
      .catch((thrown: unknown) => {
        if (stillMounted() && !(thrown instanceof CancelledError)) {
          console.error('export_read_after_action_failed', thrown instanceof Error ? thrown.name : typeof thrown);
        }
      });
    if (stillMounted()) {
      announce(message);
    }
  }

  function remove(action: 'cancel' | 'delete'): void {
    act(action, async () => {
      const { error, response } = await client.DELETE('/api/v1/exports/current');
      if (!response.ok) {
        return {
          kind: 'refused',
          message: refusalMessage(t, apiErrorFrom(response.status, error), retryAfterSeconds(response)),
        };
      }
      await readAgainAndAnnounce(t(action === 'cancel' ? 'myData.cancelled' : 'myData.deleted'));
      return idle;
    });
  }

  function download(): void {
    act('download', async () => {
      const { data, error, response } = await client.POST('/api/v1/exports/current/download');
      if (data === undefined) {
        if (response.status === 404) {
          await readAgainAndAnnounce(t('myData.gone'));
          return idle;
        }
        return {
          kind: 'refused',
          message: refusalMessage(t, apiErrorFrom(response.status, error), retryAfterSeconds(response)),
        };
      }
      const address = storageDownloadAddress(data.url);
      if (address === null) {
        console.error('export_download_address_invalid');
        return { kind: 'refused', message: t('errors.internal') };
      }
      startDownload(address);
      return idle;
    });
  }

  function content(): ReactElement {
    if (query.data === undefined) {
      return query.isError ? <LoadFailed error={asApiError(query.error)} onRetry={refetch} /> : <DelayedLoading />;
    }
    return (
      <>
        {query.isError ? <RefetchFailed error={asApiError(query.error)} onRetry={refetch} /> : null}
        {notice === null ? null : <FocusedNotice key={notice.id} message={notice.message} />}
        {outcome.kind === 'refused' && outcome.shown === shown ? <FormAlert>{outcome.message}</FormAlert> : null}
        <ExportState
          me={me}
          current={query.data}
          outcome={outcome}
          requestButton={requestButton}
          onRequest={() => {
            setNotice(null);
            setOutcome(idle);
            setDialogOpen(true);
          }}
          onRemove={remove}
          onDownload={download}
        />
      </>
    );
  }

  return (
    <>
      <SettingsSection heading={t('myData.heading')} description={t('myData.explanation')}>
        {content()}
      </SettingsSection>
      <RequestExportDialog
        open={dialogOpen}
        returnFocus={requestButton}
        onClose={() => {
          setDialogOpen(false);
        }}
        onRequested={(requested) => {
          queryClient.setQueryData<CurrentExport>(currentExportKey, { export: requested, next_allowed_at: null });
          setDialogOpen(false);
          announce(t('myData.requested'));
          refetch();
        }}
        onInProgress={() => {
          setDialogOpen(false);
          announce(t('myData.inProgress'));
          refetch();
        }}
        onLimited={refetch}
      />
    </>
  );
}

export function MyDataPage(): ReactElement {
  const me = useSignedInAccount();
  const texts = useFeatureTexts('myData', myDataTextLoaders);

  if (!texts.ready) {
    return (
      <Page>
        <SettingsNav />
        {texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />}
      </Page>
    );
  }

  return (
    <Page>
      <SettingsNav />
      {me.state === 'unverified' ? <ConfirmationPrompt reason="export" /> : <ExportSection key={me.id} me={me} />}
    </Page>
  );
}
