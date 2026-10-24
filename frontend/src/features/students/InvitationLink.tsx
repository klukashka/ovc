import { type ReactElement, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { ActionButton, FormAlert, FormNotice, quietButtonClass } from '../../components/ui/Form.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { type CopyResult, copyText } from '../../lib/clipboard.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import type { CardInvitation } from './api.ts';

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

export function InvitationLink({
  cardId,
  invitation,
  canSend,
  initialNotice = 'none',
  onRegenerated,
  onSent,
  onStale,
}: {
  readonly cardId: string;
  readonly invitation: CardInvitation;
  readonly canSend: boolean;
  readonly initialNotice?: InitialNotice;
  readonly onRegenerated?: (invitation: CardInvitation) => void;
  readonly onSent: () => void;
  readonly onStale?: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const labelId = useId();
  const action = useAction((state) => {
    if (showsStaleData(state)) {
      onStale?.();
    }
  });
  const [notice, setNotice] = useState<Notice>(initialNotice);
  const url = invitation.status === 'open' ? invitation.url : null;

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
    setNotice('none');
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
    setNotice('none');
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

  return (
    <section aria-labelledby={labelId} className="flex flex-col gap-3">
      <h2 id={labelId} className="text-heading">
        {t('invitation.link')}
      </h2>
      {url === null ? null : (
        <p className="rounded-md border border-border-strong bg-field px-3 py-2.5 font-mono text-small break-all select-all">
          {url}
        </p>
      )}
      <InvitationState invitation={invitation} />
      <Refusal state={action.state} dailyLimit="invitations" confirmation="invite" />
      <NoticeMessage notice={notice} />
      <div className="flex flex-wrap gap-3">
        {url === null ? null : (
          <button
            type="button"
            onClick={() => {
              copy(url);
            }}
            className={quietButtonClass}
          >
            {t('invitation.copy')}
          </button>
        )}
        {canSend ? (
          <ActionButton busy={action.busy} onClick={send} className={quietButtonClass}>
            {t('invitation.send')}
          </ActionButton>
        ) : null}
        {onRegenerated === undefined ? null : (
          <ActionButton
            busy={action.busy}
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
