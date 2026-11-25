import { useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, useAction } from '../../components/ui/action.ts';
import { ActionNotice } from '../../components/ui/ActionNotice.tsx';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { hasUnknownOutcome } from '../../components/ui/settled.ts';
import { useApiClient } from '../../lib/api/client.ts';
import { libraryKey } from './api.ts';
import { targetName } from './LibraryDialogs.tsx';
import { type Moved, placementOf, sendMove } from './moveUndo.ts';

type Phase = 'offered' | 'unconfirmed' | 'back';

function isReadable(moved: Moved): boolean {
  return moved.target.kind === 'folder' || moved.target.item.kind === 'template';
}

function undoProblem(t: TFunction, state: ActionState): string | null {
  if (state.kind !== 'refused') {
    return null;
  }
  if (state.error.status === 409) {
    return t('library.undoChanged');
  }
  if (state.error.status === 404) {
    return t('library.undoGone');
  }
  if (state.error.fieldCode('parent_id') === 'invalid') {
    return t('library.depth');
  }
  return refusalText(t, state.error, state.retryAfter, undefined);
}

export function MovedNotice({
  moved,
  folderName,
}: {
  readonly moved: Moved;
  readonly folderName: (id: string | null) => string;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const action = useAction();
  const [phase, setPhase] = useState<Phase>('offered');
  const [problem, setProblem] = useState<string | null>(null);

  function back(): ActionState {
    setPhase('back');
    void queryClient.invalidateQueries({ queryKey: libraryKey });
    return done;
  }

  function undo(): void {
    action.run(async (): Promise<ActionState> => {
      if (phase === 'unconfirmed') {
        const placement = await placementOf(client, moved.target);
        if (placement.kind === 'placed' && placement.folder === moved.from) {
          return back();
        }
        if (placement.kind === 'placed' && placement.version !== null && placement.version !== moved.version) {
          setProblem(t('library.undoChanged'));
          return { kind: 'idle' };
        }
        if (placement.kind === 'gone') {
          setProblem(t('library.undoGone'));
          return { kind: 'idle' };
        }
        if (placement.kind === 'unknown' && isReadable(moved)) {
          return { kind: 'idle' };
        }
      }
      const answer = await sendMove(client, moved.target, moved.from, moved.version);
      if (typeof answer === 'number' || answer === null) {
        return back();
      }
      if (hasUnknownOutcome(answer)) {
        const placement = await placementOf(client, moved.target);
        if (placement.kind === 'placed' && placement.folder === moved.from) {
          return back();
        }
        setPhase('unconfirmed');
        setProblem(null);
        return { kind: 'idle' };
      }
      if (answer.kind === 'refused' && answer.error.status === 409) {
        void queryClient.invalidateQueries({ queryKey: libraryKey });
      }
      setProblem(undoProblem(t, answer));
      return { kind: 'idle' };
    });
  }

  if (phase === 'back') {
    return (
      <FocusedNotice
        message={
          <>
            {t('library.movedBack')} <bdi>{targetName(moved.target)}</bdi>
          </>
        }
      />
    );
  }

  return (
    <ActionNotice
      action={{
        label: t(phase === 'unconfirmed' ? 'library.retry' : 'library.undo'),
        busy: action.busy,
        onActivate: undo,
      }}
      problem={problem}
    >
      {phase === 'unconfirmed' ? (
        t('library.unconfirmed')
      ) : (
        <>
          {t('library.movedItem')} <bdi>{targetName(moved.target)}</bdi> {t('library.movedTo')}{' '}
          <bdi>{folderName(moved.to)}</bdi>
        </>
      )}
    </ActionNotice>
  );
}
