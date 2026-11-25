import { useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, type RefObject, type SyntheticEvent, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';

import { type Action, type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormActions,
  FormAlert,
  TextField,
  fieldMessage,
  primaryButtonClass,
  quietButtonClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { useApiClient } from '../../lib/api/client.ts';
import { studentsKey } from '../students/api.ts';
import {
  type GroupColour,
  groupCapReached,
  groupLimit,
  groupMemberLimit,
  groupsKey,
  isGroupColour,
  memberCapReached,
  memberChoiceKey,
} from './api.ts';
import { ColourField } from './ColourField.tsx';
import { type Chosen, MemberPicker, toggled, without } from './MemberPicker.tsx';

export function membershipRefusal(t: TFunction, state: ActionState): string | null {
  if (state.kind !== 'refused') {
    return null;
  }
  const error = state.error;
  if (error.code === 'relationship_not_active') {
    return t('groups.studentsNoLongerActive');
  }
  if (error.code === 'not_found') {
    return t('groups.studentsChanged');
  }
  if (memberCapReached(error)) {
    return t('groups.memberLimit', { limit: groupMemberLimit });
  }
  if (groupCapReached(error)) {
    return t('groups.groupLimit', { limit: groupLimit });
  }
  if (error.fieldCode('name') !== undefined) {
    return null;
  }
  return refusalText(t, error, state.retryAfter, undefined);
}

export function chosenAfterRefusal(chosen: Chosen, state: ActionState): Chosen {
  if (state.kind !== 'refused') {
    return chosen;
  }
  if (state.error.code === 'relationship_not_active') {
    return without(chosen, state.error.relationshipIds);
  }
  return state.error.code === 'not_found' ? new Map() : chosen;
}

function NewGroupForm({
  headingId,
  defaultColour,
  action,
  onCancel,
}: {
  readonly headingId: string;
  readonly defaultColour: GroupColour;
  readonly action: Action;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const formId = useId();
  const [idempotencyKey] = useState(() => crypto.randomUUID());
  const [chosen, setChosen] = useState<Chosen>(new Map());
  const [refusals, setRefusals] = useState(0);
  const error = action.state.kind === 'refused' ? action.state.error : null;
  const refusal = membershipRefusal(t, action.state);

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const name = stringField(data, 'name');
    const colour = stringField(data, 'colour');
    const memberIds = [...chosen.keys()];
    action.run(async (): Promise<ActionState> => {
      const {
        data: group,
        error: problem,
        response,
      } = await client.POST('/api/v1/groups', {
        params: { header: { 'Idempotency-Key': idempotencyKey } },
        body: { name, colour: isGroupColour(colour) ? colour : defaultColour, member_ids: memberIds },
      });
      if (group === undefined) {
        const refused = refusedBy(response, problem);
        setChosen((current) => chosenAfterRefusal(current, refused));
        setRefusals((count) => count + 1);
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: groupsKey }),
          queryClient.invalidateQueries({ queryKey: memberChoiceKey }),
        ]);
        return refused;
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: groupsKey }),
        queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' }),
      ]);
      void navigate(`/groups/${encodeURIComponent(group.id)}`);
      return done;
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="font-serif text-title">
        {t('groups.newTitle')}
      </h2>
      {refusal === null ? null : <FormAlert key={refusals}>{refusal}</FormAlert>}
      <form id={formId} onSubmit={submit} className="flex flex-col gap-4">
        <TextField
          label={t('groups.name')}
          name="name"
          autoComplete="off"
          maxLength={60}
          error={fieldMessage(t, error, 'name')}
        />
        <ColourField defaultColour={defaultColour} />
      </form>
      <MemberPicker
        excluded={new Set()}
        chosen={chosen}
        slots={groupMemberLimit}
        onToggle={(id, name) => {
          setChosen((current) => toggled(current, id, name));
        }}
      />
      <FormActions>
        <ActionButton type="submit" form={formId} busy={action.busy} className={primaryButtonClass}>
          {t('groups.create')}
        </ActionButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={onCancel} className={quietButtonClass}>
          {t('groups.cancel')}
        </ActionButton>
      </FormActions>
    </div>
  );
}

export function NewGroupDialog({
  open,
  defaultColour,
  returnFocus,
  onClose,
}: {
  readonly open: boolean;
  readonly defaultColour: GroupColour;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
}): ReactElement {
  const headingId = useId();
  const action = useAction();

  function close(): void {
    action.reset();
    onClose();
  }

  return (
    <ModalDialog open={open} busy={action.busy} labelledBy={headingId} returnFocus={returnFocus} onClose={close}>
      <NewGroupForm headingId={headingId} defaultColour={defaultColour} action={action} onCancel={close} />
    </ModalDialog>
  );
}
