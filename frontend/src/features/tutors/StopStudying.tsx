import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type RefObject, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import { ActionButton, dangerButtonClass, textButtonClass } from '../../components/ui/Form.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { useApiClient } from '../../lib/api/client.ts';
import { type MyTutor, myTutorsKey } from './api.ts';

export type StopOutcome = 'ended' | 'removed' | 'changed';

export function StopStudyingDialog({
  tutor,
  checking,
  returnFocus,
  onClose,
  onSettled,
}: {
  readonly tutor: MyTutor | null;
  readonly checking: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onSettled: (outcome: StopOutcome) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
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
      {tutor === null ? null : (
        <div className="flex flex-col gap-4">
          <h2 id={headingId} className="text-heading">
            {t('tutors.stopTitle')}
          </h2>
          <p id={consequenceId}>
            {t(tutor.state === 'provisional' ? 'tutors.stopProvisionalConsequence' : 'tutors.stopConsequence')}
          </p>
          <Refusal state={action.state} />
          <div className="flex flex-wrap gap-3">
            <ActionButton
              busy={action.busy}
              unavailable={checking}
              onClick={() => {
                stop(tutor);
              }}
              className={dangerButtonClass}
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
              className={textButtonClass}
            >
              {t('tutors.cancel')}
            </ActionButton>
          </div>
        </div>
      )}
    </ModalDialog>
  );
}
