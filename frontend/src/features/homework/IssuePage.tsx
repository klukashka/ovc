import { useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type ReactNode, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';

import { type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { ActionMenu, type MenuItem } from '../../components/ui/ActionMenu.tsx';
import {
  DataTable,
  type TableColumn,
  cellClass,
  primaryCellClass,
  rowLinkClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormAlert,
  SelectChevron,
  dangerButtonClass,
  fieldClass,
  linkClass,
  primaryButtonClass,
  selectClass,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { FocusedNotice, LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed, focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { PageColumns } from '../../components/ui/PageColumns.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { type ApiClient, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSignedInAccount } from '../../lib/api/session.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { formatDate, formatDateTime } from '../../lib/format/dates.ts';
import {
  addDays,
  isLocalDate,
  localDateOf,
  localInstant,
  localMinutesOf,
  localTimeOf,
} from '../../lib/time/localDates.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { fetchGroup, fetchGroups, groupKey, groupsKey } from '../groups/api.ts';
import { fetchLesson } from '../schedule/api.ts';
import { lessonKey } from '../schedule/keys.ts';
import { fetchStudentCard, studentCardKey } from '../students/api.ts';
import { type Issue, type IssueItem, type IssuePatch, fetchIssue, issueKey, issuesKey } from './api.ts';
import { ConfirmEmailPrompt, homeworkTextLoaders } from './IssueAssignment.tsx';
import { AudienceLabel, DeadlineText, ReclaimCue, SubmissionState, scoreText } from './IssueParts.tsx';

type DialogKind = 'deadline' | 'lesson' | 'withdraw';

interface OpenDialog {
  readonly kind: DialogKind;
  readonly version: number;
}

type PageNotice = 'changed' | 'withdrawn' | 'hasSubmissions';

const deadlineDays = 7;
const endOfDay = 23 * 60 + 59;
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

async function patchIssue(client: ApiClient, id: string, patch: IssuePatch): Promise<IssueItem | ActionState> {
  const { data, error, response } = await client.PATCH('/api/v1/issues/{id}', {
    params: { path: { id } },
    body: patch,
  });
  return data ?? refusedBy(response, error);
}

async function withdrawIssue(client: ApiClient, id: string): Promise<IssueItem | ActionState> {
  const { data, error, response } = await client.POST('/api/v1/issues/{id}/withdraw', { params: { path: { id } } });
  return data ?? refusedBy(response, error);
}

const closingRefusals: ReadonlySet<string> = new Set(['issue_has_submissions', 'conflict', 'not_found']);

function isIssueItem(value: IssueItem | ActionState): value is IssueItem {
  return 'id' in value;
}

function withdrawable(issue: Issue): boolean {
  return issue.status === 'active' && issue.submissions.every((submission) => submission.first_submitted_at === null);
}

interface Handlers {
  readonly onBusy: (busy: boolean) => void;
  readonly onDismiss: () => void;
  readonly onSaved: (announcement: string) => void;
  readonly onConflict: () => void;
  readonly onGone: () => void;
}

function useSavePatch(
  issue: Issue,
  handlers: Handlers,
): {
  readonly busy: boolean;
  readonly save: (
    patch: Omit<IssuePatch, 'version'>,
    announcement: string,
    refused: (state: ActionState) => void,
  ) => void;
} {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const action = useAction();
  return {
    busy: action.busy,
    save: (patch, announcement, refused): void => {
      handlers.onBusy(true);
      action.run(async (): Promise<ActionState> => {
        const answer = await patchIssue(client, issue.id, { ...patch, version: issue.version });
        handlers.onBusy(false);
        if (isIssueItem(answer)) {
          handlers.onSaved(announcement);
          void queryClient.invalidateQueries({ queryKey: issuesKey });
          return done;
        }
        if (answer.kind === 'refused' && answer.error.code === 'conflict') {
          handlers.onConflict();
        } else if (answer.kind === 'refused' && answer.error.code === 'not_found' && patch.lesson_id === undefined) {
          handlers.onGone();
        } else {
          refused(answer);
        }
        return answer;
      });
    },
  };
}

interface DialogProblem {
  readonly field: boolean;
  readonly message: string;
  readonly count: number;
}

function nextProblem(previous: DialogProblem | null, field: boolean, message: string): DialogProblem {
  return { field, message, count: (previous?.count ?? 0) + 1 };
}

function DeadlineDialog({
  issue,
  open,
  returnFocus,
  handlers,
}: {
  readonly issue: Issue;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly handlers: Handlers;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const headingId = useId();
  const dateId = useId();
  const clockId = useId();
  const errorId = useId();
  const dateField = useRef<HTMLInputElement>(null);
  const initial = issue.deadline_at === null ? null : Date.parse(issue.deadline_at);
  const [none, setNone] = useState(initial === null);
  const [date, setDate] = useState<string>(() =>
    initial === null ? addDays(localDateOf(Date.now(), me.timezone), deadlineDays) : localDateOf(initial, me.timezone),
  );
  const [clock, setClock] = useState(() =>
    localTimeOf(initial === null ? endOfDay : localMinutesOf(initial, me.timezone)),
  );
  const [problem, setProblem] = useState<DialogProblem | null>(null);
  const { busy, save } = useSavePatch(issue, handlers);
  const fieldError = problem?.field === true ? problem.message : null;

  useEffect(() => {
    if (problem?.field === true) {
      dateField.current?.focus();
    }
  }, [problem]);

  function invalid(): void {
    setProblem((previous) => nextProblem(previous, true, t('homework.refusals.futureDeadline')));
  }

  function refused(state: ActionState): void {
    if (state.kind !== 'refused') {
      return;
    }
    if (state.error.fieldCode('deadline_at') !== undefined) {
      invalid();
      return;
    }
    setProblem((previous) => nextProblem(previous, false, refusalText(t, state.error, state.retryAfter, undefined)));
  }

  function submit(): void {
    if (none) {
      save({ deadline_at: null }, t('homework.announcements.deadlineChanged'), refused);
      return;
    }
    const minutes = minutesOf(clock);
    if (!isLocalDate(date) || minutes === null) {
      invalid();
      return;
    }
    save(
      { deadline_at: new Date(localInstant(date, minutes, me.timezone)).toISOString() },
      t('homework.announcements.deadlineChanged'),
      refused,
    );
  }

  return (
    <ModalDialog
      open={open}
      busy={busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={dateField}
      onClose={handlers.onDismiss}
    >
      <div className="flex flex-col gap-4">
        <h2 id={headingId} className="font-serif text-title">
          {t('homework.actions.changeDeadline')}
        </h2>
        <div className="grid grid-cols-2 gap-3">
          <div className="flex flex-col gap-1">
            <label htmlFor={dateId} className="text-label">
              {t('homework.fields.deadlineDate')}
            </label>
            <input
              ref={dateField}
              id={dateId}
              type="date"
              value={date}
              aria-invalid={fieldError === null ? undefined : true}
              aria-describedby={fieldError === null ? undefined : errorId}
              onChange={(event) => {
                setDate(event.target.value);
                setNone(false);
              }}
              className={fieldClass}
            />
          </div>
          <div className="flex flex-col gap-1">
            <label htmlFor={clockId} className="text-label">
              {t('homework.fields.deadlineTime')}
            </label>
            <input
              id={clockId}
              type="time"
              value={clock}
              aria-invalid={fieldError === null ? undefined : true}
              aria-describedby={fieldError === null ? undefined : errorId}
              onChange={(event) => {
                setClock(event.target.value);
                setNone(false);
              }}
              className={fieldClass}
            />
          </div>
        </div>
        {fieldError === null ? null : (
          <p id={errorId} className="text-small text-danger">
            {fieldError}
          </p>
        )}
        <label className="inline-flex min-h-11 items-center gap-2 text-small">
          <input
            type="checkbox"
            checked={none}
            onChange={(event) => {
              setNone(event.target.checked);
              setProblem(null);
            }}
            className="size-4 accent-accent"
          />
          {t('homework.list.noDeadline')}
        </label>
        {problem === null || problem.field ? null : <FormAlert key={problem.count}>{problem.message}</FormAlert>}
        <DialogButtons busy={busy} confirm={t('homework.save')} onConfirm={submit} onCancel={handlers.onDismiss} />
      </div>
    </ModalDialog>
  );
}

interface LessonChoice {
  readonly id: string;
  readonly label: string;
}

function lessonLabel(title: string, startsAt: string, me: Me): string {
  return `${formatDateTime(startsAt, me.language, me.timezone)} · ${title}`;
}

function useAudienceLessons(issue: Issue, open: boolean): readonly LessonChoice[] {
  const client = useApiClient();
  const me = useSignedInAccount();
  const relationship = issue.audience.relationship_id;
  const groupId = issue.audience.group_id;
  const card = useQuery({
    queryKey: studentCardKey(relationship ?? ''),
    queryFn: () => fetchStudentCard(client, relationship ?? ''),
    enabled: open && relationship !== undefined,
    retry: false,
  });
  const group = useQuery({
    queryKey: groupKey(groupId ?? ''),
    queryFn: () => fetchGroup(client, groupId ?? ''),
    enabled: open && groupId !== undefined,
    retry: false,
  });
  const lessons = (relationship === undefined ? group.data?.next_lessons : card.data?.next_lessons) ?? [];
  return lessons.map((lesson) => ({ id: lesson.id, label: lessonLabel(lesson.title, lesson.starts_at, me) }));
}

function useLinkedLesson(issue: Issue, enabled: boolean): { readonly label: string | null } {
  const client = useApiClient();
  const me = useSignedInAccount();
  const lessonId = issue.lesson_id;
  const lesson = useQuery({
    queryKey: lessonKey(lessonId ?? ''),
    queryFn: () => fetchLesson(client, lessonId ?? ''),
    enabled: enabled && lessonId !== null,
    retry: false,
  });
  return { label: lesson.data === undefined ? null : lessonLabel(lesson.data.title, lesson.data.starts_at, me) };
}

function LessonDialog({
  issue,
  open,
  returnFocus,
  handlers,
}: {
  readonly issue: Issue;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly handlers: Handlers;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const selectId = useId();
  const errorId = useId();
  const field = useRef<HTMLSelectElement>(null);
  const lessons = useAudienceLessons(issue, open);
  const current = useLinkedLesson(issue, open);
  const [value, setValue] = useState(issue.lesson_id ?? '');
  const [problem, setProblem] = useState<DialogProblem | null>(null);
  const fieldError = problem?.field === true ? problem.message : null;
  const { busy, save } = useSavePatch(issue, handlers);
  const pinned =
    issue.lesson_id !== null && !lessons.some((lesson) => lesson.id === issue.lesson_id)
      ? { id: issue.lesson_id, label: current.label ?? t('page.lesson') }
      : null;

  useEffect(() => {
    if (problem?.field === true) {
      field.current?.focus();
    }
  }, [problem]);

  function refused(state: ActionState): void {
    if (state.kind !== 'refused') {
      return;
    }
    if (state.error.fieldCode('lesson_id') === 'too_many') {
      setProblem((previous) => nextProblem(previous, true, t('homework.refusals.lessonLimit')));
    } else if (state.error.code === 'not_found') {
      setProblem((previous) => nextProblem(previous, true, t('homework.refusals.lessonGone')));
    } else {
      setProblem((previous) => nextProblem(previous, false, refusalText(t, state.error, state.retryAfter, undefined)));
    }
  }

  return (
    <ModalDialog
      open={open}
      busy={busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={field}
      onClose={handlers.onDismiss}
    >
      <div className="flex flex-col gap-4">
        <h2 id={headingId} className="font-serif text-title">
          {t('homework.actions.changeLesson')}
        </h2>
        <div className="flex flex-col gap-1">
          <label htmlFor={selectId} className="text-label">
            {t('homework.fields.lesson')}
          </label>
          <div className="relative">
            <select
              ref={field}
              id={selectId}
              value={value}
              aria-invalid={fieldError === null ? undefined : true}
              aria-describedby={fieldError === null ? undefined : errorId}
              onChange={(event) => {
                setValue(event.target.value);
                setProblem(null);
              }}
              className={selectClass}
            >
              <option value="">{t('homework.fields.noLesson')}</option>
              {pinned === null ? null : (
                <optgroup label={t('homework.fields.chosen')}>
                  <option value={pinned.id}>{pinned.label}</option>
                </optgroup>
              )}
              {lessons.map((lesson) => (
                <option key={lesson.id} value={lesson.id}>
                  {lesson.label}
                </option>
              ))}
            </select>
            <SelectChevron />
          </div>
          {fieldError === null ? null : (
            <p id={errorId} className="text-small text-danger">
              {fieldError}
            </p>
          )}
        </div>
        {problem === null || problem.field ? null : <FormAlert key={problem.count}>{problem.message}</FormAlert>}
        <DialogButtons
          busy={busy}
          confirm={t('homework.save')}
          onConfirm={() => {
            save({ lesson_id: value === '' ? null : value }, t('homework.announcements.lessonChanged'), refused);
          }}
          onCancel={handlers.onDismiss}
        />
      </div>
    </ModalDialog>
  );
}

function DialogButtons({
  busy,
  confirm,
  danger = false,
  cancelRef,
  onConfirm,
  onCancel,
}: {
  readonly busy: boolean;
  readonly confirm: string;
  readonly danger?: boolean;
  readonly cancelRef?: RefObject<HTMLButtonElement | null>;
  readonly onConfirm: () => void;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-3">
      <ActionButton busy={busy} onClick={onConfirm} className={danger ? dangerButtonClass : primaryButtonClass}>
        {confirm}
      </ActionButton>
      <ActionButton
        busy={false}
        unavailable={busy}
        onClick={onCancel}
        className={textButtonClass}
        {...(cancelRef === undefined ? {} : { ref: cancelRef })}
      >
        {t('homework.cancel')}
      </ActionButton>
    </div>
  );
}

function WithdrawDialog({
  issue,
  open,
  returnFocus,
  onDismiss,
  onBusy,
  onWithdrawn,
  onRefused,
}: {
  readonly issue: Issue;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onDismiss: () => void;
  readonly onBusy: (busy: boolean) => void;
  readonly onWithdrawn: () => void;
  readonly onRefused: (state: ActionState) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const bodyId = useId();
  const cancel = useRef<HTMLButtonElement>(null);
  const action = useAction();
  const [alert, setAlert] = useState<{ readonly message: string; readonly count: number } | null>(null);
  return (
    <ModalDialog
      open={open}
      busy={action.busy}
      labelledBy={headingId}
      describedBy={bodyId}
      returnFocus={returnFocus}
      initialFocus={cancel}
      onClose={onDismiss}
    >
      <div className="flex flex-col gap-4">
        <h2 id={headingId} className="font-serif text-title">
          {t('homework.withdrawConfirm.title')}
        </h2>
        <p id={bodyId}>{t('homework.withdrawConfirm.body')}</p>
        {alert === null ? null : <FormAlert key={alert.count}>{alert.message}</FormAlert>}
        <DialogButtons
          busy={action.busy}
          confirm={t('homework.withdrawConfirm.confirm')}
          danger
          cancelRef={cancel}
          onConfirm={() => {
            onBusy(true);
            action.run(async (): Promise<ActionState> => {
              const answer = await withdrawIssue(client, issue.id);
              onBusy(false);
              if (isIssueItem(answer)) {
                onWithdrawn();
                return done;
              }
              if (answer.kind === 'refused' && closingRefusals.has(answer.error.code ?? '')) {
                onRefused(answer);
              } else if (answer.kind === 'refused') {
                const message = refusalText(t, answer.error, answer.retryAfter, undefined);
                setAlert((previous) => ({ message, count: (previous?.count ?? 0) + 1 }));
              }
              return answer;
            });
          }}
          onCancel={onDismiss}
        />
      </div>
    </ModalDialog>
  );
}

function Detail({ term, children }: { readonly term: string; readonly children: ReactNode }): ReactElement {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-label text-text-muted">{term}</dt>
      <dd className="break-words">{children}</dd>
    </div>
  );
}

function AboutIssue({ issue }: { readonly issue: Issue }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const me = useSignedInAccount();
  const headingId = useId();
  const groups = useQuery({ queryKey: groupsKey, queryFn: () => fetchGroups(client), retry: false });
  const linked = useLinkedLesson(issue, true);
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('homework.issuePage.about')}
      </h2>
      <dl className="flex flex-col gap-3">
        <Detail term={t('homework.list.audience')}>
          <AudienceLabel audience={issue.audience} groups={groups.data ?? []} />
        </Detail>
        <Detail term={t('homework.list.issued')}>{formatDate(issue.issued_at, me.language, me.timezone)}</Detail>
        <Detail term={t('homework.list.deadline')}>
          <DeadlineText deadline={issue.deadline_at} />
        </Detail>
        <Detail term={t('homework.fields.lesson')}>
          {issue.lesson_id === null ? (
            t('homework.fields.noLesson')
          ) : (
            <Link to={`/lessons/${encodeURIComponent(issue.lesson_id)}`} dir="auto" className={linkClass}>
              {linked.label ?? t('page.lesson')}
            </Link>
          )}
        </Detail>
        <Detail term={t('homework.fields.policy')}>{t(`homework.policies.${issue.answer_policy}`)}</Detail>
        <Detail term={t('homework.issuePage.points')}>
          {t('homework.issuePage.maxPoints', { count: issue.max_points })}
        </Detail>
      </dl>
      {issue.has_manual_questions ? <p className="text-small">{t('homework.issuePage.manual')}</p> : null}
      {issue.withdrawn_at === null ? null : (
        <p className="text-small text-text-muted">
          {t('homework.issuePage.withdrawnOn', { date: formatDate(issue.withdrawn_at, me.language, me.timezone) })}
        </p>
      )}
    </section>
  );
}

function Submissions({ issue }: { readonly issue: Issue }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const headingId = useId();
  const columns: readonly TableColumn[] = [
    { key: 'student', label: t('homework.issuePage.student'), width: 'w-5/12' },
    { key: 'state', label: t('homework.issuePage.state'), width: 'w-4/12' },
    { key: 'points', label: t('homework.issuePage.points'), width: 'w-3/12' },
  ];
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('homework.issuePage.submissions')}
      </h2>
      <DataTable label={t('homework.issuePage.submissions')} columns={columns}>
        {issue.submissions.map((submission) => (
          <tr key={submission.id} data-row={submission.id} className={tableRowClass}>
            <th scope="row" className={primaryCellClass}>
              <Link
                data-row-link=""
                to={`/homework/review/${encodeURIComponent(submission.id)}?issue=${encodeURIComponent(issue.id)}`}
                dir="auto"
                className={rowLinkClass}
              >
                {submission.student.display_name}
              </Link>
              {submission.issued_before_reclaim ? <ReclaimCue /> : null}
            </th>
            <td className={cellClass}>
              <SubmissionState
                status={submission.status}
                late={submission.late}
                manual={issue.has_manual_questions}
                draft={submission.has_draft}
              />
            </td>
            <td className={cellClass}>
              <span className="text-small">
                {issue.max_points === 0 || (submission.status === 'checked' && submission.final_points !== null)
                  ? scoreText(t, me.language, submission.final_points ?? 0, issue.max_points, submission.percentage)
                  : null}
              </span>
            </td>
          </tr>
        ))}
      </DataTable>
    </section>
  );
}

function IssueActions({
  issue,
  menuButton,
  onOpen,
}: {
  readonly issue: Issue;
  readonly menuButton: RefObject<HTMLButtonElement | null>;
  readonly onOpen: (kind: DialogKind) => void;
}): ReactElement {
  const { t } = useTranslation();
  const items: MenuItem[] = [
    {
      label: t('homework.actions.changeDeadline'),
      onChoose: (): void => {
        onOpen('deadline');
      },
    },
    {
      label: t('homework.actions.changeLesson'),
      onChoose: (): void => {
        onOpen('lesson');
      },
    },
    ...(withdrawable(issue)
      ? [
          {
            label: t('homework.actions.withdraw'),
            onChoose: (): void => {
              onOpen('withdraw');
            },
          },
        ]
      : []),
  ];
  return (
    <ActionMenu label={t('homework.issuePage.actions')} unavailable={false} items={items} buttonRef={menuButton}>
      <MoreIcon />
    </ActionMenu>
  );
}

function NextFrameAlert({ message }: { readonly message: string }): ReactElement {
  const element = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      element.current?.focus();
    });
    return (): void => {
      window.cancelAnimationFrame(frame);
    };
  }, []);

  return (
    <p ref={element} role="alert" tabIndex={-1} className="text-danger outline-none">
      {message}
    </p>
  );
}

