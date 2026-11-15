import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { type Action, type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormAlert,
  SelectChevron,
  fieldClass,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  selectClass,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { type ApiClient, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { addDays, isLocalDate, localDateOf, localInstant, utcOffsetText } from '../../lib/time/localDates.ts';
import { cityOf } from '../../lib/time/timezoneCities.ts';
import { fetchGroup, groupKey, groupsKey } from '../groups/api.ts';
import { libraryKey } from '../library/api.ts';
import type { CalendarItem } from '../schedule/api.ts';
import { type Audience, AudiencePicker } from '../schedule/AudiencePicker.tsx';
import { fetchStudentCard, studentCardKey, studentsKey } from '../students/api.ts';
import { type AnswerPolicy, type IssueItem, type NewIssue, answerPolicies, issuesKey } from './api.ts';
import { type Problem, problemOf } from './issueProblems.ts';
import { ConfirmEmailPrompt } from './IssueAssignment.tsx';
import { type ChosenTemplate, TemplatePicker } from './TemplatePicker.tsx';

export interface PresetLesson {
  readonly id: string;
  readonly title: string;
  readonly startsAt: string;
}

export interface IssuePreset {
  readonly template?: ChosenTemplate;
  readonly audience?: Audience;
  readonly lesson?: PresetLesson;
}

const deadlineDays = 7;
const endOfDay = '23:59';
const minutesPerHour = 60;

interface LessonChoice {
  readonly id: string;
  readonly title: string;
  readonly startsAt: string;
}

function minutesOf(clock: string): number | null {
  const match = /^([0-9]{2}):([0-9]{2})$/.exec(clock);
  if (match === null) {
    return null;
  }
  const hours = Number(match[1]);
  const minutes = Number(match[2]);
  return hours < 24 && minutes < minutesPerHour ? hours * minutesPerHour + minutes : null;
}

function deadlineInstant(date: string, clock: string, timeZone: string): string | null {
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

function lessonLabel(lesson: LessonChoice, me: Me): string {
  return `${formatDateTime(lesson.startsAt, me.language, me.timezone)} · ${lesson.title}`;
}

function choicesOf(items: readonly CalendarItem[]): readonly LessonChoice[] {
  return items.map((item) => ({ id: item.id, title: item.title, startsAt: item.starts_at }));
}

function useAudienceDetail(audience: Audience | null): {
  readonly lessons: readonly LessonChoice[];
  readonly students: number | null;
} {
  const client = useApiClient();
  const card = useQuery({
    queryKey: studentCardKey(audience?.kind === 'one_to_one' ? audience.id : ''),
    queryFn: () => fetchStudentCard(client, audience?.id ?? ''),
    enabled: audience?.kind === 'one_to_one',
    retry: false,
  });
  const group = useQuery({
    queryKey: groupKey(audience?.kind === 'group' ? audience.id : ''),
    queryFn: () => fetchGroup(client, audience?.id ?? ''),
    enabled: audience?.kind === 'group',
    retry: false,
  });
  if (audience === null) {
    return { lessons: [], students: null };
  }
  if (audience.kind === 'one_to_one') {
    return { lessons: choicesOf(card.data?.next_lessons ?? []), students: 1 };
  }
  return {
    lessons: choicesOf(group.data?.next_lessons ?? []),
    students: group.data === undefined ? null : group.data.members.length,
  };
}

async function sendIssue(client: ApiClient, body: NewIssue, idempotencyKey: string): Promise<IssueItem | ActionState> {
  const { data, error, response } = await client.POST('/api/v1/issues', {
    params: { header: { 'Idempotency-Key': idempotencyKey } },
    body,
  });
  return data ?? refusedBy(response, error);
}

function isIssue(value: IssueItem | ActionState): value is IssueItem {
  return 'id' in value;
}

function ConfirmationAlert(): ReactElement {
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

function TemplateField({
  preset,
  template,
  searchField,
  changeButton,
  onChoose,
  onChange,
}: {
  readonly preset: boolean;
  readonly template: ChosenTemplate | null;
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly changeButton: RefObject<HTMLButtonElement | null>;
  readonly onChoose: (template: ChosenTemplate) => void;
  readonly onChange: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const legendId = useId();
  const chosenId = useId();
  return (
    <fieldset aria-labelledby={legendId} className="flex flex-col gap-2">
      <legend id={legendId} className="text-label">
        {t('homework.fields.template')}
      </legend>
      {template === null ? (
        <TemplatePicker searchField={searchField} onChoose={onChoose} />
      ) : (
        <p className="flex min-h-11 items-center gap-3">
          <span id={chosenId} dir="auto" className="min-w-0 break-words text-strong">
            {template.title}
          </span>
          {preset ? null : (
            <button
              ref={changeButton}
              type="button"
              aria-describedby={chosenId}
              onClick={onChange}
              className={quietButtonClass}
            >
              {t('homework.picker.change')}
            </button>
          )}
        </p>
      )}
    </fieldset>
  );
}

function LessonField({
  lessons,
  preset,
  value,
  error,
  field,
  onChange,
}: {
  readonly lessons: readonly LessonChoice[];
  readonly preset: PresetLesson | null;
  readonly value: string;
  readonly error: string | null;
  readonly field: RefObject<HTMLSelectElement | null>;
  readonly onChange: (value: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const selectId = useId();
  const errorId = useId();
  const pinned = preset !== null && !lessons.some((lesson) => lesson.id === preset.id) ? preset : null;
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={selectId} className="text-label">
        {t('homework.fields.lesson')}
      </label>
      <div className="relative">
        <select
          ref={field}
          id={selectId}
          value={value}
          aria-invalid={error === null ? undefined : true}
          aria-describedby={error === null ? undefined : errorId}
          onChange={(event) => {
            onChange(event.target.value);
          }}
          className={selectClass}
        >
          <option value="">{t('homework.fields.noLesson')}</option>
          {pinned === null ? null : (
            <optgroup label={t('homework.fields.chosen')}>
              <option value={pinned.id}>{lessonLabel(pinned, me)}</option>
            </optgroup>
          )}
          {lessons.map((lesson) => (
            <option key={lesson.id} value={lesson.id}>
              {lessonLabel(lesson, me)}
            </option>
          ))}
        </select>
        <SelectChevron />
      </div>
      {error === null ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
    </div>
  );
}

function DeadlineField({
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
        {t('homework.fields.setDeadline')}
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

function PolicyField({
  policy,
  onChange,
}: {
  readonly policy: AnswerPolicy;
  readonly onChange: (policy: AnswerPolicy) => void;
}): ReactElement {
  const { t } = useTranslation();
  const legendId = useId();
  const noteId = useId();
  const name = useId();
  return (
    <fieldset aria-labelledby={legendId} className="flex flex-col gap-1">
      <legend id={legendId} className="text-label">
        {t('homework.fields.policy')}
      </legend>
      {answerPolicies.map((option) => (
        <div key={option} className="flex flex-col">
          <label className="inline-flex min-h-11 items-center gap-2 text-small">
            <input
              type="radio"
              name={name}
              value={option}
              checked={policy === option}
              aria-describedby={option === 'after_submission' ? noteId : undefined}
              onChange={() => {
                onChange(option);
              }}
              className="size-4 accent-accent"
            />
            {t(`homework.policies.${option}`)}
          </label>
          {option === 'after_submission' ? (
            <p id={noteId} className="pl-6 text-small text-text-muted">
              {t('homework.afterSubmissionNote')}
            </p>
          ) : null}
        </div>
      ))}
    </fieldset>
  );
}

type PendingFocus = 'templateSearch' | 'templateChange' | 'deadline' | 'lesson' | null;

function IssueForm({
  headingId,
  preset,
  refs,
  action,
  onCancel,
  onIssued,
}: {
  readonly headingId: string;
  readonly preset: IssuePreset;
  readonly refs: {
    readonly templateSearch: RefObject<HTMLInputElement | null>;
    readonly audienceSearch: RefObject<HTMLInputElement | null>;
    readonly audienceChange: RefObject<HTMLButtonElement | null>;
    readonly deadlineCheckbox: RefObject<HTMLInputElement | null>;
  };
  readonly action: Action;
  readonly onCancel: () => void;
  readonly onIssued: (issue: IssueItem) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const me = useSignedInAccount();
  const [idempotencyKey] = useState(() => crypto.randomUUID());
  const [template, setTemplate] = useState<ChosenTemplate | null>(preset.template ?? null);
  const [audience, setAudience] = useState<Audience | null>(preset.audience ?? null);
  const [lessonId, setLessonId] = useState(preset.lesson?.id ?? '');
  const [deadlineOn, setDeadlineOn] = useState(false);
  const [deadlineDate, setDeadlineDate] = useState<string>(() =>
    addDays(localDateOf(Date.now(), me.timezone), deadlineDays),
  );
  const [deadlineClock, setDeadlineClock] = useState(endOfDay);
  const [policy, setPolicy] = useState<AnswerPolicy>('after_check');
  const [problem, setProblem] = useState<{ readonly problem: Problem; readonly count: number } | null>(null);
  const templateChange = useRef<HTMLButtonElement>(null);
  const deadlineDateField = useRef<HTMLInputElement>(null);
  const lessonField = useRef<HTMLSelectElement>(null);
  const pendingFocus = useRef<PendingFocus>(null);
  const issued = useRef<IssueItem | null>(null);
  const detail = useAudienceDetail(audience);
  const presetLesson = preset.lesson !== undefined && audience?.id === preset.audience?.id ? preset.lesson : null;

  useEffect(() => {
    const target = pendingFocus.current;
    pendingFocus.current = null;
    if (target === 'templateSearch') {
      refs.templateSearch.current?.focus();
    } else if (target === 'templateChange') {
      templateChange.current?.focus();
    } else if (target === 'deadline') {
      deadlineDateField.current?.focus();
    } else if (target === 'lesson') {
      lessonField.current?.focus();
    }
  });

  useEffect(() => {
    if (action.state.kind === 'done' && issued.current !== null) {
      const issue = issued.current;
      issued.current = null;
      onIssued(issue);
    }
  }, [action.state, onIssued]);

  function report(state: ActionState, chosen: ChosenTemplate | null): void {
    const next = problemOf(t, state, chosen);
    if (next === null) {
      return;
    }
    if (state.kind === 'refused') {
      const code = state.error.code;
      if (code === 'not_editable' && preset.template === undefined) {
        setTemplate(null);
      }
      if (code === 'relationship_not_active' || code === 'audience_not_active') {
        setAudience(null);
        setLessonId('');
      }
      if (code === 'not_found') {
        void queryClient.invalidateQueries({ queryKey: libraryKey });
        void queryClient.invalidateQueries({ queryKey: studentsKey });
        void queryClient.invalidateQueries({ queryKey: groupsKey });
      }
    }
    if (next.kind === 'deadline') {
      pendingFocus.current = 'deadline';
    } else if (next.kind === 'lesson') {
      pendingFocus.current = 'lesson';
    }
    setProblem((previous) => ({ problem: next, count: (previous?.count ?? 0) + 1 }));
  }

  function issue(): void {
    if (template === null || audience === null) {
      return;
    }
    const deadlineAt = deadlineOn ? deadlineInstant(deadlineDate, deadlineClock, me.timezone) : null;
    if (deadlineOn && deadlineAt === null) {
      pendingFocus.current = 'deadline';
      setProblem((previous) => ({ problem: { kind: 'deadline' }, count: (previous?.count ?? 0) + 1 }));
      return;
    }
    const chosen = template;
    const body: NewIssue = {
      template_id: chosen.id,
      audience: audience.kind === 'group' ? { group_id: audience.id } : { relationship_id: audience.id },
      lesson_id: lessonId === '' ? null : lessonId,
      deadline_at: deadlineAt,
      answer_policy: policy,
    };
    setProblem(null);
    action.run(async (): Promise<ActionState> => {
      const answer = await sendIssue(client, body, idempotencyKey);
      if (isIssue(answer)) {
        void queryClient.invalidateQueries({ queryKey: issuesKey });
        issued.current = answer;
        return done;
      }
      report(answer, chosen);
      return answer;
    });
  }

  const shown = problem?.problem ?? null;
  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="font-serif text-title">
        {t('homework.issue')}
      </h2>
      <TemplateField
        preset={preset.template !== undefined}
        template={template}
        searchField={refs.templateSearch}
        changeButton={templateChange}
        onChoose={(chosen) => {
          pendingFocus.current = 'templateChange';
          setTemplate(chosen);
        }}
        onChange={() => {
          pendingFocus.current = 'templateSearch';
          setTemplate(null);
        }}
      />
      <AudiencePicker
        chosen={audience}
        preselected={preset.audience ?? null}
        groupsWithMembers
        searchField={refs.audienceSearch}
        changeButton={refs.audienceChange}
        error={undefined}
        onChoose={(chosen) => {
          setAudience(chosen);
          setLessonId(chosen.id === preset.audience?.id ? (preset.lesson?.id ?? '') : '');
        }}
        onClear={() => {
          setAudience(null);
          setLessonId('');
        }}
      />
      {audience === null ? null : (
        <LessonField
          lessons={detail.lessons}
          preset={presetLesson}
          value={lessonId}
          error={shown?.kind === 'lesson' ? t('homework.refusals.lessonLimit') : null}
          field={lessonField}
          onChange={setLessonId}
        />
      )}
      <DeadlineField
        enabled={deadlineOn}
        date={deadlineDate}
        clock={deadlineClock}
        error={shown?.kind === 'deadline' ? t('homework.refusals.futureDeadline') : null}
        checkbox={refs.deadlineCheckbox}
        dateField={deadlineDateField}
        onToggle={setDeadlineOn}
        onDate={setDeadlineDate}
        onClock={setDeadlineClock}
      />
      <PolicyField policy={policy} onChange={setPolicy} />
      {detail.students === null ? null : (
        <p className="text-small text-text-muted">{t('homework.summary', { count: detail.students })}</p>
      )}
      {shown?.kind === 'confirmation' ? <ConfirmationAlert key={problem?.count} /> : null}
      {shown?.kind === 'alert' ? (
        <div className="flex flex-col items-start gap-1">
          <FormAlert key={problem?.count}>{shown.message}</FormAlert>
          {shown.builderLink === null ? null : (
            <Link to={shown.builderLink} className={linkClass}>
              {t('homework.refusals.openInBuilder')}
            </Link>
          )}
        </div>
      ) : null}
      <div className="flex flex-wrap gap-3">
        <ActionButton
          busy={action.busy}
          unavailable={template === null || audience === null}
          onClick={issue}
          className={primaryButtonClass}
        >
          {t('homework.submit')}
        </ActionButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={onCancel} className={textButtonClass}>
          {t('homework.cancel')}
        </ActionButton>
      </div>
    </div>
  );
}

export function IssueDialog({
  open,
  preset,
  returnFocus,
  onClose,
  onIssued,
}: {
  readonly open: boolean;
  readonly preset: IssuePreset;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onIssued: (issue: IssueItem) => void;
}): ReactElement {
  const headingId = useId();
  const templateSearch = useRef<HTMLInputElement>(null);
  const audienceSearch = useRef<HTMLInputElement>(null);
  const audienceChange = useRef<HTMLButtonElement>(null);
  const deadlineCheckbox = useRef<HTMLInputElement>(null);
  const action = useAction();

  function close(): void {
    action.reset();
    onClose();
  }

  function initialFocus(): RefObject<HTMLElement | null> {
    if (preset.template === undefined) {
      return templateSearch;
    }
    return preset.audience === undefined ? audienceSearch : deadlineCheckbox;
  }

  return (
    <ModalDialog
      open={open}
      busy={action.busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={initialFocus()}
      onClose={close}
    >
      <IssueForm
        headingId={headingId}
        preset={preset}
        refs={{ templateSearch, audienceSearch, audienceChange, deadlineCheckbox }}
        action={action}
        onCancel={close}
        onIssued={onIssued}
      />
    </ModalDialog>
  );
}
