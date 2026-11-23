import { type ReactElement, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

export type ProblemSource = 'client' | 'server';

export interface FieldProblem {
  readonly field: string;
  readonly message: string;
  readonly source: ProblemSource;
}

export interface SummaryProblem {
  readonly key: string;
  readonly message: string;
  readonly target: () => HTMLElement | null;
}

export function namedField(form: HTMLFormElement | null, name: string): HTMLElement | null {
  const item = form?.elements.namedItem(name) ?? null;
  const element = item instanceof RadioNodeList ? item[0] : item;
  return element instanceof HTMLElement ? element : null;
}

export interface FieldProblems {
  readonly attempt: number;
  readonly message: (field: string) => string | undefined;
  readonly refuse: (problems: readonly FieldProblem[]) => void;
  readonly edited: (field: string, problemNow: string | undefined) => void;
  readonly cleared: () => void;
  readonly listed: (order: readonly string[]) => readonly FieldProblem[];
}

export function useFieldProblems(): FieldProblems {
  const [problems, setProblems] = useState<ReadonlyMap<string, FieldProblem>>(new Map());
  const [attempt, setAttempt] = useState(0);
  return {
    attempt,
    message: (field) => problems.get(field)?.message,
    refuse: (refused): void => {
      setProblems(new Map(refused.map((problem) => [problem.field, problem])));
      setAttempt((count) => count + 1);
    },
    edited: (field, problemNow): void => {
      const shown = problems.get(field);
      if (shown === undefined) {
        return;
      }
      if (shown.source === 'client' && problemNow !== undefined) {
        if (problemNow !== shown.message) {
          setProblems(new Map(problems).set(field, { ...shown, message: problemNow }));
        }
        return;
      }
      const next = new Map(problems);
      next.delete(field);
      setProblems(next);
    },
    cleared: (): void => {
      if (problems.size > 0) {
        setProblems(new Map());
      }
    },
    listed: (order) => order.flatMap((field) => problems.get(field) ?? []),
  };
}

export function ErrorSummary({
  problems,
  attempt,
}: {
  readonly problems: readonly SummaryProblem[];
  readonly attempt: number;
}): ReactElement | null {
  const { t } = useTranslation();
  const headingId = useId();
  const element = useRef<HTMLDivElement>(null);
  const shown = problems.length > 0;

  useEffect(() => {
    if (shown && attempt > 0) {
      element.current?.focus();
    }
  }, [attempt, shown]);

  if (!shown) {
    return null;
  }
  return (
    <div
      ref={element}
      role="alert"
      tabIndex={-1}
      aria-labelledby={headingId}
      className="flex flex-col gap-2 rounded-lg border-2 border-danger bg-surface px-5 py-4 outline-none"
    >
      <h2 id={headingId} className="text-heading">
        {t('form.errorSummary')}
      </h2>
      <ul className="flex list-disc flex-col gap-1 ps-5">
        {problems.map((problem) => (
          <li key={problem.key}>
            <a
              href={`#${problem.key}`}
              onClick={(event) => {
                event.preventDefault();
                problem.target()?.focus();
              }}
              className="text-danger underline"
            >
              {problem.message}
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}
