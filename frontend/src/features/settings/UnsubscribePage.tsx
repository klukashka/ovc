import { type ReactElement, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import {
  ActionButton,
  FormAlert,
  FormNotice,
  errorMessage,
  linkClass,
  primaryButtonClass,
  retryAfterSeconds,
} from '../../components/ui/Form.tsx';
import { LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiClient, type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { useAddressToken } from '../auth/AddressToken.tsx';

type EmailGroup = components['schemas']['UnsubscribePreview']['group'];

type Unsubscribe =
  | { readonly kind: 'checking' }
  | { readonly kind: 'invalid' }
  | { readonly kind: 'previewFailed'; readonly error: ApiError; readonly retryAfter: number | null }
  | {
      readonly kind: 'ready';
      readonly group: EmailGroup;
      readonly sending: boolean;
      readonly refusal: { readonly error: ApiError; readonly retryAfter: number | null } | null;
    }
  | { readonly kind: 'done'; readonly group: EmailGroup };

const settingsAddress = '/settings/notifications';

async function preview(client: ApiClient, token: string): Promise<Unsubscribe> {
  const { data, error, response } = await client.GET('/api/v1/unsubscribe/preview', { params: { query: { token } } });
  if (data !== undefined) {
    return { kind: 'ready', group: data.group, sending: false, refusal: null };
  }
  const refusal = apiErrorFrom(response.status, error);
  if (refusal.code === 'token_invalid') {
    return { kind: 'invalid' };
  }
  return { kind: 'previewFailed', error: refusal, retryAfter: retryAfterSeconds(response) };
}

async function turnOff(client: ApiClient, token: string, group: EmailGroup): Promise<Unsubscribe> {
  const { error, response } = await client.POST('/api/v1/unsubscribe', { params: { query: { token } } });
  if (response.ok) {
    return { kind: 'done', group };
  }
  const refusal = apiErrorFrom(response.status, error);
  if (refusal.code === 'token_invalid') {
    return { kind: 'invalid' };
  }
  return { kind: 'ready', group, sending: false, refusal: { error: refusal, retryAfter: retryAfterSeconds(response) } };
}

function SettingsLink(): ReactElement {
  const { t } = useTranslation();
  return (
    <p>
      <Link to={settingsAddress} className={linkClass}>
        {t('unsubscribe.allSettings')}
      </Link>
    </p>
  );
}

function InvalidLinkMessage(): ReactElement {
  const { t } = useTranslation();
  return (
    <>
      <FormAlert>{t('unsubscribe.invalid')}</FormAlert>
      <SettingsLink />
    </>
  );
}

export function UnsubscribePage(): ReactElement {
  const token = useAddressToken();
  return <Page>{token === null ? <InvalidLinkMessage /> : <Confirmation token={token} />}</Page>;
}

function GroupLine({ group }: { readonly group: EmailGroup }): ReactElement {
  const { t } = useTranslation();
  return <p className="text-strong">{t('unsubscribe.group', { group: t(`emailSettings.groups.${group}`) })}</p>;
}

function Confirmation({ token }: { readonly token: string }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const [state, setState] = useState<Unsubscribe>({ kind: 'checking' });
  const [previews, setPreviews] = useState(0);
  const requested = useRef(-1);

  useEffect(() => {
    if (requested.current === previews) {
      return;
    }
    requested.current = previews;
    preview(client, token).then(setState, (thrown: unknown) => {
      setState({ kind: 'previewFailed', error: asApiError(thrown), retryAfter: null });
    });
  }, [client, token, previews]);

  function confirm(group: EmailGroup): void {
    setState({ kind: 'ready', group, sending: true, refusal: null });
    turnOff(client, token, group).then(setState, (thrown: unknown) => {
      setState({ kind: 'ready', group, sending: false, refusal: { error: asApiError(thrown), retryAfter: null } });
    });
  }

  switch (state.kind) {
    case 'checking':
      return (
        <p role="status" className="text-text-muted">
          {t('unsubscribe.checking')}
        </p>
      );
    case 'invalid':
      return <InvalidLinkMessage />;
    case 'previewFailed':
      return state.error.code === 'rate_limited' ? (
        <FormAlert>{errorMessage(t, state.error, state.retryAfter)}</FormAlert>
      ) : (
        <LoadFailed
          error={state.error}
          onRetry={() => {
            setState({ kind: 'checking' });
            setPreviews((count) => count + 1);
          }}
        />
      );
    case 'ready':
      return (
        <>
          <GroupLine group={state.group} />
          {state.refusal === null ? null : (
            <FormAlert>{errorMessage(t, state.refusal.error, state.refusal.retryAfter)}</FormAlert>
          )}
          <p>
            <ActionButton
              busy={state.sending}
              onClick={() => {
                confirm(state.group);
              }}
              className={primaryButtonClass}
            >
              {t('unsubscribe.confirm')}
            </ActionButton>
          </p>
        </>
      );
    case 'done':
      return (
        <>
          <GroupLine group={state.group} />
          <FormNotice takeFocus>{t('unsubscribe.done')}</FormNotice>
          <SettingsLink />
        </>
      );
  }
}