function useHeadingWhenMenuLeaves(shown: boolean): void {
  const shownBefore = useRef(shown);
  useEffect(() => {
    if (shownBefore.current && !shown) {
      focusHeadingIfLost();
    }
    shownBefore.current = shown;
  }, [shown]);
}

function IssueContent({ issueId }: { readonly issueId: string }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const menuButton = useRef<HTMLButtonElement>(null);
  const [dialog, setDialog] = useState<OpenDialog | null>(null);
  const [openings, setOpenings] = useState(0);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState<{ readonly kind: PageNotice; readonly count: number } | null>(null);
  const [announcement, setAnnouncement] = useState<string | null>(null);
  const key = issueKey(issueId);
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => fetchIssue(client, issueId, signal),
    retry: false,
  });
  const queryError = query.isError ? asApiError(query.error) : null;
  const gone = queryError?.code === 'not_found' && query.data !== undefined;
  const notFound = queryError?.code === 'not_found' && query.data === undefined;
  const refetchError = queryError !== null && queryError.code !== 'not_found' && query.data !== undefined;
  const issue = gone || notFound ? undefined : query.data;
  const menuShown = issue?.status === 'active';
  useHeadingWhenMenuLeaves(menuShown);

  if (
    dialog !== null &&
    !saving &&
    issue !== undefined &&
    (issue.version !== dialog.version || issue.status !== 'active')
  ) {
    setDialog(null);
    setNotice((previous) => ({ kind: 'changed', count: (previous?.count ?? 0) + 1 }));
  }
  if (dialog !== null && issue === undefined) {
    setDialog(null);
  }

  function showNotice(kind: PageNotice): void {
    setNotice((previous) => ({ kind, count: (previous?.count ?? 0) + 1 }));
  }

  function reload(): void {
    void queryClient.invalidateQueries({ queryKey: issuesKey });
  }

  const handlers: Handlers = {
    onBusy: setSaving,
    onDismiss: () => {
      setDialog(null);
    },
    onSaved: (message) => {
      setDialog(null);
      setNotice(null);
      setAnnouncement(message);
    },
    onConflict: () => {
      setDialog(null);
      setAnnouncement(null);
      showNotice('changed');
      reload();
    },
    onGone: () => {
      setDialog(null);
      reload();
    },
  };

  if (notFound) {
    return (
      <Page title="page.notFound" focusTitle>
        <NotFoundContent />
      </Page>
    );
  }

  function body(): ReactElement {
    if (gone) {
      return (
        <>
          <FocusedNotice message={t('homework.gone')} whenFocusIsLost />
          <p>
            <Link to="/homework/issued" className={linkClass}>
              {t('homework.issuePage.back')}
            </Link>
          </p>
        </>
      );
    }
    if (issue === undefined) {
      return query.isError ? (
        <LoadFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : (
        <DelayedLoading />
      );
    }
    return (
      <PageColumns
        asideLabel={t('homework.issuePage.about')}
        main={<Submissions issue={issue} />}
        aside={<AboutIssue issue={issue} />}
      />
    );
  }

  return (
    <Page
      {...(issue === undefined ? {} : { headingText: issue.title, headingDir: 'auto' as const })}
      action={
        issue?.status === 'withdrawn' ? <StatusWord tone="muted">{t('homework.states.withdrawn')}</StatusWord> : null
      }
      menu={
        issue !== undefined && menuShown ? (
          <IssueActions
            issue={issue}
            menuButton={menuButton}
            onOpen={(kind) => {
              setNotice(null);
              setAnnouncement(null);
              setOpenings((count) => count + 1);
              setDialog({ kind, version: issue.version });
            }}
          />
        ) : null
      }
    >
      <p>
        <Link to="/homework/issued" className={linkClass}>
          {t('homework.issuePage.back')}
        </Link>
      </p>
      {notice === null || gone ? null : notice.kind === 'hasSubmissions' ? (
        <NextFrameAlert key={notice.count} message={t('homework.hasSubmissions')} />
      ) : (
        <FocusedNotice
          key={notice.count}
          message={notice.kind === 'changed' ? t('homework.changedMeanwhile') : t('homework.withdrawnNotice')}
        />
      )}
      <LiveNotice message={announcement} />
      {refetchError ? (
        <RefetchFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : null}
      {body()}
      {issue === undefined ? null : (
        <>
          <DeadlineDialog
            key={`deadline-${String(openings)}`}
            issue={issue}
            open={dialog?.kind === 'deadline'}
            returnFocus={menuButton}
            handlers={handlers}
          />
          <LessonDialog
            key={`lesson-${String(openings)}`}
            issue={issue}
            open={dialog?.kind === 'lesson'}
            returnFocus={menuButton}
            handlers={handlers}
          />
          <WithdrawDialog
            issue={issue}
            key={`withdraw-${String(openings)}`}
            open={dialog?.kind === 'withdraw'}
            returnFocus={menuButton}
            onDismiss={handlers.onDismiss}
            onBusy={setSaving}
            onWithdrawn={() => {
              setDialog(null);
              setAnnouncement(null);
              showNotice('withdrawn');
              reload();
            }}
            onRefused={(state) => {
              setDialog(null);
              reload();
              if (state.kind === 'refused' && state.error.code === 'issue_has_submissions') {
                showNotice('hasSubmissions');
              } else if (state.kind === 'refused' && state.error.code === 'conflict') {
                showNotice('changed');
              }
            }}
          />
        </>
      )}
    </Page>
  );
}

export function IssuePage(): ReactElement {
  const { issueId = '' } = useParams();
  const texts = useFeatureTexts('homework', homeworkTextLoaders);
  const me = useSignedInAccount();
  if (!texts.ready) {
    return (
      <Page>
        {texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />}
      </Page>
    );
  }
  if (!me.email_verified) {
    return (
      <Page>
        <ConfirmEmailPrompt />
      </Page>
    );
  }
  return <IssueContent key={`${me.id}:${issueId}`} issueId={issueId} />;
}
