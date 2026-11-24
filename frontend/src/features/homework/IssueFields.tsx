import type { TFunction } from 'i18next';
import { type ReactElement, type RefObject, useEffect, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { fieldClass } from '../../components/ui/Form.tsx';
import { type Me, useSignedInAccount } from '../../lib/api/session.ts';
import { isLocalDate, localInstant, utcOffsetText } from '../../lib/time/localDates.ts';
import { cityOf } from '../../lib/time/timezoneCities.ts';
import { type AnswerPolicy, answerPolicies } from './api.ts';
import { ConfirmEmailPrompt } from './IssueAssignment.tsx';

export const deadlineDays = 7;
export const endOfDay = '23:59';
const minutesPerHour = 60;

function minutesOf(clock: string): number | null {
  const match = /^([0-9]{2}):([0-9]{2})$/.exec(clock);
  if (match === null) {
    return null;
  }
  const hours = Number(match[1]);
  const minutes = Number(match[2]);
  return hours < 24 && minutes < minutesPerHour ? hours * minutesPerHour + minutes : null;
}

export function deadlineInstant(date: string, clock: string, timeZone: string): string | null {
  const minutes = minutesOf(clock);
  if (!isLocalDate(date) || minutes === null) {
    return null;
  }
  return new Date(localInstant(date, minutes, timeZone)).toISOString();
}

function zoneHint(t: TFunction, me: Me): string | null {
  if (Intl.DateTimeFormat().resolvedOptions().timeZone === me.timezone) {
    return null;
  }
  const city = cityOf(me.timezone, me.language);
  return city === null
    ? t('lessonEdit.offsetHint', { offset: utcOffsetText(Date.now(), me.timezone) })
    : t('lessonEdit.zoneHint', { city });
}

export function ConfirmationAlert(): ReactElement {
  const element = useRef<HTMLDivElement>(null);

  useEffect(() => {
    element.current?.focus();
  }, []);

  return (
    <div ref={element} role="alert" tabIndex={-1} className="outline-none">
      <ConfirmEmailPrompt />
    </div>
  );
}

export function DeadlineField({
  enabled,
  date,
  clock,
  error,
  checkbox,
  dateField,
  onToggle,
  onDate,
  onClock,
}: {
  readonly enabled: boolean;
  readonly date: string;
  readonly clock: string;
  readonly error: string | null;
  readonly checkbox: RefObject<HTMLInputElement | null>;
  readonly dateField: RefObject<HTMLInputElement | null>;
  readonly onToggle: (enabled: boolean) => void;
  readonly onDate: (value: string) => void;
  readonly onClock: (value: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const legendId = useId();
  const dateId = useId();
  const clockId = useId();
  const hintId = useId();
  const errorId = useId();
  const hint = zoneHint(t, me);
  const described = [hint === null ? null : hintId, error === null ? null : errorId]
    .filter((id) => id !== null)
    .join(' ');
  return (
    <div className="flex flex-col gap-2">
      <label className="inline-flex min-h-11 items-center gap-2 text-small">
        <input
          ref={checkbox}
          type="checkbox"
          checked={enabled}
          onChange={(event) => {
            onToggle(event.target.checked);
          }}
          className="size-4 accent-accent"
        />
        {t('homework.fields.setDeadline')} <span className="text-text-muted">{t('form.optional')}</span>
      </label>
      {enabled ? (
        <fieldset aria-labelledby={legendId} className="flex flex-col gap-2">
          <legend id={legendId} className="text-label">
            {t('homework.fields.deadline')}
          </legend>
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-1">
              <label htmlFor={dateId} className="text-small text-text-muted">
                {t('homework.fields.deadlineDate')}
              </label>
              <input
                ref={dateField}
                id={dateId}
                type="date"
                required
                value={date}
                aria-invalid={error === null ? undefined : true}
                aria-describedby={described === '' ? undefined : described}
                onChange={(event) => {
                  onDate(event.target.value);
                }}
                className={fieldClass}
              />
            </div>
            <div className="flex flex-col gap-1">
              <label htmlFor={clockId} className="text-small text-text-muted">
                {t('homework.fields.deadlineTime')}
              </label>
              <input
                id={clockId}
                type="time"
                required
                value={clock}
                aria-invalid={error === null ? undefined : true}
                aria-describedby={described === '' ? undefined : described}
                onChange={(event) => {
                  onClock(event.target.value);
                }}
                className={fieldClass}
              />
            </div>
          </div>
          {hint === null ? null : (
            <p id={hintId} className="text-small text-text-muted">
              {hint}
            </p>
          )}
          {error === null ? null : (
            <p id={errorId} className="text-small text-danger">
              {error}
            </p>
          )}
        </fieldset>
      ) : null}
    </div>
  );
}

function noteOf(
  option: AnswerPolicy,
  showNoManualHint: boolean,
): 'homework.afterSubmissionNote' | 'homework.noManualQuestionHint' | null {
  if (option === 'after_submission') {
    return 'homework.afterSubmissionNote';
  }
  return option === 'after_check' && showNoManualHint ? 'homework.noManualQuestionHint' : null;
}

function PolicyOption({
  option,
  name,
  policy,
  showNoManualHint,
  onChange,
}: {
  readonly option: AnswerPolicy;
  readonly name: string;
  readonly policy: AnswerPolicy;
  readonly showNoManualHint: boolean;
  readonly onChange: (policy: AnswerPolicy) => void;
}): ReactElement {
  const { t } = useTranslation();
  const noteId = useId();
  const note = noteOf(option, showNoManualHint);
  return (
    <div className="flex flex-col">
      <label className="inline-flex min-h-11 items-center gap-2 text-small">
        <input
          type="radio"
          name={name}
          value={option}
          checked={policy === option}
          aria-describedby={note === null ? undefined : noteId}
          onChange={() => {
            onChange(option);
          }}
          onClick={() => {
            onChange(option);
          }}
          className="size-4 accent-accent"
        />
        {t(`homework.policies.${option}`)}
      </label>
      {note === null ? null : (
        <p id={noteId} className="pl-6 text-small text-text-muted">
          {t(note)}
        </p>
      )}
    </div>
  );
}

export function PolicyRadios({
  policy,
  showNoManualHint,
  onChange,
}: {
  readonly policy: AnswerPolicy;
  readonly showNoManualHint: boolean;
  readonly onChange: (policy: AnswerPolicy) => void;
}): ReactElement {
  const { t } = useTranslation();
  const legendId = useId();
  const name = useId();
  return (
    <fieldset aria-labelledby={legendId} className="flex flex-col gap-1">
      <legend id={legendId} className="text-label">
        {t('homework.fields.policy')}
      </legend>
      {answerPolicies.map((option) => (
        <PolicyOption
          key={option}
          option={option}
          name={name}
          policy={policy}
          showNoManualHint={showNoManualHint}
          onChange={onChange}
        />
      ))}
    </fieldset>
  );
}
