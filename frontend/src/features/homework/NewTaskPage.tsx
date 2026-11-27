import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, useEffect, useId, useRef, useState, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import {
  ActionButton,
  FormActions,
  FormAlert,
  fieldClass,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { ErrorSummary, type FieldProblem, useFieldProblems } from '../../components/ui/FormProblems.tsx';
import { useLeaveFormRoute } from '../../components/ui/formRoute.ts';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { type ApiClient, type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { useFileDropGuard } from '../../lib/files/dropGuard.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { addDays, localDateOf } from '../../lib/time/localDates.ts';
import type { QuestionBlock, TemplateBlockInput } from '../builder/blocks.ts';
import { libraryKey } from '../library/api.ts';
import { type Audience, AudiencePicker } from '../schedule/AudiencePicker.tsx';
import {
  type AnswerPolicy,
  type IssueItem,
  type IssueTemplate,
  type NewIssue,
  audienceName,
  builderAddress,
  issueAddress,
  issuesKey,
} from './api.ts';
import { ConfirmEmailPrompt, homeworkTextLoaders } from './IssueAssignment.tsx';
import {
  ConfirmationAlert,
  DeadlineField,
  PolicyRadios,
  deadlineDays,
  deadlineInstant,
  endOfDay,
} from './IssueFields.tsx';
import { type Problem, problemOf } from './issueProblems.ts';
import {
  type PolicyView,
  type QuestionCounts,
  type QuestionProblem,
  type QuestionType,
  type ServerCopy,
  blockCount,
  blockLimit,
  countsOfQuestions,
  countsOfTemplate,
  instructionLimit,
  newQuestion,
  policyBasisOf,
  policyViewOf,
  problemKey,
  serverProblemsOf,
  taskBlocks,
  taskProblemsOf,
  taskTitleLimit,
} from './newTask.ts';
import { TaskAttachmentsField } from './TaskAttachments.tsx';
import { TaskAttachments, attachmentsPending, readyMaterialIds } from './attachmentQueue.ts';
import { TaskQuestions } from './TaskQuestions.tsx';

interface SavedTemplate {
  readonly id: string;
  readonly ready: boolean;
}

type Notice =
  | { readonly kind: 'none' }
  | { readonly kind: 'notReady' }
  | { readonly kind: 'gone' }
  | { readonly kind: 'issuedDifferently'; readonly issue: IssueItem }
  | { readonly kind: 'heldBack'; readonly view: PolicyView; readonly count: number }
  | { readonly kind: 'problem'; readonly problem: Problem; readonly count: number };

type Answer<Value> =
  | { readonly kind: 'ok'; readonly value: Value }
  | {
      readonly kind: 'refused';
      readonly error: ApiError;
      readonly retryAfter: number | null;
      readonly certain: boolean;
    };

function uncertain(status: number): boolean {
  return status === 0 || status >= 500;
}

async function postTemplate(
  client: ApiClient,
  key: string,
  title: string,
  blocks: TemplateBlockInput[],
): Promise<Answer<IssueTemplate>> {
  try {
    const { data, error, response } = await client.POST('/api/v1/library/templates', {
      params: { header: { 'Idempotency-Key': key } },
      body: { title, folder_id: null, blocks },
    });
    if (data !== undefined) {
      return { kind: 'ok', value: data };
    }
    const refusal = refusedBy(response, error);
    return refusal.kind === 'refused'
      ? { ...refusal, certain: !uncertain(response.status) }
      : { kind: 'refused', error: apiErrorFrom(response.status, error), retryAfter: null, certain: false };
  } catch (thrown: unknown) {
    return { kind: 'refused', error: asApiError(thrown), retryAfter: null, certain: false };
  }
}

async function readTemplate(client: ApiClient, id: string): Promise<Answer<IssueTemplate>> {
  try {
    const { data, error, response } = await client.GET('/api/v1/library/templates/{id}', {
      params: { path: { id } },
    });
    return data === undefined
      ? { kind: 'refused', error: apiErrorFrom(response.status, error), retryAfter: null, certain: true }
      : { kind: 'ok', value: data };
  } catch (thrown: unknown) {
    return { kind: 'refused', error: asApiError(thrown), retryAfter: null, certain: false };
  }
}

async function postIssue(client: ApiClient, key: string, body: NewIssue): Promise<Answer<IssueItem>> {
  try {
    const { data, error, response } = await client.POST('/api/v1/issues', {
      params: { header: { 'Idempotency-Key': key } },
      body,
    });
    if (data !== undefined) {
      return { kind: 'ok', value: data };
    }
    const refusal = refusedBy(response, error);
    return refusal.kind === 'refused'
      ? { ...refusal, certain: !uncertain(response.status) }
      : { kind: 'refused', error: apiErrorFrom(response.status, error), retryAfter: null, certain: false };
  } catch (thrown: unknown) {
    return { kind: 'refused', error: asApiError(thrown), retryAfter: null, certain: false };
  }
}

function sameAudience(issue: IssueItem, body: NewIssue): boolean {
  const audience = body.audience;
  return 'group_id' in audience
    ? issue.audience.group_id === audience.group_id
    : issue.audience.relationship_id === audience.relationship_id;
}

function sameView(left: PolicyView, right: PolicyView): boolean {
  return left.policy === right.policy && left.warning === right.warning;
}

const contentFields = ['title', 'instruction', 'uploads', 'blocks'] as const;

function usePolicyState(clientCounts: QuestionCounts): {
  readonly server: ServerCopy;
  readonly view: PolicyView;
  readonly chosen: AnswerPolicy | null;
  readonly choose: (policy: AnswerPolicy) => void;
  readonly freeze: () => void;
  readonly know: (template: IssueTemplate) => PolicyView;
  readonly fail: () => void;
  readonly reset: () => void;
  readonly lockedView: (locked: AnswerPolicy) => PolicyView;
} {
  const [server, setServer] = useState<ServerCopy>({ kind: 'none' });
  const [frozen, setFrozen] = useState<QuestionCounts>(clientCounts);
  const [chosen, setChosen] = useState<AnswerPolicy | null>(null);
  const basis = policyBasisOf(frozen, clientCounts, server);
  return {
    server,
    view: policyViewOf(chosen, basis),
    chosen,
    choose: setChosen,
    freeze: (): void => {
      setFrozen(basis);
      setServer({ kind: 'unknown' });
    },
    know: (template): PolicyView => {
      const counts = countsOfTemplate(template);
      setFrozen(counts);
      setServer({ kind: 'known', counts });
      return policyViewOf(chosen, counts);
    },
    fail: (): void => {
      setServer({ kind: 'failed' });
    },
    reset: (): void => {
      setServer({ kind: 'none' });
    },
    lockedView: (locked): PolicyView => policyViewOf(locked, basis),
  };
}

function PolicyDisclosure({
  view,
  locked,
  onChoose,
}: {
  readonly view: PolicyView;
  readonly locked: boolean;
  readonly onChoose: (policy: AnswerPolicy) => void;
}): ReactElement {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const shownOpen = open && !locked;
  return (
    <div className="flex flex-col gap-2">
      <div>
        <button
          type="button"
          aria-expanded={shownOpen}
          aria-controls={panelId}
          onClick={() => {
            setOpen(!open);
          }}
          className={textButtonClass}
        >
          {t('homework.newTask.policyToggle', { policy: t(`homework.policies.${view.policy}`) })}
        </button>
      </div>
      <div id={panelId} hidden={!shownOpen}>
        {shownOpen ? (
          <PolicyRadios
            policy={view.policy}
            noManualHint={view.warning === 'noManualQuestion' ? 'shown' : 'absent'}
            onChange={onChoose}
          />
        ) : null}
      </div>
      {shownOpen || view.warning === null ? null : (
        <p role="note" className="text-small text-warning">
          {view.warning === 'afterSubmission' ? t('homework.afterSubmissionNote') : t('homework.noManualQuestionHint')}
        </p>
      )}
    </div>
  );
}

function NoticeView({
  notice,
  saved,
}: {
  readonly notice: Notice;
  readonly saved: SavedTemplate | null;
}): ReactElement | null {
  const { t } = useTranslation();
  const builderLink =
    saved === null ? null : (
      <Link to={builderAddress(saved.id)} className={linkClass}>
        {t('homework.refusals.openInBuilder')}
      </Link>
    );
  switch (notice.kind) {
    case 'none':
      return null;
    case 'notReady':
      return (
        <div className="flex flex-col items-start gap-1">
          <FormAlert>{t('homework.newTask.notReady')}</FormAlert>
          {builderLink}
        </div>
      );
    case 'gone':
      return (
        <div className="flex flex-col items-start gap-1">
          <FormAlert>{t('homework.refusals.notFound')}</FormAlert>
          <Link to="/homework/library" className={linkClass}>
            {t('homework.newTask.back')}
          </Link>
        </div>
      );
    case 'heldBack':
      return (
        <FormAlert key={notice.count}>
          {t('homework.newTask.heldBack', { policy: t(`homework.policies.${notice.view.policy}`) })}
          {notice.view.warning === null
            ? null
            : ` ${notice.view.warning === 'afterSubmission' ? t('homework.afterSubmissionNote') : t('homework.noManualQuestionHint')}`}
        </FormAlert>
      );
    case 'issuedDifferently':
      return (
        <div className="flex flex-col items-start gap-1">
          <FormAlert>
            {t('homework.newTask.issuedDifferently', {
              policy: t(`homework.policies.${notice.issue.answer_policy}`),
            })}{' '}
            <bdi dir="auto">{audienceName(notice.issue.audience)}</bdi>
          </FormAlert>
          <Link to={issueAddress(notice.issue.id)} className={linkClass}>
            {t('homework.openIssued')}
          </Link>
        </div>
      );
    case 'problem':
      if (notice.problem.kind === 'confirmation') {
        return <ConfirmationAlert key={notice.count} />;
      }
      if (notice.problem.kind !== 'alert') {
        return null;
      }
      return (
        <div className="flex flex-col items-start gap-1">
          <FormAlert key={notice.count}>{notice.problem.message}</FormAlert>
          {builderLink}
        </div>
      );
  }
}

function NewTaskForm({ onIssued }: { readonly onIssued: (issueId: string) => void }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const session = useSession();
  const queryClient = useQueryClient();
  const me = useSignedInAccount();
  const titleId = useId();
  const instructionFieldId = useId();
  const attachmentsId = useId();
  const [attachments] = useState(
    () =>
      new TaskAttachments(client, session, {
        folder: t('homework.newTask.folderName'),
        fallbackTitle: t('homework.newTask.attachmentTitle'),
      }),
  );
  const attached = useSyncExternalStore(attachments.subscribe, attachments.snapshot);
  const [instructionBlockId] = useState(() => crypto.randomUUID());
  const [issueKey] = useState(() => crypto.randomUUID());
  const [templateKey, setTemplateKey] = useState(() => crypto.randomUUID());
  const [title, setTitle] = useState('');
  const [instruction, setInstruction] = useState('');
  const [questions, setQuestions] = useState<readonly QuestionBlock[]>([]);
  const [audience, setAudience] = useState<Audience | null>(null);
  const [deadlineOn, setDeadlineOn] = useState(false);
  const [deadlineDate, setDeadlineDate] = useState<string>(() =>
    addDays(localDateOf(Date.now(), me.timezone), deadlineDays),
  );
  const [deadlineClock, setDeadlineClock] = useState(endOfDay);
  const [saved, setSaved] = useState<SavedTemplate | null>(null);
  const [templateUncertain, setTemplateUncertain] = useState(false);
  const [lockedIssue, setLockedIssue] = useState<NewIssue | null>(null);
  const [notice, setNotice] = useState<Notice>({ kind: 'none' });
  const problems = useFieldProblems();
  const action = useAction();
  const audienceSearch = useRef<HTMLInputElement>(null);
  const audienceChange = useRef<HTMLButtonElement>(null);
  const deadlineCheckbox = useRef<HTMLInputElement>(null);
  const deadlineDateField = useRef<HTMLInputElement>(null);
  const clientCounts = countsOfQuestions(questions);
  const policy = usePolicyState(clientCounts);
  const contentLocked = saved !== null || templateUncertain || action.busy;
  const issueLocked = lockedIssue !== null || templateUncertain || action.busy;
  useFileDropGuard();

  useEffect(() => {
    attachments.resume();
    return (): void => {
      attachments.stop();
    };
  }, [attachments]);

  function holdBack(view: PolicyView): void {
    setNotice((previous) => ({
      kind: 'heldBack',
      view,
      count: previous.kind === 'heldBack' ? previous.count + 1 : 1,
    }));
  }

  useEffect(() => {
    if (!attachmentsPending(attached)) {
      problems.edited('uploads', undefined);
    }
    if (blockCount(attached.length, questions.length) <= blockLimit) {
      problems.edited('blocks', undefined);
    }
  }, [attached, questions, problems]);

  function contentEdited(): void {
    setNotice({ kind: 'none' });
    setTemplateKey(crypto.randomUUID());
  }

  function questionMessages(question: QuestionBlock): readonly QuestionProblem[] {
    return taskProblemsOf([question]);
  }

  function changeQuestion(changed: QuestionBlock): void {
    setQuestions((previous) => previous.map((question) => (question.id === changed.id ? changed : question)));
    const now = questionMessages(changed);
    for (const field of ['prompt', 'options', 'answers', 'value', 'tolerance'] as const) {
      const problem = now.find((candidate) => candidate.field === field);
      problems.edited(
        problemKey(changed.id, field),
        problem === undefined
          ? undefined
          : questionProblemText(problem, questions.findIndex((q) => q.id === changed.id) + 1),
      );
    }
    contentEdited();
  }

  function questionProblemText(problem: QuestionProblem, number: number): string {
    return t(`homework.newTask.problems.${problem.code}`, { number });
  }

  function numberOf(questionId: string): number {
    return questions.findIndex((question) => question.id === questionId) + 1;
  }

  function questionProblemList(list: readonly QuestionProblem[]): readonly FieldProblem[] {
    return list.map((problem) => ({
      field: problemKey(problem.questionId, problem.field),
      message: questionProblemText(problem, numberOf(problem.questionId)),
      source: 'client' as const,
    }));
  }

  function clientProblems(): readonly FieldProblem[] {
    const found: FieldProblem[] = [];
    if (title.trim() === '') {
      found.push({ field: 'title', message: t('homework.newTask.problems.title'), source: 'client' });
    }
    if (instruction.trim() === '') {
      found.push({ field: 'instruction', message: t('homework.newTask.problems.instruction'), source: 'client' });
    }
    if (attachmentsPending(attached)) {
      found.push({ field: 'uploads', message: t('homework.newTask.problems.uploads'), source: 'client' });
    }
    if (blockCount(attached.length, questions.length) > blockLimit) {
      found.push({ field: 'blocks', message: t('homework.newTask.problems.tooManyBlocks'), source: 'client' });
    }
    found.push(...questionProblemList(taskProblemsOf(questions)));
    if (audience === null) {
      found.push({ field: 'audience', message: t('homework.newTask.problems.audience'), source: 'client' });
    }
    if (deadlineOn && deadlineInstant(deadlineDate, deadlineClock, me.timezone) === null) {
      found.push({ field: 'deadline', message: t('homework.refusals.futureDeadline'), source: 'client' });
    }
    return found;
  }

  function issueBody(chosenPolicy: AnswerPolicy, templateId: string): NewIssue | null {
    if (audience === null) {
      return null;
    }
    return {
      template_id: templateId,
      audience: audience.kind === 'group' ? { group_id: audience.id } : { relationship_id: audience.id },
      lesson_id: null,
      deadline_at: deadlineOn ? deadlineInstant(deadlineDate, deadlineClock, me.timezone) : null,
      answer_policy: chosenPolicy,
    };
  }

  function reportIssueRefusal(answer: Extract<Answer<IssueItem>, { kind: 'refused' }>): ActionState {
    const state: ActionState = { kind: 'refused', error: answer.error, retryAfter: answer.retryAfter };
    const problem = problemOf(t, state, saved === null ? null : { id: saved.id, title });
    if (answer.error.code === 'not_found') {
      void queryClient.invalidateQueries({ queryKey: libraryKey });
    }
    if (problem?.kind === 'deadline') {
      problems.refuse([{ field: 'deadline', message: t('homework.refusals.futureDeadline'), source: 'server' }]);
    } else if (problem !== null) {
      setNotice((previous) => ({
        kind: 'problem',
        problem,
        count: previous.kind === 'problem' ? previous.count + 1 : 1,
      }));
    }
    return state;
  }

  async function send(body: NewIssue): Promise<ActionState> {
    const answer = await postIssue(client, issueKey, body);
    if (answer.kind === 'refused') {
      setLockedIssue(answer.certain ? null : body);
      return reportIssueRefusal(answer);
    }
    setLockedIssue(null);
    void queryClient.invalidateQueries({ queryKey: issuesKey });
    if (answer.value.answer_policy !== body.answer_policy || !sameAudience(answer.value, body)) {
      setNotice({ kind: 'issuedDifferently', issue: answer.value });
      return done;
    }
    onIssued(answer.value.id);
    return done;
  }

  async function issueTemplate(template: IssueTemplate, shown: PolicyView): Promise<ActionState> {
    const fresh = policy.know(template);
    setSaved({ id: template.id, ready: template.ready });
    if (!template.ready) {
      const ids = new Set(questions.map((question) => question.id));
      const mapped = questionProblemList(serverProblemsOf(template.problems, ids));
      const instructionMissing = template.problems.some((problem) => problem.code === 'text_missing');
      problems.refuse([
        ...(instructionMissing
          ? [{ field: 'instruction', message: t('homework.newTask.problems.instruction'), source: 'server' as const }]
          : []),
        ...mapped,
      ]);
      setNotice({ kind: 'notReady' });
      return done;
    }
    if (lockedIssue !== null) {
      const lockedFresh = policyViewOf(lockedIssue.answer_policy, countsOfTemplate(template));
      if (lockedFresh.warning !== shown.warning) {
        holdBack(lockedFresh);
        return done;
      }
      return send(lockedIssue);
    }
    if (!sameView(fresh, shown)) {
      holdBack(fresh);
      return done;
    }
    const body = issueBody(fresh.policy, template.id);
    return body === null ? done : send(body);
  }

  function saveAndIssue(shown: PolicyView): void {
    const found = clientProblems();
    if (found.length > 0) {
      problems.refuse(found);
      return;
    }
    problems.cleared();
    setNotice({ kind: 'none' });
    policy.freeze();
    const blocks = taskBlocks(instructionBlockId, instruction, readyMaterialIds(attached), questions, () =>
      crypto.randomUUID(),
    );
    action.run(async (): Promise<ActionState> => {
      const answer = await postTemplate(client, templateKey, title.trim(), blocks);
      if (answer.kind === 'refused') {
        setTemplateUncertain(!answer.certain);
        if (answer.certain) {
          setTemplateKey(crypto.randomUUID());
          policy.reset();
        }
        const state: ActionState = { kind: 'refused', error: answer.error, retryAfter: answer.retryAfter };
        if (answer.error.code === 'email_not_verified') {
          setNotice({ kind: 'problem', problem: { kind: 'confirmation' }, count: 1 });
          return state;
        }
        const message =
          answer.error.code === 'payload_too_large' ||
          answer.error.fieldCode('blocks') === 'too_many' ||
          answer.error.fieldCode('body') === 'too_many'
            ? t('homework.newTask.problems.tooManyBlocks')
            : refusalText(t, answer.error, answer.retryAfter, undefined);
        setNotice((previous) => ({
          kind: 'problem',
          problem: { kind: 'alert', message, builderLink: null },
          count: previous.kind === 'problem' ? previous.count + 1 : 1,
        }));
        return state;
      }
      setTemplateUncertain(false);
      void queryClient.invalidateQueries({ queryKey: libraryKey, refetchType: 'none' });
      return issueTemplate(answer.value, shown);
    });
  }

  function issueSaved(templateId: string, shown: PolicyView): void {
    setNotice({ kind: 'none' });
    policy.freeze();
    action.run(async (): Promise<ActionState> => {
      const answer = await readTemplate(client, templateId);
      if (answer.kind === 'refused') {
        if (answer.error.code === 'not_found') {
          setNotice({ kind: 'gone' });
        }
        policy.fail();
        return { kind: 'refused', error: answer.error, retryAfter: answer.retryAfter };
      }
      return issueTemplate(answer.value, shown);
    });
  }

  function issue(): void {
    if (saved === null) {
      saveAndIssue(policy.view);
      return;
    }
    issueSaved(saved.id, lockedIssue === null ? policy.view : policy.lockedView(lockedIssue.answer_policy));
  }

  function recheck(): void {
    if (saved === null) {
      return;
    }
    action.run(async (): Promise<ActionState> => {
      const answer = await readTemplate(client, saved.id);
      if (answer.kind === 'refused') {
        if (answer.error.code === 'not_found') {
          setNotice({ kind: 'gone' });
        }
        policy.fail();
        return { kind: 'refused', error: answer.error, retryAfter: answer.retryAfter };
      }
      policy.know(answer.value);
      return done;
    });
  }

  const summaryOrder = [
    ...contentFields.slice(0, 3),
    ...questions.flatMap((question) =>
      (['prompt', 'options', 'answers', 'value', 'tolerance'] as const).map((field) => problemKey(question.id, field)),
    ),
    'blocks',
    'audience',
    'deadline',
  ];

  function summaryTarget(field: string): HTMLElement | null {
    switch (field) {
      case 'title':
        return document.getElementById(titleId);
      case 'instruction':
        return document.getElementById(instructionFieldId);
      case 'uploads':
      case 'blocks':
        return document.getElementById(attachmentsId);
      case 'audience':
        return audienceSearch.current ?? audienceChange.current;
      case 'deadline':
        return deadlineDateField.current;
      default: {
        const [questionId, questionField] = field.split('.');
        return questionId === undefined || questionField === undefined
          ? null
          : document.getElementById(`question-${questionId}-${questionField}`);
      }
    }
  }

  const checkFailed = policy.server.kind === 'failed' && notice.kind !== 'gone' && saved !== null;
  const unavailableReason = checkFailed
    ? t('homework.newTask.checkFailed')
    : notice.kind === 'gone'
      ? t('homework.refusals.notFound')
      : undefined;

  return (
    <div className="flex flex-col gap-6">
      <ErrorSummary
        attempt={problems.attempt}
        problems={problems.listed(summaryOrder).map((problem) => ({
          key: problem.field,
          message: problem.message,
          target: () => summaryTarget(problem.field),
        }))}
      />
      {saved === null ? null : (
        <div className="flex flex-wrap items-center gap-x-3">
          <p className="text-small text-text-muted">{t('homework.newTask.saved')}</p>
          <Link to={builderAddress(saved.id)} className={linkClass}>
            {t('homework.refusals.openInBuilder')}
          </Link>
        </div>
      )}
      <div className="flex flex-col gap-1">
        <label htmlFor={titleId} className="text-label">
          {t('homework.newTask.title')}
        </label>
        <input
          id={titleId}
          value={title}
          readOnly={contentLocked}
          maxLength={taskTitleLimit}
          aria-invalid={problems.message('title') === undefined ? undefined : true}
          aria-describedby={problems.message('title') === undefined ? undefined : `${titleId}-error`}
          onChange={(event) => {
            setTitle(event.target.value);
            problems.edited(
              'title',
              event.target.value.trim() === '' ? t('homework.newTask.problems.title') : undefined,
            );
            contentEdited();
          }}
          className={fieldClass}
        />
        {problems.message('title') === undefined ? null : (
          <p id={`${titleId}-error`} className="text-small text-danger">
            {problems.message('title')}
          </p>
        )}
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor={instructionFieldId} className="text-label">
          {t('homework.newTask.instruction')}
        </label>
        <textarea
          id={instructionFieldId}
          value={instruction}
          rows={4}
          readOnly={contentLocked}
          maxLength={instructionLimit}
          aria-invalid={problems.message('instruction') === undefined ? undefined : true}
          aria-describedby={problems.message('instruction') === undefined ? undefined : `${instructionFieldId}-error`}
          onChange={(event) => {
            setInstruction(event.target.value);
            problems.edited(
              'instruction',
              event.target.value.trim() === '' ? t('homework.newTask.problems.instruction') : undefined,
            );
            contentEdited();
          }}
          className={fieldClass}
        />
        {problems.message('instruction') === undefined ? null : (
          <p id={`${instructionFieldId}-error`} className="text-small text-danger">
            {problems.message('instruction')}
          </p>
        )}
      </div>
      <div id={attachmentsId} tabIndex={-1} className="outline-none">
        <TaskAttachmentsField attachments={attachments} locked={contentLocked} />
        {problems.message('uploads') === undefined ? null : (
          <p className="text-small text-danger">{problems.message('uploads')}</p>
        )}
        {problems.message('blocks') === undefined ? null : (
          <p className="text-small text-danger">{problems.message('blocks')}</p>
        )}
      </div>
      <TaskQuestions
        questions={questions}
        problems={problems.message}
        locked={contentLocked}
        full={blockCount(attached.length, questions.length) >= blockLimit}
        onAdd={(type: QuestionType) => {
          setQuestions((previous) => [...previous, newQuestion(type, () => crypto.randomUUID())]);
          problems.edited('blocks', undefined);
          contentEdited();
        }}
        onChange={changeQuestion}
        onRemove={(questionId) => {
          setQuestions((previous) => previous.filter((question) => question.id !== questionId));
          for (const field of ['prompt', 'options', 'answers', 'value', 'tolerance'] as const) {
            problems.edited(problemKey(questionId, field), undefined);
          }
          contentEdited();
        }}
      />
      <fieldset disabled={issueLocked} className="flex min-w-0 flex-col gap-4">
        <AudiencePicker
          chosen={audience}
          preselected={null}
          groupsWithMembers
          searchField={audienceSearch}
          changeButton={audienceChange}
          error={problems.message('audience')}
          onChoose={(chosen) => {
            setAudience(chosen);
            problems.edited('audience', undefined);
          }}
          onClear={() => {
            setAudience(null);
          }}
        />
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
            problems.edited(
              'deadline',
              deadlineInstant(value, deadlineClock, me.timezone) === null
                ? t('homework.refusals.futureDeadline')
                : undefined,
            );
          }}
          onClock={(value) => {
            setDeadlineClock(value);
            problems.edited(
              'deadline',
              deadlineInstant(deadlineDate, value, me.timezone) === null
                ? t('homework.refusals.futureDeadline')
                : undefined,
            );
          }}
        />
        <PolicyDisclosure
          view={lockedIssue === null ? policy.view : policy.lockedView(lockedIssue.answer_policy)}
          locked={issueLocked}
          onChoose={policy.choose}
        />
      </fieldset>
      {checkFailed ? (
        <div className="flex flex-wrap items-center gap-3">
          <FormAlert>{t('homework.newTask.checkFailed')}</FormAlert>
          <ActionButton busy={action.busy} onClick={recheck} className={quietButtonClass}>
            {t('homework.newTask.checkAgain')}
          </ActionButton>
        </div>
      ) : null}
      <NoticeView notice={notice} saved={saved} />
      <FormActions>
        <ActionButton
          busy={action.busy}
          unavailable={checkFailed || notice.kind === 'gone'}
          unavailableReason={unavailableReason}
          onClick={issue}
          className={primaryButtonClass}
        >
          {t('homework.submit')}
        </ActionButton>
        <CancelButton busy={action.busy} />
      </FormActions>
    </div>
  );
}

function CancelButton({ busy }: { readonly busy: boolean }): ReactElement {
  const { t } = useTranslation();
  const { leave } = useLeaveFormRoute('/homework/library');
  return (
    <ActionButton
      busy={false}
      unavailable={busy}
      onClick={() => {
        leave();
      }}
      className={quietButtonClass}
    >
      {t('homework.cancel')}
    </ActionButton>
  );
}

function NewTaskContent(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const { backAddress, leave } = useLeaveFormRoute('/homework/library');
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
          {t('homework.newTask.back')}
        </Link>
      </p>
      {me.email_verified ? (
        <NewTaskForm
          onIssued={(issueId) => {
            leave({ kind: 'assignmentIssued', issueId });
          }}
        />
      ) : (
        <ConfirmEmailPrompt />
      )}
    </Page>
  );
}

export function NewTaskPage(): ReactElement {
  const texts = useFeatureTexts('homework', homeworkTextLoaders);
  if (!texts.ready) {
    return (
      <Page width="reading">
        {texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />}
      </Page>
    );
  }
  return <NewTaskContent />;
}
