import { useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useSearchParams } from 'react-router';

import { type Action, type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import {
  ActionButton,
  FormActions,
  FormAlert,
  SelectChevron,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  selectClass,
} from '../../components/ui/Form.tsx';
import { ErrorSummary, useFieldProblems } from '../../components/ui/FormProblems.tsx';
import { useFormPreset, useLeaveFormRoute } from '../../components/ui/formRoute.ts';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiClient, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { type Me, useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { addDays, localDateOf } from '../../lib/time/localDates.ts';
import { fetchGroup, groupKey, groupsKey } from '../groups/api.ts';
import { libraryKey } from '../library/api.ts';
import type { CalendarItem } from '../schedule/api.ts';
import { type Audience, AudiencePicker, audienceOf } from '../schedule/AudiencePicker.tsx';
import { fetchStudentCard, studentCardKey, studentsKey } from '../students/api.ts';
import {
  type AnswerPolicy,
  type IssueItem,
  type NewIssue,
  builderAddress,
  fetchIssueTemplate,
  hasManualQuestion,
  issueTemplateKey,
  issuesKey,
} from './api.ts';
import { type Problem, problemOf } from './issueProblems.ts';
import { ConfirmEmailPrompt, homeworkTextLoaders } from './IssueAssignment.tsx';
import {
  ConfirmationAlert,
  DeadlineField,
  PolicyRadios,
  deadlineDays,
  deadlineInstant,
  endOfDay,
} from './IssueFields.tsx';
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

interface LessonChoice {
  readonly id: string;
  readonly title: string;
  readonly startsAt: string;
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
        {t('homework.fields.lesson')} <span className="font-normal text-text-muted">{t('form.optional')}</span>
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

function useNoManualQuestion(template: ChosenTemplate | null): boolean {
  const client = useApiClient();
  const templateId = template?.id ?? '';
  const read = useQuery({
    queryKey: issueTemplateKey(templateId),
    queryFn: () => fetchIssueTemplate(client, templateId),
    enabled: template !== null,
    staleTime: 0,
    gcTime: 0,
  });
  return template !== null && read.data?.id === templateId && !hasManualQuestion(read.data);
}

export function PolicyField({
  template,
  policy,
  onChange,
}: {
  readonly template: ChosenTemplate | null;
  readonly policy: AnswerPolicy;
  readonly onChange: (policy: AnswerPolicy) => void;
}): ReactElement {
  const noManualQuestion = useNoManualQuestion(template);
  return (
    <PolicyRadios policy={policy} showNoManualHint={policy === 'after_check' && noManualQuestion} onChange={onChange} />
  );
}

type PendingFocus = 'templateSearch' | 'templateChange' | null;

const problemOrder: readonly string[] = ['lesson', 'deadline'];

function IssueForm({
  preset,
  idempotencyKey,
  action,
  onCancel,
  onIssued,
}: {
  readonly preset: IssuePreset;
  readonly idempotencyKey: string;
  readonly action: Action;
  readonly onCancel: () => void;
  readonly onIssued: (issue: IssueItem) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const me = useSignedInAccount();
  const [template, setTemplate] = useState<ChosenTemplate | null>(preset.template ?? null);
  const [audience, setAudience] = useState<Audience | null>(preset.audience ?? null);
  const [lessonId, setLessonId] = useState(preset.lesson?.id ?? '');
  const [deadlineOn, setDeadlineOn] = useState(false);
  const [deadlineDate, setDeadlineDate] = useState<string>(() =>
    addDays(localDateOf(Date.now(), me.timezone), deadlineDays),
  );
  const [deadlineClock, setDeadlineClock] = useState(endOfDay);
  const [policy, setPolicy] = useState<AnswerPolicy>('after_check');
  const [alert, setAlert] = useState<{ readonly problem: Problem; readonly count: number } | null>(null);
  const problems = useFieldProblems();
  const templateSearch = useRef<HTMLInputElement>(null);
  const templateChange = useRef<HTMLButtonElement>(null);
  const audienceSearch = useRef<HTMLInputElement>(null);
  const audienceChange = useRef<HTMLButtonElement>(null);
  const deadlineCheckbox = useRef<HTMLInputElement>(null);
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
      templateSearch.current?.focus();
    } else if (target === 'templateChange') {
      templateChange.current?.focus();
    }
  });

  useEffect(() => {
    if (action.state.kind === 'done' && issued.current !== null) {
      const issue = issued.current;
      issued.current = null;
      onIssued(issue);
    }
  }, [action.state, onIssued]);

  function deadlineProblem(date: string, clock: string): string | undefined {
    return deadlineInstant(date, clock, me.timezone) === null ? t('homework.refusals.futureDeadline') : undefined;
  }

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
      problems.refuse([{ field: 'deadline', message: t('homework.refusals.futureDeadline'), source: 'server' }]);
      return;
    }
    if (next.kind === 'lesson') {
      problems.refuse([{ field: 'lesson', message: t('homework.refusals.lessonLimit'), source: 'server' }]);
      return;
    }
    setAlert((previous) => ({ problem: next, count: (previous?.count ?? 0) + 1 }));
  }

  function issue(): void {
    if (template === null || audience === null) {
      return;
    }
    const deadlineAt = deadlineOn ? deadlineInstant(deadlineDate, deadlineClock, me.timezone) : null;
    setAlert(null);
    if (deadlineOn && deadlineAt === null) {
      problems.refuse([{ field: 'deadline', message: t('homework.refusals.futureDeadline'), source: 'client' }]);
      return;
    }
    problems.cleared();
    const chosen = template;
    const body: NewIssue = {
      template_id: chosen.id,
      audience: audience.kind === 'group' ? { group_id: audience.id } : { relationship_id: audience.id },
      lesson_id: lessonId === '' ? null : lessonId,
      deadline_at: deadlineAt,
      answer_policy: policy,
    };
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

  const shown = alert?.problem ?? null;
  return (
    <div className="flex flex-col gap-4">
      <ErrorSummary
        attempt={problems.attempt}
        problems={problems.listed(problemOrder).map((problem) => ({
          key: problem.field,
          message: problem.message,
          target: () => (problem.field === 'lesson' ? lessonField.current : deadlineDateField.current),
        }))}
      />
      <TemplateField
        preset={preset.template !== undefined}
        template={template}
        searchField={templateSearch}
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
        searchField={audienceSearch}
        changeButton={audienceChange}
        error={undefined}
        onChoose={(chosen) => {
          setAudience(chosen);
          setLessonId(chosen.id === preset.audience?.id ? (preset.lesson?.id ?? '') : '');
          problems.edited('lesson', undefined);
        }}
        onClear={() => {
          setAudience(null);
          setLessonId('');
          problems.edited('lesson', undefined);
        }}
      />
      {audience === null ? null : (
        <LessonField
          lessons={detail.lessons}
          preset={presetLesson}
          value={lessonId}
          error={problems.message('lesson') ?? null}
          field={lessonField}
          onChange={(value) => {
            setLessonId(value);
            problems.edited('lesson', undefined);
          }}
        />
      )}
      <DeadlineField
        enabled={deadlineOn}
        date={deadlineDate}
        clock={deadlineClock}
        error={problems.message('deadline') ?? null}
        checkbox={deadlineCheckbox}
        dateField={deadlineDateField}
        onToggle={(enabled) => {
          setDeadlineOn(enabled);
          problems.edited('deadline', undefined);
        }}
        onDate={(value) => {
          setDeadlineDate(value);
          problems.edited('deadline', deadlineProblem(value, deadlineClock));
        }}
        onClock={(value) => {
          setDeadlineClock(value);
          problems.edited('deadline', deadlineProblem(deadlineDate, value));
        }}
      />
      <PolicyField template={template} policy={policy} onChange={setPolicy} />
      {detail.students === null ? null : (
        <p className="text-small text-text-muted">{t('homework.summary', { count: detail.students })}</p>
      )}
      {shown?.kind === 'confirmation' ? <ConfirmationAlert key={alert?.count} /> : null}
      {shown?.kind === 'alert' ? (
        <div className="flex flex-col items-start gap-1">
          <FormAlert key={alert?.count}>{shown.message}</FormAlert>
          {shown.builderLink === null ? null : (
            <Link to={shown.builderLink} className={linkClass}>
              {t('homework.refusals.openInBuilder')}
            </Link>
          )}
        </div>
      ) : null}
      <FormActions>
        <ActionButton
          busy={action.busy}
          unavailable={template === null || audience === null}
          onClick={issue}
          className={primaryButtonClass}
        >
          {t('homework.submit')}
        </ActionButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={onCancel} className={quietButtonClass}>
          {t('homework.cancel')}
        </ActionButton>
      </FormActions>
    </div>
  );
}

