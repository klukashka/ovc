import { useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, type SyntheticEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import {
  FormAlert,
  FormNotice,
  SubmitButton,
  TextField,
  fieldMessage,
  stringField,
} from '../../components/ui/Form.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { useApiClient } from '../../lib/api/client.ts';
import { myTutorsKey, sentRequestsKey } from './api.ts';

function codeRefusal(t: TFunction, state: ActionState): string | null {
  if (state.kind !== 'refused') {
    return null;
  }
  switch (state.error.code) {
    case 'tutor_code_invalid':
      return t('signUp.codeInvalid');
    case 'already_linked':
      return t('tutors.alreadyLinked');
    default:
      return refusalText(t, state.error, state.retryAfter, undefined);
  }
}

export function CodeEntry({
  label,
  waiting,
  replaceWhenSent,
}: {
  readonly label: string;
  readonly waiting: boolean;
  readonly replaceWhenSent: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [sentCount, setSentCount] = useState(0);
  const action = useAction();

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const form = event.currentTarget;
    const code = stringField(new FormData(form), 'code').trim();
    action.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/join-requests', { body: { code } });
      if (data === undefined) {
        return refusedBy(response, error);
      }
      form.reset();
      setSentCount((count) => count + 1);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: sentRequestsKey }),
        queryClient.invalidateQueries({ queryKey: myTutorsKey }),
      ]);
      return done;
    });
  }

  const sent = action.state.kind === 'done';
  if (replaceWhenSent && (sent || waiting)) {
    return sent ? (
      <FormNotice key={sentCount} takeFocus>
        {t('outcome.request_sent')}
      </FormNotice>
    ) : (
      <p>{t('outcome.request_sent')}</p>
    );
  }
  const refusal = codeRefusal(t, action.state);
  const error = action.state.kind === 'refused' ? action.state.error : null;
  return (
    <form
      aria-label={label}
      onSubmit={submit}
      onChange={() => {
        if (action.state.kind !== 'busy') {
          action.reset();
        }
      }}
      className="flex max-w-sm flex-col gap-3"
    >
      {refusal === null ? null : <FormAlert>{refusal}</FormAlert>}
      {sent ? (
        <FormNotice key={sentCount} takeFocus>
          {t('outcome.request_sent')}
        </FormNotice>
      ) : null}
      <TextField
        label={label}
        name="code"
        autoComplete="off"
        maxLength={64}
        hint={t('tutors.codeHint')}
        error={fieldMessage(t, error, 'code')}
      />
      <div>
        <SubmitButton busy={action.busy}>{t('tutors.sendRequest')}</SubmitButton>
      </div>
    </form>
  );
}
