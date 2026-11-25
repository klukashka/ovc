import { type ReactElement, type ReactNode, useId, useRef } from 'react';

import { ActionButton, textButtonClass } from './Form.tsx';
import { useFocusOnMount } from './LiveNotice.tsx';

export interface NoticeAction {
  readonly label: string;
  readonly busy: boolean;
  readonly onActivate: () => void;
}

export function ActionNotice({
  action,
  problem,
  children,
}: {
  readonly action: NoticeAction;
  readonly problem: string | null;
  readonly children: ReactNode;
}): ReactElement {
  const button = useRef<HTMLButtonElement>(null);
  const textId = useId();
  const problemId = useId();
  useFocusOnMount(button);

  return (
    <div className="flex w-full flex-col gap-2 rounded-md border border-border px-4 py-3">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <p id={textId} role="status" className="min-w-0 flex-1 text-text">
          {children}
        </p>
        <ActionButton
          ref={button}
          busy={action.busy}
          describedBy={problem === null ? textId : `${textId} ${problemId}`}
          onClick={action.onActivate}
          className={textButtonClass}
        >
          {action.label}
        </ActionButton>
      </div>
      {problem === null ? null : (
        <p id={problemId} role="alert" className="text-danger">
          {problem}
        </p>
      )}
    </div>
  );
}
