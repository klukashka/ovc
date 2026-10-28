import { useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, type RefObject, type SyntheticEvent, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type Action, type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormAlert,
  fieldClass,
  fieldMessage,
  primaryButtonClass,
  stringField,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { Refusal, refusalText } from '../../components/ui/Refusal.tsx';
import { type ApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { type NewLesson, lessonDurationLimits, seriesCountLimit } from './api.ts';
import { type Audience, AudiencePicker } from './AudiencePicker.tsx';
import { formatWeekdayName } from './calendarFormat.ts';
import { type LocalDate, addDays, localDateOf, localMinutesOf, localTimeOf } from '../../lib/time/localDates.ts';
import { calendarKey } from './keys.ts';
import { InputField, descriptionLimit, titleLimit } from './LessonFields.tsx';
import { OverlapWarning } from './OverlapWarning.tsx';

export interface LessonPrefill {
  readonly date: LocalDate;
  readonly start: string;
  readonly audience?: Audience;
  readonly durationMinutes?: number;
  readonly weekdays?: readonly number[];
}

export function nextFullHour(now: number, timeZone: string): LessonPrefill {
  const today = localDateOf(now, timeZone);
  const hour = Math.floor(localMinutesOf(now, timeZone) / 60) + 1;
  return hour >= 24 ? { date: addDays(today, 1), start: '00:00' } : { date: today, start: localTimeOf(hour * 60) };
}

const defaultDurationMinutes = 60;
const weekdays = [1, 2, 3, 4, 5, 6, 7];

type SeriesEnd = 'date' | 'count';

function defaultTitle(t: TFunction, audience: Audience | null): string {
  return audience?.kind === 'group'
    ? t('newLesson.defaultGroupTitle', { group: audience.name })
    : t('newLesson.defaultTitle');
}

function lessonRefusal(t: TFunction, state: ActionState): string | null {
  if (state.kind !== 'refused' || state.error.code === 'lesson_overlap' || state.error.code === 'email_not_verified') {
    return null;
  }
  const error = state.error;
  if (error.fieldCode('body') === 'too_many') {
    return t('newLesson.tooMany');
  }
  if (error.code === 'audience_not_active' || error.code === 'not_found') {
    return t('newLesson.audienceChanged');
  }
  return refusalText(t, error, state.retryAfter, undefined);
}

function SeriesFields({
  error,
  readFirstDate,
  chosenWeekdays,
}: {
  readonly error: ApiError | null;
  readonly readFirstDate: () => string;
  readonly chosenWeekdays: readonly number[];
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const [end, setEnd] = useState<SeriesEnd>('count');
  const [endDefault, setEndDefault] = useState('');
  const weekdaysId = useId();
  const endsId = useId();
  const weekdaysError = error?.fieldCode('weekdays');
  return (
    <div className="flex flex-col gap-4">
      <fieldset aria-labelledby={weekdaysId} className="flex flex-col gap-2">
        <legend id={weekdaysId} className="text-label">
          {t('newLesson.weekdays')}
        </legend>
        <ul className="flex flex-wrap gap-2">
          {weekdays.map((weekday) => (
            <li key={weekday}>
              <label className="inline-flex min-h-11 items-center gap-2 rounded-md border border-border-strong px-3 text-small has-checked:border-accent has-checked:text-strong">
                <input
                  type="checkbox"
                  name="weekday"
                  value={weekday}
                  defaultChecked={chosenWeekdays.includes(weekday)}
                  className="size-4 accent-accent"
                />
                <span aria-hidden="true">{formatWeekdayName(weekday, me.language, 'short')}</span>
                <span className="sr-only">{formatWeekdayName(weekday, me.language, 'long')}</span>
              </label>
            </li>
          ))}
        </ul>
        {weekdaysError === undefined ? null : (
          <p className="text-small text-danger">{fieldMessage(t, error, 'weekdays')}</p>
        )}
      </fieldset>
      <fieldset aria-labelledby={endsId} className="flex flex-col gap-2">
        <legend id={endsId} className="text-label">
          {t('newLesson.ends')}
        </legend>
        <label className="inline-flex min-h-11 items-center gap-2 text-small">
          <input
            type="radio"
            name="series_end"
            value="count"
            checked={end === 'count'}
            onChange={() => {
              setEnd('count');
            }}
            className="size-4 accent-accent"
          />
          {t('newLesson.afterCount')}
        </label>
        <label className="inline-flex min-h-11 items-center gap-2 text-small">
          <input
            type="radio"
            name="series_end"
            value="date"
            checked={end === 'date'}
            onChange={() => {
              setEndDefault(readFirstDate());
              setEnd('date');
            }}
            className="size-4 accent-accent"
          />
          {t('newLesson.endsOn')}
        </label>
        {end === 'count' ? (
          <InputField
            key="count"
            label={t('newLesson.count')}
            name="count"
            type="number"
            defaultValue="8"
            min={1}
            max={seriesCountLimit}
            step={1}
            error={fieldMessage(t, error, 'count')}
          />
        ) : (
          <InputField
            key="end_date"
            label={t('newLesson.endDate')}
            name="end_date"
            type="date"
            defaultValue={endDefault}
            error={fieldMessage(t, error, 'end_date')}
          />
        )}
      </fieldset>
    </div>
  );
}

function lessonBody(data: FormData, audience: Audience, title: string, repeat: boolean): NewLesson {
  const description = stringField(data, 'description');
  const target = audience.kind === 'group' ? { group_id: audience.id } : { relationship_id: audience.id };
  const lesson: NewLesson = {
    ...target,
    title,
    date: stringField(data, 'date'),
    start: stringField(data, 'start'),
    duration_minutes: Number(stringField(data, 'duration_minutes')),
    ...(description === '' ? {} : { description }),
  };
  if (!repeat) {
    return lesson;
  }
  const chosenDays = data.getAll('weekday').flatMap((value) => (typeof value === 'string' ? [Number(value)] : []));
  const end =
    stringField(data, 'series_end') === 'date'
      ? { end_date: stringField(data, 'end_date') }
      : { count: Number(stringField(data, 'count')) };
  return { ...lesson, weekdays: chosenDays, ...end };
}

function NewLessonForm({
  headingId,
  prefill,
  searchField,
  action,
  onCancel,
}: {
  readonly headingId: string;
  readonly prefill: LessonPrefill;
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly action: Action;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const formId = useId();
  const titleId = useId();
  const titleHintId = useId();
  const titleErrorId = useId();
  const descriptionId = useId();
  const form = useRef<HTMLFormElement>(null);
  const [idempotencyKey] = useState(() => crypto.randomUUID());
  const preselected = prefill.audience ?? null;
  const [audience, setAudience] = useState<Audience | null>(preselected);
  const [title, setTitle] = useState(() => defaultTitle(t, preselected));
  const [titleEdited, setTitleEdited] = useState(false);
  const [repeat, setRepeat] = useState(prefill.weekdays !== undefined);
  const [missing, setMissing] = useState<'audience' | 'weekdays' | null>(null);
  const [refusals, setRefusals] = useState(0);
  const [missingAttempts, setMissingAttempts] = useState(0);
  const [lastOverlap, setLastOverlap] = useState<ApiError | null>(null);
  const [startedBy, setStartedBy] = useState<'submit' | 'anyway'>('submit');
  const error = action.state.kind === 'refused' ? action.state.error : null;
  const overlap = error?.code === 'lesson_overlap' ? error : action.busy && startedBy === 'anyway' ? lastOverlap : null;
  const descriptionErrorId = useId();
  const descriptionError = fieldMessage(t, error, 'description');
  const refusal = lessonRefusal(t, action.state);
  const titleError = fieldMessage(t, error, 'title');

  function send(allowOverlap: boolean): void {
    const element = form.current;
    if (element === null) {
      return;
    }
    const data = new FormData(element);
    if (audience === null) {
      setMissing('audience');
      searchField.current?.focus();
      return;
    }
    if (repeat && data.getAll('weekday').length === 0) {
      setMissing('weekdays');
      setMissingAttempts((count) => count + 1);
      return;
    }
    setMissing(null);
    setStartedBy(allowOverlap ? 'anyway' : 'submit');
    const body: NewLesson = {
      ...lessonBody(data, audience, title, repeat),
      ...(allowOverlap ? { allow_overlap: true } : {}),
    };
    action.run(async (): Promise<ActionState> => {
      const {
        data: created,
        error: problem,
        response,
      } = await client.POST('/api/v1/lessons', {
        params: { header: { 'Idempotency-Key': idempotencyKey } },
        body,
      });
      await queryClient.invalidateQueries({ queryKey: calendarKey });
      if (created === undefined) {
        const refused = refusedBy(response, problem);
        setRefusals((count) => count + 1);
        setLastOverlap(refused.kind === 'refused' && refused.error.code === 'lesson_overlap' ? refused.error : null);
        return refused;
      }
      return done;
    });
  }

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    send(false);
  }

  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="font-serif text-title">
        {t('newLesson.title')}
      </h2>
      <form ref={form} id={formId} onSubmit={submit} noValidate className="flex flex-col gap-4">
        <AudiencePicker
          chosen={audience}
          preselected={preselected}
          searchField={searchField}
          error={missing === 'audience' ? t('newLesson.audienceRequired') : undefined}
          onChoose={(chosen) => {
            setAudience(chosen);
            setMissing(null);
            if (!titleEdited) {
              setTitle(defaultTitle(t, chosen));
            }
          }}
        />
        <div className="flex flex-col gap-1">
          <label htmlFor={titleId} className="text-label">
            {t('newLesson.lessonTitle')}
          </label>
          <input
            id={titleId}
            name="title"
            type="text"
            dir="auto"
            autoComplete="off"
            required
            maxLength={titleLimit}
            value={title}
            aria-invalid={titleError === undefined ? undefined : true}
            aria-describedby={titleError === undefined ? titleHintId : `${titleHintId} ${titleErrorId}`}
            onChange={(event) => {
              setTitle(event.target.value);
              setTitleEdited(true);
            }}
            className={fieldClass}
          />
          <p id={titleHintId} className="text-small text-text-muted">
            {t('newLesson.titleHint')}
          </p>
          {titleError === undefined ? null : (
            <p id={titleErrorId} className="text-small text-danger">
              {titleError}
            </p>
          )}
        </div>
        <div className="grid grid-cols-1 gap-4 tablet:grid-cols-3">
          <InputField
            label={t('newLesson.date')}
            name="date"
            type="date"
            defaultValue={prefill.date}
            error={fieldMessage(t, error, 'date')}
          />
          <InputField
            label={t('newLesson.start')}
            name="start"
            type="time"
            defaultValue={prefill.start}
            step={300}
            error={fieldMessage(t, error, 'start')}
          />
          <InputField
            label={t('newLesson.duration')}
            name="duration_minutes"
            type="number"
            defaultValue={String(prefill.durationMinutes ?? defaultDurationMinutes)}
            min={lessonDurationLimits.minimum}
            max={lessonDurationLimits.maximum}
            step={lessonDurationLimits.step}
            error={fieldMessage(t, error, 'duration_minutes')}
          />
        </div>
        <label className="inline-flex min-h-11 items-center gap-2 text-strong">
          <input
            type="checkbox"
            checked={repeat}
            onChange={(event) => {
              setRepeat(event.target.checked);
            }}
            className="size-4 accent-accent"
          />
          {t('newLesson.repeat')}
        </label>
        {repeat ? (
          <SeriesFields
            error={error}
            readFirstDate={() => {
              if (form.current === null) {
                throw new Error('the series fields are shown outside the lesson form');
              }
              return stringField(new FormData(form.current), 'date');
            }}
            chosenWeekdays={prefill.weekdays ?? []}
          />
        ) : null}
        {missing === 'weekdays' ? <FormAlert key={missingAttempts}>{t('newLesson.weekdaysRequired')}</FormAlert> : null}
        <div className="flex flex-col gap-1">
          <label htmlFor={descriptionId} className="text-label">
            {t('newLesson.description')}
          </label>
          <textarea
            id={descriptionId}
            name="description"
            dir="auto"
            rows={3}
            maxLength={descriptionLimit}
            aria-invalid={descriptionError === undefined ? undefined : true}
            aria-describedby={descriptionError === undefined ? undefined : descriptionErrorId}
            className="min-h-22 w-full rounded-md border border-border-strong bg-field px-3 py-2 text-body-phone text-text tablet:text-body"
          />
          {descriptionError === undefined ? null : (
            <p id={descriptionErrorId} className="text-small text-danger">
              {descriptionError}
            </p>
          )}
        </div>
      </form>
      {error?.code === 'email_not_verified' ? <Refusal state={action.state} confirmation="schedule" /> : null}
      {refusal === null ? null : <FormAlert key={refusals}>{refusal}</FormAlert>}
      {overlap === null ? null : (
        <OverlapWarning
          key={refusals}
          error={overlap}
          language={me.language}
          timeZone={me.timezone}
          busy={action.busy && startedBy === 'anyway'}
          unavailable={action.busy}
          onConfirm={() => {
            send(true);
          }}
        />
      )}
      <div className="flex flex-wrap gap-3">
        <ActionButton
          type="submit"
          form={formId}
          busy={action.busy && startedBy === 'submit'}
          unavailable={action.busy}
          className={primaryButtonClass}
        >
          {t('newLesson.submit')}
        </ActionButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={onCancel} className={textButtonClass}>
          {t('newLesson.cancel')}
        </ActionButton>
      </div>
    </div>
  );
}

export function NewLessonDialog({
  open,
  prefill,
  returnFocus,
  onClose,
  onScheduled,
}: {
  readonly open: boolean;
  readonly prefill: LessonPrefill;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onScheduled: () => void;
}): ReactElement {
  const headingId = useId();
  const searchField = useRef<HTMLInputElement>(null);
  const action = useAction((settled) => {
    if (settled.kind === 'done') {
      onScheduled();
    }
  });

  function close(): void {
    action.reset();
    onClose();
  }

  return (
    <ModalDialog
      open={open}
      busy={action.busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={searchField}
      onClose={close}
    >
      <NewLessonForm
        headingId={headingId}
        prefill={prefill}
        searchField={searchField}
        action={action}
        onCancel={close}
      />
    </ModalDialog>
  );
}
