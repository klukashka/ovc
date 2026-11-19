import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { ConfirmationPrompt, type ConfirmationReason } from '../../components/ui/ConfirmationPrompt.tsx';
import { ActionButton, linkClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { CheckIcon } from '../../components/ui/icons.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { useApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { tutorSettingsKey } from '../settings/tutorSettings.ts';
import { type TutorHome, type TutorHomeChecklist, homeKey, tutorHomeKey } from './api.ts';

type StepName = 'profile' | 'invite' | 'schedule' | 'assignment';

const stepText = {
  profile: 'today.checklistProfile',
  invite: 'today.checklistInvite',
  schedule: 'today.checklistSchedule',
  assignment: 'today.checklistAssignment',
} as const satisfies Readonly<Record<StepName, string>>;

const stepAction = {
  profile: 'today.checklistProfileAction',
  invite: 'today.checklistInviteAction',
  schedule: 'today.checklistScheduleAction',
  assignment: 'today.checklistAssignmentAction',
} as const satisfies Readonly<Record<StepName, string>>;

interface Step {
  readonly name: StepName;
  readonly done: boolean;
  readonly address: string;
  readonly confirmation: ConfirmationReason | null;
}

function stepsOf(checklist: TutorHomeChecklist, confirmed: boolean): readonly Step[] {
  const { steps } = checklist;
  return [
    { name: 'profile', done: steps.profile, address: '/settings/profile', confirmation: null },
    {
      name: 'invite',
      done: steps.student === true,
      address: '/students',
      confirmation: steps.student === null ? 'invite' : null,
    },
    {
      name: 'schedule',
      done: steps.lesson,
      address: '/schedule',
      confirmation: confirmed ? null : 'schedule',
    },
    { name: 'assignment', done: steps.assignment, address: '/homework/library', confirmation: null },
  ];
}

export function checklistShown(checklist: TutorHomeChecklist): boolean {
  const { steps } = checklist;
  return (
    !checklist.onboarding_dismissed && !(steps.profile && steps.student === true && steps.lesson && steps.assignment)
  );
}

function withDismissal(home: TutorHome | undefined): TutorHome | undefined {
  return home === undefined ? undefined : { ...home, checklist: { ...home.checklist, onboarding_dismissed: true } };
}

export function FirstRunChecklist({
  me,
  checklist,
  onDismissed,
}: {
  readonly me: Me;
  readonly checklist: TutorHomeChecklist;
  readonly onDismissed: () => void;
}): ReactElement | null {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const headingId = useId();
  const listRef = useRef<HTMLOListElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const action = useAction((settled) => {
    if (settled.kind === 'done') {
      onDismissed();
    }
  });
  const steps = stepsOf(checklist, me.state === 'active');
  const shown = checklistShown(checklist);
  useFocusAfterRemovedRow(
    listRef,
    shown ? steps.map((step) => `${step.name}-${String(step.done)}`) : [],
    'a',
    headingRef,
  );

  function dismiss(): void {
    action.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.PATCH('/api/v1/tutor/settings', {
        body: { onboarding_dismissed: true },
      });
      if (data === undefined) {
        return refusedBy(response, error);
      }
      await queryClient.cancelQueries({ queryKey: tutorHomeKey });
      queryClient.setQueryData<TutorHome>(tutorHomeKey, withDismissal);
      queryClient.setQueryData(tutorSettingsKey, data);
      void queryClient.invalidateQueries({ queryKey: homeKey });
      return done;
    });
  }

  if (!shown) {
    return null;
  }
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 ref={headingRef} id={headingId} tabIndex={-1} className="text-heading outline-none">
          {t('today.checklistHeading')}
        </h2>
        <ActionButton busy={action.busy} onClick={dismiss} className={quietButtonClass}>
          {t('today.checklistDismiss')}
        </ActionButton>
      </div>
      <Refusal state={action.state} />
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
    </section>
  );
}
