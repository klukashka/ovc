import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { ConfirmationPrompt, type ConfirmationReason } from '../../components/ui/ConfirmationPrompt.tsx';
import { linkClass } from '../../components/ui/Form.tsx';
import { CheckIcon } from '../../components/ui/icons.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import type { Calendar } from '../schedule/api.ts';
import { firstStudentCardKey } from '../students/api.ts';

type StepName = 'profile' | 'invite' | 'schedule';

const stepText = {
  profile: 'today.checklistProfile',
  invite: 'today.checklistInvite',
  schedule: 'today.checklistSchedule',
} as const satisfies Readonly<Record<StepName, string>>;

const stepAction = {
  profile: 'today.checklistProfileAction',
  invite: 'today.checklistInviteAction',
  schedule: 'today.checklistScheduleAction',
} as const satisfies Readonly<Record<StepName, string>>;

interface Step {
  readonly name: StepName;
  readonly done: boolean;
  readonly address: string;
  readonly confirmation: ConfirmationReason | null;
}

function useHasStudentCard(me: Me): UseQueryResult<boolean> {
  const client = useApiClient();
  return useQuery({
    queryKey: firstStudentCardKey,
    queryFn: async ({ signal }) => {
      const { data, error, response } = await client.GET('/api/v1/students', {
        params: { query: { limit: 1 } },
        signal,
      });
      if (data === undefined) {
        throw apiErrorFrom(response.status, error);
      }
      return data.items.length > 0;
    },
    enabled: me.state === 'active',
    retry: false,
  });
}

function profileDone(me: Me): boolean {
  return (me.bio ?? '').trim() !== '' || (me.subjects ?? []).length > 0;
}

export function FirstRunChecklist({
  me,
  lessons,
}: {
  readonly me: Me;
  readonly lessons: UseQueryResult<Calendar>;
}): ReactElement | null {
  const { t } = useTranslation();
  const headingId = useId();
  const listRef = useRef<HTMLOListElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const cards = useHasStudentCard(me);
  const confirmed = me.state === 'active';
  const steps: readonly Step[] = [
    { name: 'profile', done: profileDone(me), address: '/settings/profile', confirmation: null },
    {
      name: 'invite',
      done: confirmed && cards.data === true,
      address: '/students',
      confirmation: confirmed ? null : 'invite',
    },
    {
      name: 'schedule',
      done: confirmed && (lessons.data?.items.length ?? 0) > 0,
      address: '/schedule',
      confirmation: confirmed ? null : 'schedule',
    },
  ];
  const failed = confirmed ? [cards, lessons].find((query) => query.isError && query.data === undefined) : undefined;
  const cardsRefetchFailed = confirmed && cards.isError && cards.data !== undefined;
  const pending = confirmed && (cards.isPending || lessons.isPending);
  const shown = !pending && (failed !== undefined || !steps.every((step) => step.done));
  useFocusAfterRemovedRow(
    listRef,
    shown && failed === undefined ? steps.map((step) => `${step.name}-${String(step.done)}`) : [],
    'a',
    headingRef,
  );
  if (!shown) {
    return null;
  }
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-5">
      <h2 ref={headingRef} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('today.checklistHeading')}
      </h2>
      {failed === undefined ? null : (
        <LoadFailed
          error={asApiError(failed.error)}
          onRetry={() => {
            void failed.refetch();
          }}
        />
      )}
      {failed === undefined && cardsRefetchFailed ? (
        <RefetchFailed
          error={asApiError(cards.error)}
          onRetry={() => {
            void cards.refetch();
          }}
        />
      ) : null}
      {failed !== undefined ? null : (
        <ol ref={listRef} className="flex flex-col gap-3">
          {steps.map((step) => (
            <li
              key={`${step.name}-${String(step.done)}`}
              data-row={`${step.name}-${String(step.done)}`}
              data-step={step.name}
              data-done={step.done}
              className="flex items-start gap-3"
            >
              <span
                aria-hidden="true"
                className={
                  step.done
                    ? 'mt-0.5 inline-flex size-5 shrink-0 items-center justify-center rounded-full bg-success text-bg'
                    : 'mt-0.5 inline-flex size-5 shrink-0 rounded-full border border-border-strong'
                }
              >
                {step.done ? <CheckIcon /> : null}
              </span>
              <span className="flex min-w-0 flex-col gap-1">
                <span className={step.done ? 'text-text-muted line-through' : 'text-strong'}>
                  {t(stepText[step.name])}
                  <span className="sr-only"> {step.done ? t('today.checklistDone') : t('today.checklistNotDone')}</span>
                </span>
                {step.done ? null : step.confirmation === null ? (
                  <Link to={step.address} className={linkClass}>
                    {t(stepAction[step.name])}
                  </Link>
                ) : (
                  <ConfirmationPrompt reason={step.confirmation} />
                )}
              </span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
