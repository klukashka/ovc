import { type ReactElement, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { ActionMenu } from '../../components/ui/ActionMenu.tsx';
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
import { Refusal } from '../../components/ui/Refusal.tsx';
import { SectionHead } from '../../components/ui/SectionHead.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { type CopyResult, copyText } from '../../lib/clipboard.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import type { CardInvitation } from './api.ts';

export interface NewLinkTexts {
  readonly menu: string;
  readonly title: string;
  readonly consequence: string;
  readonly cancel: string;
}

type Notice = 'none' | 'copied' | 'copyFailed' | 'sent' | 'regenerated' | 'created';

export type InitialNotice = Extract<Notice, 'none' | 'created'>;

function InvitationState({ invitation }: { readonly invitation: CardInvitation }): ReactElement | null {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  switch (invitation.status) {
    case 'open':
      return (
        <p className="tabular text-small text-text-muted">
          {t('invitation.validUntil', { date: formatDateTime(invitation.expires_at, me.language, me.timezone) })}
        </p>
      );
    case 'expired':
      return <StatusWord tone="warning">{t('invitation.expired')}</StatusWord>;
    case 'revoked_after_mismatches':
      return <StatusWord tone="danger">{t('invitation.revoked')}</StatusWord>;
  }
}

function NoticeMessage({ notice }: { readonly notice: Notice }): ReactElement | null {
  const { t } = useTranslation();
  switch (notice) {
    case 'none':
      return null;
    case 'copyFailed':
      return <FormAlert>{t('invitation.copyFailed')}</FormAlert>;
    case 'copied':
    case 'sent':
    case 'regenerated':
    case 'created':
      return <FormNotice takeFocus>{t(`invitation.${notice}`)}</FormNotice>;
  }
}

export function NewLinkForm({
  cardId,
  texts,
  headingId,
  consequenceId,
  cancelRef,
  onBusy,
  onCreated,
  onStale,
  onErased,
  onCancel,
}: {
  readonly cardId: string;
  readonly texts: NewLinkTexts;
  readonly headingId: string;
  readonly consequenceId: string;
  readonly cancelRef: RefObject<HTMLButtonElement | null>;
  readonly onBusy: (busy: boolean) => void;
  readonly onCreated: (invitation: CardInvitation) => void;
  readonly onStale: () => void;
  readonly onErased: () => void;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const action = useAction(() => {
    onBusy(false);
  });

  function create(): void {
    if (action.busy) {
      return;
    }
    onBusy(true);
    action.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/students/{id}/invitation', {
        params: { path: { id: cardId } },
      });
      if (data === undefined) {
        const refusal = refusedBy(response, error);
        if (showsStaleData(refusal)) {
          onStale();
        } else if (refusal.kind === 'refused' && refusal.error.code === 'email_erased') {
          onErased();
        }
        return refusal;
      }
      onCreated(data.invitation);
      return done;
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="text-heading">
        {texts.title}
      </h2>
      <p id={consequenceId}>{texts.consequence}</p>
      <Refusal state={action.state} dailyLimit="invitations" confirmation="invite" />
      <FormActions>
        <ActionButton busy={action.busy} onClick={create} className={dangerFilledButtonClass}>
          {t('invitation.regenerate')}
        </ActionButton>
        <ActionButton
          ref={cancelRef}
          busy={false}
          unavailable={action.busy}
          onClick={onCancel}
          className={quietButtonClass}
        >
          {texts.cancel}
        </ActionButton>
      </FormActions>
    </div>
  );
}