function presetLessonOf(value: unknown): PresetLesson | undefined {
  if (typeof value !== 'object' || value === null || !('id' in value) || !('title' in value)) {
    return undefined;
  }
  if (!('startsAt' in value)) {
    return undefined;
  }
  const { id, title, startsAt } = value;
  return typeof id === 'string' && typeof title === 'string' && typeof startsAt === 'string'
    ? { id, title, startsAt }
    : undefined;
}

export function issuePresetOf(value: unknown): IssuePreset {
  if (typeof value !== 'object' || value === null) {
    return {};
  }
  const audience = 'audience' in value ? audienceOf(value.audience) : undefined;
  const lesson = 'lesson' in value ? presetLessonOf(value.lesson) : undefined;
  return {
    ...(audience === undefined ? {} : { audience }),
    ...(audience === undefined || lesson === undefined ? {} : { lesson }),
  };
}

type BackLinkKey =
  | 'homework.issueForm.back.library'
  | 'homework.issueForm.back.builder'
  | 'homework.issueForm.back.student'
  | 'homework.issueForm.back.lesson';

function backLinkKey(address: string): BackLinkKey {
  if (address.startsWith('/homework/templates/')) {
    return 'homework.issueForm.back.builder';
  }
  if (address.startsWith('/students/')) {
    return 'homework.issueForm.back.student';
  }
  if (address.startsWith('/lessons/')) {
    return 'homework.issueForm.back.lesson';
  }
  return 'homework.issueForm.back.library';
}

const templateIdPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function IssueAssignmentContent(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const me = useSignedInAccount();
  const [searchParams] = useSearchParams();
  const { backAddress, leave } = useLeaveFormRoute('/homework/library');
  const templateId = searchParams.get('template');
  const templateIdValid = templateId === null || templateIdPattern.test(templateId);
  const preset = issuePresetOf(useFormPreset(me.id));
  const [idempotencyKey] = useState(() => crypto.randomUUID());
  const action = useAction();
  const template = useQuery({
    queryKey: issueTemplateKey(templateId ?? ''),
    queryFn: () => fetchIssueTemplate(client, templateId ?? ''),
    enabled: templateId !== null && templateIdValid && me.email_verified,
    retry: false,
  });
  const templateError = template.error === null || template.data !== undefined ? null : asApiError(template.error);
  const notFound = !templateIdValid || templateError?.code === 'not_found';
  const backLink = (
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
  );

  function body(): ReactElement {
    if (!me.email_verified) {
      return <ConfirmEmailPrompt />;
    }
    if (templateId === null) {
      return (
        <IssueForm
          preset={preset}
          idempotencyKey={idempotencyKey}
          action={action}
          onCancel={() => {
            leave();
          }}
          onIssued={(issue) => {
            leave({ kind: 'assignmentIssued', issueId: issue.id });
          }}
        />
      );
    }
    if (notFound) {
      return <NotFoundContent />;
    }
    if (templateError !== null) {
      return (
        <LoadFailed
          error={templateError}
          onRetry={() => {
            void template.refetch();
          }}
        />
      );
    }
    if (template.data === undefined) {
      return <DelayedLoading />;
    }
    if (!template.data.ready) {
      return (
        <div className="flex flex-col items-start gap-1">
          <p>{t('homework.refusals.notEditable')}</p>
          <Link to={builderAddress(template.data.id)} className={linkClass}>
            {t('homework.refusals.openInBuilder')}
          </Link>
        </div>
      );
    }
    return (
      <IssueForm
        preset={{ ...preset, template: { id: template.data.id, title: template.data.title } }}
        idempotencyKey={idempotencyKey}
        action={action}
        onCancel={() => {
          leave();
        }}
        onIssued={(issue) => {
          leave({ kind: 'assignmentIssued', issueId: issue.id });
        }}
      />
    );
  }

  return (
    <Page width="reading" {...(notFound ? { title: 'page.notFound' as const } : {})} focusTitle={notFound}>
      {backLink}
      {body()}
    </Page>
  );
}

export function IssueAssignmentPage(): ReactElement {
  const texts = useFeatureTexts('homework', homeworkTextLoaders);
  if (!texts.ready) {
    return (
      <Page width="reading">
        {texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />}
      </Page>
    );
  }
  return <IssueAssignmentContent />;
}
