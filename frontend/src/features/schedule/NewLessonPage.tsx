import { useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, type RefObject, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useSearchParams } from 'react-router';

import { useScheduler } from '../../app/clock.ts';

import { type Action, type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import {
  ActionButton,
  FormActions,
  FormAlert,
  fieldClass,
  fieldMessage,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { ErrorSummary, type FieldProblem, namedField, useFieldProblems } from '../../components/ui/FormProblems.tsx';
import { useFormPreset, useLeaveFormRoute } from '../../components/ui/formRoute.ts';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { Refusal, refusalText } from '../../components/ui/Refusal.tsx';
import { type ApiError, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { isLocalDate } from '../../lib/time/localDates.ts';
import { type NewLesson, lessonDurationLimits, seriesCountLimit } from './api.ts';
import { type Audience, AudiencePicker, audienceOf } from './AudiencePicker.tsx';
import { formatWeekdayName } from './calendarFormat.ts';
import { calendarKey } from './keys.ts';
import { InputField, descriptionLimit, titleLimit } from './LessonFields.tsx';
import { useMaterialTexts } from './materialTexts.ts';
import { MaterialsField } from './MaterialPicker.tsx';
import { type PickedMaterial, idsOf } from './materials.ts';
import { type LessonPrefill, nextFullHour } from './newLesson.ts';
import { newLessonFormTextLoaders } from './newLessonTexts.ts';
import { OverlapWarning } from './OverlapWarning.tsx';

const defaultDurationMinutes = 60;
const durationPresets = [45, 60, 90] as const;
const weekdays = [1, 2, 3, 4, 5, 6, 7];

type SeriesEnd = 'date' | 'count';

function defaultTitle(t: TFunction, audience: Audience | null): string {
  return audience?.kind === 'group'
    ? t('newLessonForm.defaultGroupTitle', { group: audience.name })
    : t('newLessonForm.defaultTitle');
}

function lessonRefusal(t: TFunction, state: ActionState, materialGone: string | null): string | null {
  if (state.kind !== 'refused' || state.error.code === 'lesson_overlap' || state.error.code === 'email_not_verified') {
    return null;
  }
  const error = state.error;
  if (error.fieldCode('body') === 'too_many') {
    return t('newLesson.tooMany');
  }
  if (error.code === 'not_found' && materialGone !== null) {
    return materialGone;
  }
  if (error.code === 'audience_not_active' || error.code === 'not_found') {
    return t('newLessonForm.audienceChanged');
  }
  return refusalText(t, error, state.retryAfter, undefined);
}

function SeriesFields({
  message,
  readFirstDate,
  chosenWeekdays,
}: {
  readonly message: (field: string) => string | undefined;
  readonly readFirstDate: () => string;
  readonly chosenWeekdays: readonly number[];
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const [end, setEnd] = useState<SeriesEnd>('count');
  const [endDefault, setEndDefault] = useState('');
  const weekdaysId = useId();
  const endsId = useId();
  const weekdaysErrorId = useId();
  const weekdaysError = message('weekdays');
  return (
    <div className="flex flex-col gap-4">
      <fieldset
        aria-labelledby={weekdaysId}
        aria-describedby={weekdaysError === undefined ? undefined : weekdaysErrorId}
        className="flex flex-col gap-2"
      >
        <legend id={weekdaysId} className="text-label">
          {t('newLessonForm.weekdays')}
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
          <p id={weekdaysErrorId} className="text-small text-danger">
            {weekdaysError}
          </p>
        )}
      </fieldset>
      <fieldset aria-labelledby={endsId} className="flex flex-col gap-2">
        <legend id={endsId} className="text-label">
          {t('newLessonForm.ends')}
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
          {t('newLessonForm.afterCount')}
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
          {t('newLessonForm.endsOn')}
        </label>
        {end === 'count' ? (
          <InputField
            key="count"
            label={t('newLessonForm.count')}
            name="count"
            type="number"
            defaultValue="8"
            min={1}
            max={seriesCountLimit}
            step={1}
            error={message('count')}
          />
        ) : (
          <InputField
            key="end_date"
            label={t('newLessonForm.endDate')}
            name="end_date"
            type="date"
            defaultValue={endDefault}
            error={message('end_date')}
          />
        )}
      </fieldset>
    </div>
  );
}

function DurationPresets({
  durationField,
  onPreset,
}: {
  readonly durationField: RefObject<HTMLInputElement | null>;
  readonly onPreset: () => void;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <ul aria-label={t('newLessonForm.durationPresets')} className="flex flex-wrap gap-2">
      {durationPresets.map((preset) => (
        <li key={preset}>
          <button
            type="button"
            onClick={() => {
              if (durationField.current !== null) {
                durationField.current.value = String(preset);
                onPreset();
              }
            }}
            className="min-h-11 rounded-md border border-border-strong px-3 text-small hover:border-accent"
          >
            {t('newLessonForm.minutes', { count: preset })}
          </button>
        </li>
      ))}
    </ul>
  );
}

function DescriptionField({ error }: { readonly error: string | undefined }): ReactElement {
  const { t } = useTranslation();
  const descriptionId = useId();
  const errorId = useId();
  const field = useRef<HTMLTextAreaElement>(null);
  const [opened, setOpened] = useState(false);
  const focusOnOpen = useRef(false);
  const shown = opened || error !== undefined;

  useEffect(() => {
    if (focusOnOpen.current) {
      field.current?.focus();
    }
    focusOnOpen.current = false;
  });

  if (!shown) {
    return (
      <p>
        <button
          type="button"
          onClick={() => {
            focusOnOpen.current = true;
            setOpened(true);
          }}
          className={quietButtonClass}
        >
          {t('newLessonForm.addDescription')}
        </button>
      </p>
    );
  }
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={descriptionId} className="text-label">
        {t('newLesson.description')} <span className="font-normal text-text-muted">{t('form.optional')}</span>
      </label>
      <textarea
        ref={field}
        id={descriptionId}
        name="description"
        dir="auto"
        rows={3}
        maxLength={descriptionLimit}
        aria-invalid={error === undefined ? undefined : true}
        aria-describedby={error === undefined ? undefined : errorId}
        className="min-h-22 w-full rounded-md border border-border-strong bg-field px-3 py-2 text-body-phone text-text tablet:text-body"
      />
      {error === undefined ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
    </div>
  );
}

function lessonBody(
  data: FormData,
  audience: Audience,
  title: string,
  repeat: boolean,
  materials: readonly PickedMaterial[],
): NewLesson {
  const description = data.has('description') ? stringField(data, 'description') : '';
  const target = audience.kind === 'group' ? { group_id: audience.id } : { relationship_id: audience.id };
  const lesson: NewLesson = {
    ...target,
    title,
    date: stringField(data, 'date'),
    start: stringField(data, 'start'),
    duration_minutes: Number(stringField(data, 'duration_minutes')),
    ...(description === '' ? {} : { description }),
    ...(materials.length === 0 ? {} : { material_ids: idsOf(materials) }),
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

const serverFields = [
  'title',
  'date',
  'start',
  'duration_minutes',
  'weekdays',
  'count',
  'end_date',
  'description',
] as const;

const problemOrder: readonly string[] = ['audience', ...serverFields];

const inputProblemField: Readonly<Record<string, string>> = { weekday: 'weekdays', series_end: 'end_date' };

function serverProblems(t: TFunction, error: ApiError): readonly FieldProblem[] {
  return serverFields.flatMap((field) => {
    const message = fieldMessage(t, error, field);
    return message === undefined ? [] : [{ field, message, source: 'server' as const }];
  });
}

function NewLessonForm({
  prefill,
  action,
  onCancel,
}: {
  readonly prefill: LessonPrefill;
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
  const form = useRef<HTMLFormElement>(null);
  const searchField = useRef<HTMLInputElement>(null);
  const changeButton = useRef<HTMLButtonElement>(null);
  const durationField = useRef<HTMLInputElement>(null);
  const [idempotencyKey] = useState(() => crypto.randomUUID());
  const preselected = prefill.audience ?? null;
  const [audience, setAudience] = useState<Audience | null>(preselected);
  const [title, setTitle] = useState(() => defaultTitle(t, preselected));
  const [titleEdited, setTitleEdited] = useState(false);
  const [repeat, setRepeat] = useState(prefill.weekdays !== undefined);
  const [materials, setMaterials] = useState<readonly PickedMaterial[]>([]);
  const [refusals, setRefusals] = useState(0);
  const [lastOverlap, setLastOverlap] = useState<ApiError | null>(null);
  const [startedBy, setStartedBy] = useState<'submit' | 'anyway'>('submit');
  const problems = useFieldProblems();
  const error = action.state.kind === 'refused' ? action.state.error : null;
  const overlap = error?.code === 'lesson_overlap' ? error : action.busy && startedBy === 'anyway' ? lastOverlap : null;
  const materialTexts = useMaterialTexts();
  const refusal = lessonRefusal(t, action.state, materials.length > 0 ? materialTexts.materialGone : null);
  const titleError = problems.message('title');

  function weekdaysProblem(data: FormData): string | undefined {
    return repeat && data.getAll('weekday').length === 0 ? t('newLessonForm.weekdaysRequired') : undefined;
  }

  function summaryTarget(field: string): HTMLElement | null {
    if (field === 'audience') {
      return searchField.current ?? changeButton.current;
    }
    return namedField(form.current, field === 'weekdays' ? 'weekday' : field);
  }

  function send(allowOverlap: boolean): void {
    const element = form.current;
    if (element === null) {
      return;
    }
    const data = new FormData(element);
    const missing: FieldProblem[] = [];
    if (audience === null) {
      missing.push({ field: 'audience', message: t('newLessonForm.audienceRequired'), source: 'client' });
    }
    const weekdays = weekdaysProblem(data);
    if (weekdays !== undefined) {
      missing.push({ field: 'weekdays', message: weekdays, source: 'client' });
    }
    if (audience === null || missing.length > 0) {
      action.reset();
      problems.refuse(missing);
      return;
    }
    problems.cleared();
    setStartedBy(allowOverlap ? 'anyway' : 'submit');
    const body: NewLesson = {
      ...lessonBody(data, audience, title, repeat, materials),
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
        if (refused.kind === 'refused') {
          problems.refuse(serverProblems(t, refused.error));
        }
        return refused;
      }
      return done;
    });
  }

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    send(false);
  }

  function inputChanged(event: SyntheticEvent<HTMLFormElement>): void {
    const target = event.target;
    if (!(target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement)) {
      return;
    }
    const field = inputProblemField[target.name] ?? target.name;
    const data = form.current === null ? null : new FormData(form.current);
    problems.edited(field, field === 'weekdays' && data !== null ? weekdaysProblem(data) : undefined);
  }

  return (
    <div className="flex flex-col gap-4">
      <ErrorSummary
        attempt={problems.attempt}
        problems={problems.listed(problemOrder).map((problem) => ({
          key: problem.field,
          message: problem.message,
          target: () => summaryTarget(problem.field),
        }))}
      />
      <form
        ref={form}
        id={formId}
        onSubmit={submit}
        onInput={inputChanged}
        onChange={inputChanged}
        noValidate
        className="flex flex-col gap-4"
      >
        <AudiencePicker
          chosen={audience}
          preselected={preselected}
          searchField={searchField}
          changeButton={changeButton}
          error={problems.message('audience')}
          onChoose={(chosen) => {
            setAudience(chosen);
            problems.edited('audience', undefined);
            if (!titleEdited) {
              setTitle(defaultTitle(t, chosen));
            }
          }}
          onClear={() => {
            setAudience(null);
            if (!titleEdited) {
              setTitle(defaultTitle(t, null));
            }
          }}
        />
        <div className="flex flex-col gap-1">
          <label htmlFor={titleId} className="text-label">
            {t('newLesson.lessonTitle')}
          </label>
          <p id={titleHintId} className="text-small text-text-muted">
            {t('newLesson.titleHint')}
          </p>
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
          {titleError === undefined ? null : (
            <p id={titleErrorId} className="text-small text-danger">
              {titleError}
            </p>
          )}
        </div>
        <InputField
          label={t('newLesson.date')}
          name="date"
          type="date"
          defaultValue={prefill.date}
          error={problems.message('date')}
        />
        <div className="grid grid-cols-1 gap-4 tablet:grid-cols-2">
          <InputField
            label={t('newLesson.start')}
            alignRows
            name="start"
            type="time"
            defaultValue={prefill.start}
            step={300}
            error={problems.message('start')}
          />
          <InputField
            label={t('newLesson.duration')}
            alignRows
            inputRef={durationField}
            name="duration_minutes"
            type="number"
            defaultValue={String(prefill.durationMinutes ?? defaultDurationMinutes)}
            min={lessonDurationLimits.minimum}
            max={lessonDurationLimits.maximum}
            step={lessonDurationLimits.step}
            error={problems.message('duration_minutes')}
          />
        </div>
        <DurationPresets
          durationField={durationField}
          onPreset={() => {
            problems.edited('duration_minutes', undefined);
          }}
        />
        <label className="inline-flex min-h-11 items-center gap-2 text-strong">
          <input
            type="checkbox"
            checked={repeat}
            onChange={(event) => {
              setRepeat(event.target.checked);
              if (!event.target.checked) {
                problems.edited('weekdays', undefined);
              }
            }}
            className="size-4 accent-accent"
          />
          {t('newLessonForm.repeat')}
        </label>
        {repeat ? (
          <SeriesFields
            message={problems.message}
            readFirstDate={() => {
              if (form.current === null) {
                throw new Error('the series fields are shown outside the lesson form');
              }
              return stringField(new FormData(form.current), 'date');
            }}
            chosenWeekdays={prefill.weekdays ?? []}
          />
        ) : null}
        <DescriptionField error={problems.message('description')} />
        <MaterialsField chosen={materials} onChange={setMaterials} />
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
      <FormActions>
        <ActionButton
          type="submit"
          form={formId}
          busy={action.busy && startedBy === 'submit'}
          unavailable={action.busy}
          className={primaryButtonClass}
        >
          {t('newLessonForm.submit')}
        </ActionButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={onCancel} className={quietButtonClass}>
          {t('newLesson.cancel')}
        </ActionButton>
      </FormActions>
    </div>
  );
}

const startPattern = /^([01][0-9]|2[0-3]):[0-5][0-9]$/;

function weekdaysOf(value: unknown): readonly number[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const days = value.filter((day): day is number => typeof day === 'number' && weekdays.includes(day));
  return days.length === value.length ? days : undefined;
}

export function lessonPrefillOf(preset: unknown, queryDate: string | null, fallback: LessonPrefill): LessonPrefill {
  const date = isLocalDate(queryDate) ? queryDate : fallback.date;
  if (typeof preset !== 'object' || preset === null) {
    return { ...fallback, date };
  }
  const presetDate =
    'date' in preset && typeof preset.date === 'string' && isLocalDate(preset.date) ? preset.date : date;
  const start =
    'start' in preset && typeof preset.start === 'string' && startPattern.test(preset.start)
      ? preset.start
      : fallback.start;
  const audience = 'audience' in preset ? audienceOf(preset.audience) : undefined;
  const duration =
    'durationMinutes' in preset && typeof preset.durationMinutes === 'number' ? preset.durationMinutes : undefined;
  const chosenWeekdays = 'weekdays' in preset ? weekdaysOf(preset.weekdays) : undefined;
  return {
    date: presetDate,
    start,
    ...(audience === undefined ? {} : { audience }),
    ...(duration === undefined ? {} : { durationMinutes: duration }),
    ...(chosenWeekdays === undefined ? {} : { weekdays: chosenWeekdays }),
  };
}

type BackLinkKey =
  | 'newLessonForm.back.schedule'
  | 'newLessonForm.back.today'
  | 'newLessonForm.back.student'
  | 'newLessonForm.back.group'
  | 'newLessonForm.back.lesson';

function backLinkKey(address: string): BackLinkKey {
  if (address.startsWith('/today')) {
    return 'newLessonForm.back.today';
  }
  if (address.startsWith('/students/')) {
    return 'newLessonForm.back.student';
  }
  if (address.startsWith('/groups/')) {
    return 'newLessonForm.back.group';
  }
  if (address.startsWith('/lessons/')) {
    return 'newLessonForm.back.lesson';
  }
  return 'newLessonForm.back.schedule';
}

export function NewLessonPage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const scheduler = useScheduler();
  const [searchParams] = useSearchParams();
  const { backAddress, leave } = useLeaveFormRoute('/schedule');
  const preset = useFormPreset(me.id);
  const [prefill] = useState(() =>
    lessonPrefillOf(preset, searchParams.get('date'), nextFullHour(scheduler.wallClock(), me.timezone)),
  );
  const action = useAction((settled) => {
    if (settled.kind === 'done') {
      leave({ kind: 'lessonScheduled' });
    }
  });
  const texts = useFeatureTexts('newLessonForm', newLessonFormTextLoaders);

  if (!texts.ready) {
    return (
      <Page width="reading">
        {texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />}
      </Page>
    );
  }

  return (
    <Page width="reading">
      <p>
        <Link
          to={backAddress}
          onClick={(event) => {
            event.preventDefault();
            leave();
          }}
          className={linkClass}
        >
          {t(backLinkKey(backAddress))}
        </Link>
      </p>
      {me.state === 'active' ? (
        <NewLessonForm
          prefill={prefill}
          action={action}
          onCancel={() => {
            leave();
          }}
        />
      ) : (
        <ConfirmationPrompt reason="schedule" />
      )}
    </Page>
  );
}
