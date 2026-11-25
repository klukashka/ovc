import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type RefObject, useEffect, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import { ActionButton, FormActions, dangerFilledButtonClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { type MyTutor, myTutorsKey } from './api.ts';
import { myTutorsTextLoaders } from './texts.ts';

export type StopOutcome = 'ended' | 'removed' | 'changed';

export function StopStudyingDialog({
  tutor,
  vanished,
  checking,
  returnFocus,
  onClose,
  onSettled,
}: {
  readonly tutor: MyTutor | null;
  readonly vanished: StopOutcome | null;
  readonly checking: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onSettled: (outcome: StopOutcome) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const texts = useFeatureTexts('myTutors', myTutorsTextLoaders);
  const headingId = useId();
  const consequenceId = useId();
  const cancel = useRef<HTMLButtonElement>(null);
  const stopped = useRef<StopOutcome>('ended');
  const action = useAction((state) => {
    if (state.kind === 'done') {
      onSettled(stopped.current);
    }
    if (showsStaleData(state)) {
      onSettled('changed');
      void queryClient.invalidateQueries({ queryKey: myTutorsKey });
    }
  });

  useEffect(() => {
    if (vanished !== null && !action.busy) {
      action.reset();
      onSettled(vanished);
    }
  }, [vanished, action, onSettled]);

  function stop(chosen: MyTutor): void {
    action.run(async (): Promise<ActionState> => {
      const { error, response } = await client.POST('/api/v1/tutors/{relationshipId}/end', {
        params: { path: { relationshipId: chosen.relationship_id } },
      });
      if (!response.ok) {
        return refusedBy(response, error);
      }
      stopped.current = response.status === 204 ? 'removed' : 'ended';
      await queryClient.invalidateQueries({ queryKey: myTutorsKey });
      return done;
    });
  }

  return (
    <ModalDialog
      open={tutor !== null}
      busy={action.busy}
      labelledBy={headingId}
      describedBy={consequenceId}
      returnFocus={returnFocus}
      initialFocus={cancel}
      onClose={() => {
        action.reset();
        onClose();
      }}
    >
      {tutor === null ? null : texts.ready ? (
        <div className="flex flex-col gap-4">
          <h2 id={headingId} className="text-heading">
            {t('tutors.stopTitle')}
          </h2>
          <p id={consequenceId}>
            {t(tutor.state === 'provisional' ? 'myTutors.stopProvisionalConsequence' : 'tutors.stopConsequence')}
          </p>
          <Refusal state={action.state} />
          <FormActions>
            <ActionButton
              busy={action.busy}
              unavailable={checking}
              onClick={() => {
                stop(tutor);
              }}
              className={dangerFilledButtonClass}
            >
              {t('tutors.stopSubmit')}
            </ActionButton>
            <ActionButton
              ref={cancel}
              busy={false}
              unavailable={action.busy}
              onClick={() => {
                action.reset();
                onClose();
              }}
              className={quietButtonClass}
            >
              {t('tutors.cancel')}
            </ActionButton>
          </FormActions>
        </div>
      ) : texts.failed ? (
        <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} />
      ) : (
        <DelayedLoading />
      )}
    </ModalDialog>
  );
}