export function InvitationLink({
  cardId,
  invitation,
  canSend,
  initialNotice = 'none',
  onRegenerated,
  onSent,
  onStale,
  onErased,
  newLinkTexts,
  onDialog,
  onChangedMeanwhile,
}: {
  readonly cardId: string;
  readonly invitation: CardInvitation;
  readonly canSend: boolean;
  readonly initialNotice?: InitialNotice;
  readonly onRegenerated?: (invitation: CardInvitation) => void;
  readonly onSent: () => void;
  readonly onStale?: () => void;
  readonly onErased?: () => void;
  readonly newLinkTexts?: NewLinkTexts;
  readonly onDialog?: (open: boolean) => void;
  readonly onChangedMeanwhile?: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const labelId = useId();
  const action = useAction((state) => {
    if (showsStaleData(state)) {
      onStale?.();
    }
    if (state.kind === 'refused' && state.error.code === 'email_erased') {
      onErased?.();
    }
  });
  const [notice, setNotice] = useState<Notice>(initialNotice);
  const [started, setStarted] = useState<'send' | 'regenerate' | null>(null);
  const [confirmingUrl, setConfirmingUrl] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const menuButton = useRef<HTMLButtonElement>(null);
  const keepLink = useRef<HTMLButtonElement>(null);
  const dialogHeadingId = useId();
  const consequenceId = useId();
  const url = invitation.status === 'open' ? invitation.url : null;
  const changedMeanwhile = confirmingUrl !== null && confirmingUrl !== url && !creating;
  const confirmOpen = confirmingUrl !== null && !changedMeanwhile;
  const reportedChange = useRef<string | null>(null);

  useEffect(() => {
    if (changedMeanwhile && reportedChange.current !== confirmingUrl) {
      reportedChange.current = confirmingUrl;
      onChangedMeanwhile?.();
    }
  }, [changedMeanwhile, confirmingUrl, onChangedMeanwhile]);

  function copy(text: string): void {
    action.reset();
    setNotice('none');
    copyText(navigator.clipboard, text).then(
      (result: CopyResult) => {
        setNotice(result === 'copied' ? 'copied' : 'copyFailed');
      },
      () => {
        setNotice('copyFailed');
      },
    );
  }

  function send(): void {
    if (action.busy) {
      return;
    }
    setNotice('none');
    setStarted('send');
    action.run(async (): Promise<ActionState> => {
      const { error, response } = await client.POST('/api/v1/students/{id}/invitation/send', {
        params: { path: { id: cardId } },
      });
      if (!response.ok) {
        return refusedBy(response, error);
      }
      setNotice('sent');
      onSent();
      return done;
    });
  }

  function regenerate(handle: (invitation: CardInvitation) => void): void {
    if (action.busy) {
      return;
    }
    setNotice('none');
    setStarted('regenerate');
    action.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/students/{id}/invitation', {
        params: { path: { id: cardId } },
      });
      if (data === undefined) {
        return refusedBy(response, error);
      }
      handle(data.invitation);
      setNotice('regenerated');
      return done;
    });
  }

  function closeConfirmation(): void {
    setConfirmingUrl(null);
    onDialog?.(false);
  }

  const confirmsNewLink = onRegenerated !== undefined && newLinkTexts !== undefined && url !== null;

  return (
    <section aria-labelledby={labelId} className="flex flex-col gap-3">
      <SectionHead
        headingId={labelId}
        title={t('invitation.link')}
        action={
          confirmsNewLink ? (
            <ActionMenu
              label={newLinkTexts.menu}
              unavailable={action.busy || creating}
              buttonRef={menuButton}
              items={[
                {
                  label: t('invitation.regenerate'),
                  danger: true,
                  onChoose: (): void => {
                    setNotice('none');
                    action.reset();
                    setConfirmingUrl(url);
                    onDialog?.(true);
                  },
                },
              ]}
            >
              <MoreIcon />
            </ActionMenu>
          ) : null
        }
      />
      {url === null ? null : (
        <p className="rounded-md border border-border-strong bg-field px-3 py-2.5 font-mono text-small break-all select-all">
          {url}
        </p>
      )}
      <InvitationState invitation={invitation} />
      <Refusal state={action.state} dailyLimit="invitations" confirmation="invite" />
      {onRegenerated === undefined || newLinkTexts === undefined ? null : (
        <ModalDialog
          open={confirmOpen}
          busy={creating}
          labelledBy={dialogHeadingId}
          describedBy={consequenceId}
          returnFocus={menuButton}
          initialFocus={keepLink}
          onClose={closeConfirmation}
        >
          {confirmingUrl === null ? null : (
            <NewLinkForm
              cardId={cardId}
              texts={newLinkTexts}
              headingId={dialogHeadingId}
              consequenceId={consequenceId}
              cancelRef={keepLink}
              onBusy={setCreating}
              onCreated={(created) => {
                setConfirmingUrl(null);
                onDialog?.(false);
                onRegenerated(created);
                setNotice('regenerated');
              }}
              onStale={() => {
                setConfirmingUrl(null);
                onChangedMeanwhile?.();
              }}
              onErased={() => {
                setConfirmingUrl(null);
                onErased?.();
              }}
              onCancel={closeConfirmation}
            />
          )}
        </ModalDialog>
      )}
      <NoticeMessage notice={notice} />
      <div className="flex flex-wrap gap-3">
        {url === null ? null : (
          <button
            type="button"
            onClick={() => {
              copy(url);
            }}
            className={`${quietButtonClass} w-full tablet:w-auto`}
          >
            {t('invitation.copy')}
          </button>
        )}
        {canSend ? (
          <ActionButton
            busy={action.busy && started === 'send'}
            unavailable={action.busy}
            onClick={send}
            className={quietButtonClass}
          >
            {t('invitation.send')}
          </ActionButton>
        ) : null}
        {onRegenerated === undefined || confirmsNewLink ? null : (
          <ActionButton
            busy={action.busy && started === 'regenerate'}
            unavailable={action.busy}
            onClick={() => {
              regenerate(onRegenerated);
            }}
            className={quietButtonClass}
          >
            {t('invitation.regenerate')}
          </ActionButton>
        )}
      </div>
    </section>
  );
}
