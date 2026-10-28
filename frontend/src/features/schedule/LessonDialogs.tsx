import type { TFunction } from 'i18next';
import { type ReactElement, type RefObject, type SyntheticEvent, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type Action, type ActionState, done, refusedBy } from '../../components/ui/action.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormAlert,
  dangerButtonClass,
  fieldClass,
  fieldMessage,
  primaryButtonClass,
  stringField,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { type ApiError, useApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { localDateOf, localMinutesOf, localTimeOf, utcOffsetText } from '../../lib/time/localDates.ts';
import { cityOf } from '../../lib/time/timezoneCities.ts';
import {
  type ChangedLessons,
  type LessonDetail,
  type LessonPatch,
  type LessonScope,
  lessonDurationLimits,
} from './api.ts';
import { InputField, descriptionLimit, titleLimit } from './LessonFields.tsx';
import { OverlapWarning } from './OverlapWarning.tsx';

type LessonChanges = Omit<LessonPatch, 'scope' | 'version' | 'allow_overlap'>;

interface ShownValues {
  readonly title: string;
  readonly date: string;
  readonly start: string;
  readonly duration: number;
  readonly description: string;
}

export interface DialogHandlers {
  readonly onDismiss: () => void;
  readonly onSaved: (changed: ChangedLessons) => void;
  readonly onStale: () => void;
}

const staleCodes: ReadonlySet<string> = new Set(['not_editable', 'conflict', 'not_found']);

function isStale(state: ActionState): boolean {
  return state.kind === 'refused' && state.error.code !== undefined && staleCodes.has(state.error.code);
}

function withUnixLineBreaks(text: string): string {
  return text.replace(/\r\n?/g, '\n');
}

function shownValues(lesson: LessonDetail): ShownValues {
  const start = Date.parse(lesson.starts_at);
  return {
    title: lesson.title,
    date: localDateOf(start, lesson.timezone),
    start: localTimeOf(localMinutesOf(start, lesson.timezone)),
    duration: lesson.duration_minutes,
    description: withUnixLineBreaks(lesson.description ?? ''),
  };
}

function changesOf(data: FormData, shown: ShownValues, scope: LessonScope): LessonChanges | null {
  const title = stringField(data, 'title');
  const date = scope === 'this' ? stringField(data, 'date') : shown.date;
  const start = stringField(data, 'start');
  const duration = Number(stringField(data, 'duration_minutes'));
  const description = withUnixLineBreaks(stringField(data, 'description'));
  const changes: LessonChanges = {
    ...(title === shown.title ? {} : { title }),
    ...(date === shown.date ? {} : { date }),
    ...(start === shown.start ? {} : { start }),
    ...(duration === shown.duration ? {} : { duration_minutes: duration }),
    ...(description === shown.description ? {} : { description: description === '' ? null : description }),
  };
  return Object.keys(changes).length === 0 ? null : changes;
}

function editRefusal(t: TFunction, state: ActionState): string | null {
  if (state.kind !== 'refused' || state.error.code === 'lesson_overlap' || isStale(state)) {
    return null;
  }
  if (state.error.fieldCode('body') === 'too_many') {
    return t('newLesson.tooMany');
  }
  return refusalText(t, state.error, state.retryAfter, undefined);
}

function zoneHint(t: TFunction, lesson: LessonDetail, me: Me): string | undefined {
  if (lesson.timezone === me.timezone) {
    return undefined;
  }
  const city = cityOf(lesson.timezone, me.language);
  return city === null
    ? t('lessonEdit.offsetHint', { offset: utcOffsetText(Date.parse(lesson.starts_at), lesson.timezone) })
    : t('lessonEdit.zoneHint', { city });
}

function ScopeChoice({
  legend,
  scope,
  firstField,
  onChange,
}: {
  readonly legend: string;
  readonly scope: LessonScope;
  readonly firstField: RefObject<HTMLInputElement | null>;
  readonly onChange: (scope: LessonScope) => void;
}): ReactElement {
  const { t } = useTranslation();
  const legendId = useId();
  const name = useId();
  const choices: readonly { readonly value: LessonScope; readonly label: string }[] = [
    { value: 'this', label: t('lessonEdit.scopeThis') },
    { value: 'following', label: t('lessonEdit.scopeFollowing') },
  ];
  return (
    <fieldset aria-labelledby={legendId} className="flex flex-col gap-1">
      <legend id={legendId} className="text-label">
        {legend}
      </legend>
      {choices.map((choice) => (
        <label key={choice.value} className="inline-flex min-h-11 items-center gap-2 text-small">
          <input
            ref={choice.value === 'this' ? firstField : undefined}
            type="radio"
            name={name}
            value={choice.value}
            checked={scope === choice.value}
            onChange={() => {
              onChange(choice.value);
            }}
            className="size-4 accent-accent"
          />
          {choice.label}
        </label>
      ))}
    </fieldset>
  );
}

export function EditForm({
  lesson,
  me,
  headingId,
  firstField,
  action,
  handlers,
}: {
  readonly lesson: LessonDetail;
  readonly me: Me;
  readonly headingId: string;
  readonly firstField: RefObject<HTMLInputElement | null>;
  readonly action: Action;
  readonly handlers: DialogHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const formId = useId();
  const titleId = useId();
  const titleHintId = useId();
  const titleErrorId = useId();
  const descriptionId = useId();
  const descriptionErrorId = useId();
  const form = useRef<HTMLFormElement>(null);
  const [shown] = useState(() => shownValues(lesson));
  const [scope, setScope] = useState<LessonScope>('this');
  const [refusals, setRefusals] = useState(0);
  const [lastOverlap, setLastOverlap] = useState<ApiError | null>(null);
  const [startedBy, setStartedBy] = useState<'submit' | 'anyway'>('submit');
  const error = action.state.kind === 'refused' && !isStale(action.state) ? action.state.error : null;
  const overlap = error?.code === 'lesson_overlap' ? error : action.busy && startedBy === 'anyway' ? lastOverlap : null;
  const refusal = editRefusal(t, action.state);
  const titleError = fieldMessage(t, error, 'title');
  const descriptionError = fieldMessage(t, error, 'description');
  const hint = zoneHint(t, lesson, me);
  const zoneHintId = useId();
  const hintId = hint === undefined ? undefined : zoneHintId;

  function send(allowOverlap: boolean): void {
    const element = form.current;
    if (element === null) {
      return;
    }
    const changes = changesOf(new FormData(element), shown, scope);
    if (changes === null) {
      handlers.onDismiss();
      return;
    }
    setStartedBy(allowOverlap ? 'anyway' : 'submit');
    const body: LessonPatch = {
      scope,
      version: lesson.version,
      ...changes,
      ...(allowOverlap ? { allow_overlap: true } : {}),
    };
    action.run(async (): Promise<ActionState> => {
      const {
        data: changed,
        error: problem,
        response,
      } = await client.PATCH('/api/v1/lessons/{id}', { params: { path: { id: lesson.id } }, body });
      if (changed === undefined) {
        const refused = refusedBy(response, problem);
        if (isStale(refused)) {
          handlers.onStale();
          return refused;
        }
        setRefusals((count) => count + 1);
        setLastOverlap(refused.kind === 'refused' && refused.error.code === 'lesson_overlap' ? refused.error : null);
        return refused;
      }
      handlers.onSaved(changed);
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
        {t('lessonEdit.title')}
      </h2>
      <form ref={form} id={formId} onSubmit={submit} noValidate className="flex flex-col gap-4">
        {lesson.series_id === null ? null : (
          <ScopeChoice
            legend={t('lessonEdit.scope')}
            scope={scope}
            firstField={firstField}
            onChange={(chosen) => {
              if (!action.busy) {
                setScope(chosen);
              }
            }}
          />
        )}
        <div className="flex flex-col gap-1">
          <label htmlFor={titleId} className="text-label">
            {t('newLesson.lessonTitle')}
          </label>
          <input
            ref={lesson.series_id === null ? firstField : undefined}
            id={titleId}
            name="title"
            type="text"
            dir="auto"
            autoComplete="off"
            required
            maxLength={titleLimit}
            defaultValue={shown.title}
            aria-invalid={titleError === undefined ? undefined : true}
            aria-describedby={titleError === undefined ? titleHintId : `${titleHintId} ${titleErrorId}`}
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
          {scope === 'this' ? (
            <InputField
              label={t('newLesson.date')}
              name="date"
              type="date"
              defaultValue={shown.date}
              error={fieldMessage(t, error, 'date')}
              {...(hintId === undefined ? {} : { hintId })}
            />
          ) : null}
          <InputField
            label={t('newLesson.start')}
            name="start"
            type="time"
            defaultValue={shown.start}
            step={300}
            error={fieldMessage(t, error, 'start')}
            {...(hintId === undefined ? {} : { hintId })}
          />
          <InputField
            label={t('newLesson.duration')}
            name="duration_minutes"
            type="number"
            defaultValue={String(shown.duration)}
            min={lessonDurationLimits.minimum}
            max={lessonDurationLimits.maximum}
            step={lessonDurationLimits.step}
            error={fieldMessage(t, error, 'duration_minutes')}
          />
        </div>
        {hint === undefined ? null : (
          <p id={zoneHintId} className="text-small text-text-muted">
            {hint}
          </p>
        )}
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
            defaultValue={shown.description}
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
          {t('lessonEdit.save')}
        </ActionButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={handlers.onDismiss} className={textButtonClass}>
          {t('newLesson.cancel')}
        </ActionButton>
      </div>
    </div>
  );
}

export function EditLessonDialog({
  lesson,
  me,
  action,
  returnFocus,
  handlers,
}: {
  readonly lesson: LessonDetail | null;
  readonly me: Me;
  readonly action: Action;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly handlers: DialogHandlers;
}): ReactElement {
  const headingId = useId();
  const firstField = useRef<HTMLInputElement>(null);
  return (
    <ModalDialog
      open={lesson !== null}
      busy={action.busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={firstField}
      onClose={handlers.onDismiss}
    >
      {lesson === null ? null : (
        <EditForm
          lesson={lesson}
          me={me}
          headingId={headingId}
          firstField={firstField}
          action={action}
          handlers={handlers}
        />
      )}
    </ModalDialog>
  );
}

type CancelPurpose = 'cancel' | 'endSeries';

interface CancelTexts {
  readonly title: string;
  readonly consequence: string;
  readonly confirm: string;
}

function cancelTexts(t: TFunction, purpose: CancelPurpose, scope: LessonScope): CancelTexts {
  if (purpose === 'endSeries') {
    return {
      title: t('lessonEndSeries.title'),
      consequence: t('lessonEndSeries.consequence'),
      confirm: t('lessonEndSeries.confirm'),
    };
  }
  return scope === 'this'
    ? {
        title: t('lessonCancel.title'),
        consequence: t('lessonCancel.thisConsequence'),
        confirm: t('lessonCancel.confirm'),
      }
    : {
        title: t('lessonCancel.title'),
        consequence: t('lessonCancel.followingConsequence'),
        confirm: t('lessonCancel.confirmFollowing'),
      };
}

function CancelForm({
  lesson,
  purpose,
  headingId,
  consequenceId,
  dismissButton,
  action,
  handlers,
}: {
  readonly lesson: LessonDetail;
  readonly purpose: CancelPurpose;
  readonly headingId: string;
  readonly consequenceId: string;
  readonly dismissButton: RefObject<HTMLButtonElement | null>;
  readonly action: Action;
  readonly handlers: DialogHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const firstScope = useRef<HTMLInputElement>(null);
  const [chosenScope, setScope] = useState<LessonScope>('this');
  const [refusals, setRefusals] = useState(0);
  const refusal = action.state.kind === 'refused' && !isStale(action.state) ? action.state : null;
  const scope = purpose === 'endSeries' ? 'following' : chosenScope;
  const texts = cancelTexts(t, purpose, scope);

  function confirm(): void {
    action.run(async (): Promise<ActionState> => {
      const {
        data: changed,
        error: problem,
        response,
      } = await client.POST('/api/v1/lessons/{id}/cancel', {
        params: { path: { id: lesson.id } },
        body: { scope, version: lesson.version },
      });
      if (changed === undefined) {
        const refused = refusedBy(response, problem);
        if (isStale(refused)) {
          handlers.onStale();
          return refused;
        }
        setRefusals((count) => count + 1);
        return refused;
      }
      handlers.onSaved(changed);
      return done;
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="font-serif text-title">
        {texts.title}
      </h2>
      {lesson.series_id === null || purpose === 'endSeries' ? null : (
        <ScopeChoice
          legend={t('lessonCancel.scope')}
          scope={scope}
          firstField={firstScope}
          onChange={(chosen) => {
            if (!action.busy) {
              setScope(chosen);
            }
          }}
        />
      )}
      <p id={consequenceId}>{texts.consequence}</p>
      {refusal === null ? null : (
        <FormAlert key={refusals}>{refusalText(t, refusal.error, refusal.retryAfter, undefined)}</FormAlert>
      )}
      <div className="flex flex-wrap gap-3">
        <ActionButton busy={action.busy} onClick={confirm} className={dangerButtonClass}>
          {texts.confirm}
        </ActionButton>
        <ActionButton
          ref={dismissButton}
          busy={false}
          unavailable={action.busy}
          onClick={handlers.onDismiss}
          className={textButtonClass}
        >
          {t('newLesson.cancel')}
        </ActionButton>
      </div>
    </div>
  );
}

export function CancelLessonDialog({
  lesson,
  purpose = 'cancel',
  action,
  returnFocus,
  handlers,
}: {
  readonly lesson: LessonDetail | null;
  readonly purpose?: CancelPurpose;
  readonly action: Action;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly handlers: DialogHandlers;
}): ReactElement {
  const headingId = useId();
  const consequenceId = useId();
  const dismissButton = useRef<HTMLButtonElement>(null);
  return (
    <ModalDialog
      open={lesson !== null}
      busy={action.busy}
      labelledBy={headingId}
      describedBy={consequenceId}
      returnFocus={returnFocus}
      initialFocus={dismissButton}
      onClose={handlers.onDismiss}
    >
      {lesson === null ? null : (
        <CancelForm
          lesson={lesson}
          purpose={purpose}
          headingId={headingId}
          consequenceId={consequenceId}
          dismissButton={dismissButton}
          action={action}
          handlers={handlers}
        />
      )}
    </ModalDialog>
  );
}
