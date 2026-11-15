import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  type ReactElement,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams, useSearchParams } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import {
  FormAlert,
  errorMessage,
  fieldClass,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed, focusIsLost } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiError, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { Markdown } from '../../lib/markdown/Markdown.tsx';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { ConfirmEmailPrompt, homeworkTextLoaders } from '../homework/IssueAssignment.tsx';
import { ReclaimCue, SubmissionState, scoreText } from '../homework/IssueParts.tsx';
import {
  type PublishAction,
  type PublishOutcome,
  type ReviewAnswer,
  type ReviewFile,
  type ReviewQuestion,
  type ReviewSubmission,
  type SaveOutcome,
  fetchReviewSubmission,
  filterFrom,
  findNextSubmission,
  isManual,
  publishReview,
  queueAddress,
  reviewAddress,
  saveDraft,
} from './api.ts';
import { type CorrectedAlert, CorrectedFiles } from './CorrectedFiles.tsx';
import {
  type CorrectedEntry,
  type Draft,
  type DraftItem,
  baseRevision,
  draftBody,
  draftFrom,
  draftPosition,
  emptyItem,
  withItem,
} from './draft.ts';
import { DraftSave, type DraftSaveStatus } from './draftSave.ts';
import { type QueueFilter, nextSubmissionKey, reviewSubmissionKey } from './keys.ts';
import { QuestionAnswer } from './ReviewAnswer.tsx';

const expiryMarginMilliseconds = 30_000;
const minimumRefetchMilliseconds = 30_000;
const fieldPattern = /^items\[(\d+)\]\.(\w+)$/;

type NoticeKind = 'notEditable' | 'checked' | 'returned';

interface Counted<Value> {
  readonly value: Value;
  readonly count: number;
}

type FileAlerts = ReadonlyMap<string, { readonly reason: CorrectedAlert | 'fileGone'; readonly count: number }>;

function reviewable(submission: ReviewSubmission): boolean {
  return submission.status === 'submitted' || submission.status === 'checked';
}

function storedFiles(submission: ReviewSubmission, question: string): readonly ReviewFile[] {
  return baseRevision(submission)?.items.get(question)?.correctedFiles ?? [];
}

function newerDraft(cached: ReviewSubmission | undefined, saved: ReviewSubmission): ReviewSubmission | undefined {
  const savedDraft = saved.draft;
  const cachedDraft = cached?.draft ?? null;
  if (cached === undefined || cachedDraft === null) {
    return saved;
  }
  if (savedDraft === null) {
    return cached;
  }
  const newer =
    savedDraft.revision > cachedDraft.revision ||
    (savedDraft.revision === cachedDraft.revision && savedDraft.version > cachedDraft.version);
  return newer ? saved : cached;
}

function earliestExpiry(submission: ReviewSubmission): number | null {
  const files: ReviewFile[] = [];
  for (const question of submission.questions) {
    files.push(...question.images, ...storedFiles(submission, question.id));
    files.push(...(submission.answers.get(question.id)?.files ?? []));
  }
  const times = files.map((file) => Date.parse(file.expiresAt)).filter((time) => Number.isFinite(time));
  return times.length === 0 ? null : Math.min(...times);
}

function fieldId(question: string, field: string): string {
  return `review-${question}-${field}`;
}

function useCounter<Value>(): [Counted<Value> | null, (value: Value | null) => void] {
  const [state, setState] = useState<Counted<Value> | null>(null);
  const set = useCallback((value: Value | null): void => {
    setState((previous) => (value === null ? null : { value, count: (previous?.count ?? 0) + 1 }));
  }, []);
  return [state, set];
}

function FocusedNotice({
  message,
  role = 'status',
}: {
  readonly message: string;
  readonly role?: 'status' | 'alert';
}): ReactElement {
  const element = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    element.current?.focus();
  }, []);
  return (
    <p ref={element} role={role} tabIndex={-1} className="text-text outline-none">
      {message}
    </p>
  );
}

function ConflictNotice({
  busy,
  failure,
  onReload,
}: {
  readonly busy: boolean;
  readonly failure: ApiError | null;
  readonly onReload: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const element = useRef<HTMLDivElement>(null);
  useEffect(() => {
    element.current?.focus();
  }, []);
  return (
    <div
      ref={element}
      role="alert"
      tabIndex={-1}
      className="flex flex-col items-start gap-3 rounded-md border border-warning p-4 outline-none"
    >
      <p className="text-strong">{t('homework.review.conflict')}</p>
      {failure === null ? null : <p className="text-danger">{errorMessage(t, failure, null)}</p>}
      <button
        type="button"
        aria-disabled={busy}
        aria-busy={busy}
        onClick={() => {
          if (!busy) {
            onReload();
          }
        }}
        className={quietButtonClass}
      >
        {t('homework.review.reload')}
      </button>
    </div>
  );
}

function SaveStatusLine({
  status,
  onRetry,
}: {
  readonly status: DraftSaveStatus;
  readonly onRetry: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const sending = status === 'pending' || status === 'saving';
  const text = sending
    ? t('homework.review.saving')
    : status === 'saved'
      ? t('homework.review.saved')
      : status === 'failed' || status === 'refused'
        ? t('homework.review.notSaved')
        : '';
  return (
    <span className="inline-flex flex-wrap items-center gap-3">
      <span role="status" className="text-small text-text-muted">
        {text}
      </span>
      {status === 'failed' ? (
        <button type="button" onClick={onRetry} className={textButtonClass}>
          {t('homework.review.retry')}
        </button>
      ) : null}
    </span>
  );
}

function PointsInput({
  id,
  label,
  value,
  max,
  error,
  onChange,
}: {
  readonly id: string;
  readonly label: string;
  readonly value: string;
  readonly max: number;
  readonly error: string | undefined;
  readonly onChange: (value: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  const hintId = useId();
  const errorId = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-label">
        {label}
      </label>
      <input
        id={id}
        type="number"
        inputMode="numeric"
        min={0}
        max={max}
        step={1}
        value={value}
        aria-invalid={error === undefined ? undefined : true}
        aria-describedby={error === undefined ? hintId : `${hintId} ${errorId}`}
        onChange={(event) => {
          onChange(event.currentTarget.value);
        }}
        className={`${fieldClass} w-28`}
      />
      <span id={hintId} className="text-small text-text-muted">
        {t('homework.review.outOf', { count: max })}
      </span>
      {error === undefined ? null : (
        <span id={errorId} className="text-small text-danger">
          {error}
        </span>
      )}
    </div>
  );
}

function QuestionScoring({
  submission,
  question,
  answer,
  item,
  errors,
  missing,
  fileAlert,
  onChange,
}: {
  readonly submission: ReviewSubmission;
  readonly question: ReviewQuestion;
  readonly answer: ReviewAnswer | undefined;
  readonly item: DraftItem;
  readonly errors: ReadonlyMap<string, string>;
  readonly missing: boolean;
  readonly fileAlert: { readonly reason: CorrectedAlert | 'fileGone'; readonly count: number } | null;
  readonly onChange: (item: DraftItem) => void;
}): ReactElement {
  const { t } = useTranslation();
  const overrideButton = useRef<HTMLButtonElement>(null);
  const [focusRequest, setFocusRequest] = useState<Counted<'override' | 'points'> | null>(null);
  useEffect(() => {
    if (focusRequest === null) {
      return;
    }
    if (focusRequest.value === 'points') {
      document.getElementById(fieldId(question.id, 'override_points'))?.focus();
    } else {
      overrideButton.current?.focus();
    }
  }, [focusRequest, question.id]);
  const commentId = fieldId(question.id, 'comment');
  const commentError = errors.get(`${question.id}:comment`);
  return (
    <div className="flex min-w-0 flex-col gap-4">
      {isManual(question) ? (
        <PointsInput
          id={fieldId(question.id, 'points')}
          label={t('homework.review.points')}
          value={item.points}
          max={question.points}
          error={missing ? t('homework.review.enterPoints') : errors.get(`${question.id}:points`)}
          onChange={(points) => {
            onChange({ ...item, points });
          }}
        />
      ) : (
        <div className="flex flex-col items-start gap-2">
          <p className="flex flex-wrap items-center gap-x-3 text-small">
            <span>{t('homework.auto_points', { count: answer?.autoPoints ?? 0 })}</span>
            <span className="text-text-muted">{t('homework.review.outOf', { count: question.points })}</span>
            {answer?.autoCorrect === null || answer === undefined ? null : (
              <span>{answer.autoCorrect ? t('homework.review.correct') : t('homework.review.incorrect')}</span>
            )}
          </p>
          {item.overriding ? (
            <>
              <PointsInput
                id={fieldId(question.id, 'override_points')}
                label={t('homework.review.overridePoints')}
                value={item.overridePoints}
                max={question.points}
                error={errors.get(`${question.id}:override_points`)}
                onChange={(overridePoints) => {
                  onChange({ ...item, overridePoints });
                }}
              />
              <div className="flex w-full flex-col gap-1">
                <label htmlFor={fieldId(question.id, 'override_reason')} className="text-label">
                  {t('homework.review.reason')}
                </label>
                <input
                  id={fieldId(question.id, 'override_reason')}
                  type="text"
                  maxLength={500}
                  value={item.overrideReason}
                  aria-invalid={errors.has(`${question.id}:override_reason`) ? true : undefined}
                  onChange={(event) => {
                    onChange({ ...item, overrideReason: event.currentTarget.value });
                  }}
                  className={fieldClass}
                />
                {errors.has(`${question.id}:override_reason`) ? (
                  <span className="text-small text-danger">{errors.get(`${question.id}:override_reason`)}</span>
                ) : null}
              </div>
              <button
                type="button"
                onClick={() => {
                  onChange({ ...item, overriding: false, overridePoints: '', overrideReason: '' });
                  setFocusRequest((previous) => ({ value: 'override', count: (previous?.count ?? 0) + 1 }));
                }}
                className={textButtonClass}
              >
                {t('homework.review.removeOverride')}
              </button>
            </>
          ) : (
            <button
              ref={overrideButton}
              type="button"
              onClick={() => {
                onChange({ ...item, overriding: true });
                setFocusRequest((previous) => ({ value: 'points', count: (previous?.count ?? 0) + 1 }));
              }}
              className={quietButtonClass}
            >
              {t('homework.review.override')}
            </button>
          )}
        </div>
      )}
      <div className="flex flex-col gap-1">
        <label htmlFor={commentId} className="text-label">
          {t('homework.review.comment')}
        </label>
        <textarea
          id={commentId}
          rows={3}
          maxLength={10_000}
          dir="auto"
          value={item.comment}
          aria-invalid={commentError === undefined ? undefined : true}
          onChange={(event) => {
            onChange({ ...item, comment: event.currentTarget.value });
          }}
          className={`${fieldClass} h-auto py-2`}
        />
        {commentError === undefined ? null : <span className="text-small text-danger">{commentError}</span>}
      </div>
      {question.type === 'file' ? (
        <CorrectedFiles
          submissionId={submission.id}
          questionId={question.id}
          files={item.files}
          stored={storedFiles(submission, question.id)}
          external={fileAlert}
          onChange={(files: readonly CorrectedEntry[]) => {
            onChange({ ...item, files });
          }}
        />
      ) : null}
    </div>
  );
}

function ReadOnlyScoring({
  submission,
  question,
}: {
  readonly submission: ReviewSubmission;
  readonly question: ReviewQuestion;
}): ReactElement | null {
  const { t } = useTranslation();
  const item = submission.published?.items.get(question.id);
  if (item === undefined || item.comment === '') {
    return null;
  }
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <span className="text-label text-text-muted">{t('homework.review.comment')}</span>
      <p dir="auto" className="break-words whitespace-pre-wrap">
        {item.comment}
      </p>
    </div>
  );
}

function FeedbackField({
  value,
  error,
  onChange,
}: {
  readonly value: string;
  readonly error: string | undefined;
  readonly onChange: (value: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  const id = useId();
  const [mode, setMode] = useState<'write' | 'preview'>('write');
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <label htmlFor={id} className="text-strong">
          {t('homework.review.feedback')}
        </label>
        <div role="group" className="flex gap-1 rounded-md bg-hover p-1">
          {(['write', 'preview'] as const).map((entry) => (
            <button
              key={entry}
              type="button"
              aria-pressed={mode === entry}
              onClick={() => {
                setMode(entry);
              }}
              className="inline-flex min-h-11 items-center rounded-sm border border-transparent px-3 text-small text-text-muted hover:text-text aria-pressed:border-border-strong aria-pressed:bg-field aria-pressed:font-medium aria-pressed:text-text desktop:pointer-fine:min-h-10"
            >
              {t(`homework.review.${entry}`)}
            </button>
          ))}
        </div>
      </div>
      <textarea
        id={id}
        hidden={mode === 'preview'}
        rows={6}
        maxLength={10_000}
        dir="auto"
        value={value}
        aria-invalid={error === undefined ? undefined : true}
        onChange={(event) => {
          onChange(event.currentTarget.value);
        }}
        className={`${fieldClass} h-auto py-2`}
      />
      {mode === 'preview' ? (
        <div data-feedback-preview="" className="min-h-24 rounded-md border border-border p-3">
          <Markdown source={value} />
        </div>
      ) : null}
      {error === undefined ? null : <span className="text-small text-danger">{error}</span>}
    </div>
  );
}

function NextLink({
  submission,
  filter,
}: {
  readonly submission: ReviewSubmission;
  readonly filter: QueueFilter;
}): ReactElement | null {
  const { t } = useTranslation();
  const client = useApiClient();
  const first = submission.firstSubmittedAt;
  const next = useQuery({
    queryKey: nextSubmissionKey(submission.id, filter),
    queryFn: ({ signal }) =>
      findNextSubmission(client, filter, { id: submission.id, firstSubmittedAt: first ?? '' }, signal),
    enabled: first !== null,
    retry: false,
  });
  if (next.data === undefined || next.data === null) {
    return null;
  }
  return (
    <Link to={reviewAddress(next.data, filter)} className={linkClass}>
      {t('homework.review.next')}
    </Link>
  );
}

function ReviewForm({
  submission,
  filter,
  gone,
}: {
  readonly submission: ReviewSubmission;
  readonly filter: QueueFilter;
  readonly gone: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const session = useSession();
  const [sessionEpoch] = useState(() => session.currentEpoch());
  const scheduler = useScheduler();
  const key = reviewSubmissionKey(submission.id);
  const [draft, setDraft] = useState<Draft>(() => draftFrom(submission));
  const [errors, setErrors] = useState<ReadonlyMap<string, string>>(new Map());
  const [feedbackError, setFeedbackError] = useState<string | undefined>(undefined);
  const [missing, setMissing] = useState<ReadonlySet<string>>(new Set());
  const [fileAlerts, setFileAlerts] = useState<FileAlerts>(new Map());
  const [alert, showAlert] = useCounter<string>();
  const [notice, showNotice] = useCounter<NoticeKind>();
  const [reloading, setReloading] = useState(false);
  const [reloadFailure, setReloadFailure] = useState<ApiError | null>(null);
  const [publishing, setPublishing] = useState<PublishAction | null>(null);
  const [focusField, requestFocusField] = useCounter<string>();
  const [focusHeading, requestFocusHeading] = useCounter<true>();
  const failedAddresses = useRef(new Set<string>());

  useEffect(() => {
    if (focusField !== null) {
      document.getElementById(focusField.value)?.focus();
    }
  }, [focusField]);
  useEffect(() => {
    if (focusHeading !== null) {
      document.querySelector<HTMLElement>('main h1')?.focus();
    }
  }, [focusHeading]);

  const reload = useCallback(
    (): Promise<ReviewSubmission> =>
      queryClient.query({
        queryKey: key,
        queryFn: ({ signal }) => fetchReviewSubmission(client, submission.id, signal),
        staleTime: 0,
        retry: false,
      }),
    [queryClient, key, client, submission.id],
  );

  const [save] = useState(
    () =>
      new DraftSave({
        scheduler,
        position: draftPosition(submission),
        body: draftBody(submission.questions, draft),
        newSaveId: (): string => crypto.randomUUID(),
        send: (request): Promise<SaveOutcome> => saveDraft(client, submission.id, request),
      }),
  );
  const status = useSyncExternalStore(save.subscribe, save.snapshot);
  const newestVersion = useRef(submission.version);
  const publishingNow = useRef(false);
  const editedWhilePublishing = useRef(false);
  const draftValue = useRef(draft);
  const questionsValue = useRef(submission.questions);
  useEffect(() => {
    draftValue.current = draft;
    questionsValue.current = submission.questions;
  }, [draft, submission.questions]);

  useEffect(() => {
    save.resume();
    return (): void => {
      save.leave();
    };
  }, [save]);

  useEffect(() => {
    if (gone) {
      save.stop();
    }
  }, [save, gone]);

  const wasReviewable = useRef(reviewable(submission));
  const publishedHere = useRef(false);
  useEffect(() => {
    if (submission.version < newestVersion.current) {
      return;
    }
    const now = reviewable(submission);
    if (wasReviewable.current && !now && !publishedHere.current) {
      save.stop();
      showNotice('notEditable');
    }
    wasReviewable.current = now;
    publishedHere.current = false;
  }, [save, submission, showNotice]);

  useEffect(() => {
    if (reviewable(submission) && submission.version >= newestVersion.current) {
      save.observed(draftPosition(submission));
    }
  }, [save, submission]);

  useEffect(() => {
    const expiry = earliestExpiry(submission);
    if (expiry === null) {
      return undefined;
    }
    const wait = Math.max(minimumRefetchMilliseconds, expiry - expiryMarginMilliseconds - scheduler.wallClock());
    return scheduler.schedule(wait, () => {
      void queryClient.invalidateQueries({ queryKey: key });
    });
  }, [submission, scheduler, queryClient, key]);

  function replaceDraft(fresh: ReviewSubmission): void {
    const next = draftFrom(fresh);
    draftValue.current = next;
    setDraft(next);
    setErrors(new Map());
    setFeedbackError(undefined);
    setMissing(new Set());
    save.reloaded(draftPosition(fresh), draftBody(fresh.questions, next));
  }

  async function reloadAfterChange(): Promise<void> {
    try {
      replaceDraft(await reload());
    } catch {
      void queryClient.invalidateQueries({ queryKey: key });
    }
    showNotice('notEditable');
  }

  async function dropUnboundFiles(error: ApiError, reason: 'fileGone' | 'storageFull'): Promise<void> {
    let fresh: ReviewSubmission;
    try {
      fresh = await reload();
    } catch {
      return;
    }
    const alerts = new Map(fileAlerts);
    let next = draftValue.current;
    let dropped = 0;
    for (const question of fresh.questions) {
      const item = next.items.get(question.id);
      if (question.type !== 'file' || item === undefined) {
        continue;
      }
      const stored = storedFiles(fresh, question.id).map((file) => file.attachmentId);
      const kept = item.files.filter((file) => stored.includes(file.attachmentId));
      if (kept.length !== item.files.length) {
        dropped += item.files.length - kept.length;
        next = withItem(next, question.id, { ...item, files: kept });
        alerts.set(question.id, { reason, count: (alerts.get(question.id)?.count ?? 0) + 1 });
      }
    }
    if (dropped === 0) {
      showAlert(errorMessage(t, error, null));
      return;
    }
    draftValue.current = next;
    setDraft(next);
    setFileAlerts(alerts);
    save.changed(draftBody(fresh.questions, next));
  }

  function showInvalid(error: ApiError): void {
    const problems = new Map<string, string>();
    let first: string | null = null;
    let feedback: string | undefined;
    for (const problem of error.fields) {
      const message = t(`fieldErrors.${problem.code}`);
      if (problem.field === 'feedback') {
        feedback = message;
        continue;
      }
      if (problem.field === 'body' && problem.code === 'too_many') {
        showAlert(t('homework.review.revisionLimit'));
        continue;
      }
      const match = fieldPattern.exec(problem.field);
      const question = match === null ? undefined : questionsValue.current[Number(match[1])];
      if (match === null || question === undefined) {
        continue;
      }
      const field = match[2] ?? '';
      const target = field === 'corrected_attachment_ids' ? 'comment' : field;
      problems.set(`${question.id}:${target}`, message);
      first ??= fieldId(question.id, target);
    }
    setErrors(problems);
    setFeedbackError(feedback);
    if (first !== null) {
      requestFocusField(first);
    }
  }

  useEffect(() => {
    save.handle({
      onSaved: (saved) => {
        queryClient.setQueryData(key, saved);
        setErrors(new Map());
        setFeedbackError(undefined);
      },
      onSavedAfterLeaving: (saved) => {
        if (session.currentEpoch() !== sessionEpoch) {
          return;
        }
        queryClient.setQueryData<ReviewSubmission>(key, (cached) => newerDraft(cached, saved));
      },
      onRefused: (refusal) => {
        switch (refusal.kind) {
          case 'notEditable':
            void reloadAfterChange();
            return;
          case 'fileGone':
            void dropUnboundFiles(refusal.error, 'fileGone');
            return;
          case 'storageFull':
            void dropUnboundFiles(refusal.error, 'storageFull');
            return;
          case 'unconfirmed':
            showAlert(t('homework.confirmEmail'));
            return;
          case 'invalid':
            showInvalid(refusal.error);
            return;
          case 'refused':
            showAlert(errorMessage(t, refusal.error, null));
        }
      },
    });
  });

  function editedMeanwhile(): boolean {
    return editedWhilePublishing.current;
  }

  function change(next: Draft): void {
    if (publishingNow.current) {
      editedWhilePublishing.current = true;
    }
    draftValue.current = next;
    setDraft(next);
    save.changed(draftBody(submission.questions, next));
  }

  async function reloadConflict(): Promise<void> {
    setReloading(true);
    setReloadFailure(null);
    try {
      replaceDraft(await reload());
      requestFocusHeading(true);
    } catch (thrown: unknown) {
      setReloadFailure(asApiError(thrown));
    } finally {
      setReloading(false);
    }
  }

  async function publish(action: PublishAction): Promise<void> {
    if (publishing !== null) {
      return;
    }
    setPublishing(action);
    showAlert(null);
    setMissing(new Set());
    const clean = await save.flush(true);
    if (!clean) {
      setPublishing(null);
      return;
    }
    publishingNow.current = true;
    editedWhilePublishing.current = false;
    save.hold();
    const outcome = await publishReview(client, submission.id, action, save.currentPosition.revision);
    setPublishing(null);
    publishingNow.current = false;
    await settlePublication(action, outcome);
    save.release();
  }

  async function settlePublication(action: PublishAction, outcome: PublishOutcome): Promise<void> {
    switch (outcome.kind) {
      case 'published':
        newestVersion.current = Math.max(newestVersion.current, outcome.submission.version);
        publishedHere.current = true;
        queryClient.setQueryData(key, outcome.submission);
        if (editedMeanwhile() && reviewable(outcome.submission)) {
          const kept = draftBody(outcome.submission.questions, draftValue.current);
          save.reloaded(draftPosition(outcome.submission), kept);
          save.changed(kept);
        } else {
          replaceDraft(outcome.submission);
        }
        showNotice(action === 'check' ? 'checked' : 'returned');
        return;
      case 'conflict':
      case 'notEditable':
        await reloadAfterChange();
        return;
      case 'gone':
        void queryClient.invalidateQueries({ queryKey: key });
        return;
      case 'returnNotAllowed':
        showAlert(t('homework.review.returnNotAllowed'));
        return;
      case 'incomplete':
        if (action === 'return') {
          showAlert(t('homework.review.returnNeedsComment'));
          return;
        }
        setMissing(
          new Set(
            submission.questions
              .filter(
                (question) => isManual(question) && (draftValue.current.items.get(question.id)?.points ?? '') === '',
              )
              .map((question) => question.id),
          ),
        );
        showAlert(t('homework.review.enterAllPoints'));
        return;
      case 'failed':
        showAlert(errorMessage(t, outcome.error, outcome.retryAfterSeconds));
    }
  }

  const editable = reviewable(submission) && status !== 'stopped';
  const manual = submission.questions.some(isManual);
  const canReturn = submission.status === 'submitted' && submission.answerPolicy !== 'after_submission' && manual;
  const showTotal =
    submission.finalPoints !== null && (submission.published !== null || submission.status === 'checked');
  const onFailed = (address: string): void => {
    if (!failedAddresses.current.has(address)) {
      failedAddresses.current.add(address);
      void queryClient.invalidateQueries({ queryKey: key });
    }
  };
  const numbers = useMemo(
    () => new Map(submission.questions.map((question, index) => [question.id, index + 1])),
    [submission.questions],
  );

  if (gone) {
    return <></>;
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <span dir="auto" className="text-strong">
          {submission.studentName}
        </span>
        <SubmissionState
          status={submission.status}
          late={submission.late}
          manual={manual}
          draft={submission.draft !== null}
        />
        {editable ? (
          <SaveStatusLine
            status={status}
            onRetry={() => {
              save.retryNow();
            }}
          />
        ) : null}
      </div>
      {submission.issuedBeforeReclaim ? <ReclaimCue /> : null}
      {notice === null ? null : (
        <FocusedNotice
          key={notice.count}
          message={
            notice.value === 'checked'
              ? t('homework.review.checked')
              : notice.value === 'returned'
                ? t('homework.review.returned')
                : t('homework.review.notEditable')
          }
        />
      )}
      {status === 'conflict' ? (
        <ConflictNotice
          busy={reloading}
          failure={reloadFailure}
          onReload={() => {
            void reloadConflict();
          }}
        />
      ) : null}
      {submission.status === 'not_started' || submission.status === 'draft' ? (
        <p>{t('homework.review.notSubmitted')}</p>
      ) : null}
      {submission.status === 'closed' ? <p>{t('homework.review.closed')}</p> : null}
      <ol className="flex flex-col">
        {submission.questions.map((question) => (
          <li key={question.id} className="border-t border-border py-6">
            <section
              aria-labelledby={fieldId(question.id, 'heading')}
              className="grid gap-6 wide:grid-cols-2 wide:gap-10"
            >
              <QuestionAnswer
                headingId={fieldId(question.id, 'heading')}
                number={numbers.get(question.id) ?? 0}
                question={question}
                answer={submission.answers.get(question.id)}
                onFailed={onFailed}
              />
              {editable ? (
                <QuestionScoring
                  submission={submission}
                  question={question}
                  answer={submission.answers.get(question.id)}
                  item={draft.items.get(question.id) ?? emptyItem}
                  errors={errors}
                  missing={missing.has(question.id)}
                  fileAlert={fileAlerts.get(question.id) ?? null}
                  onChange={(item) => {
                    change(withItem(draftValue.current, question.id, item));
                  }}
                />
              ) : (
                <ReadOnlyScoring submission={submission} question={question} />
              )}
            </section>
          </li>
        ))}
      </ol>
      {editable ? (
        <div className="flex flex-col gap-4 border-t border-border pt-6">
          <FeedbackField
            value={draft.feedback}
            error={feedbackError}
            onChange={(feedback) => {
              change({ ...draftValue.current, feedback });
            }}
          />
          {showTotal ? (
            <p className="text-strong">
              {t('homework.review.total')}:{' '}
              {scoreText(t, me.language, submission.finalPoints, submission.maxPoints, submission.percentage)}
            </p>
          ) : null}
          {alert === null ? null : <FormAlert key={alert.count}>{alert.value}</FormAlert>}
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={() => {
                void save.flush();
              }}
              className={quietButtonClass}
            >
              {t('homework.review.saveDraft')}
            </button>
            {canReturn ? (
              <button
                type="button"
                aria-disabled={publishing !== null}
                aria-busy={publishing === 'return'}
                onClick={() => {
                  void publish('return');
                }}
                className={quietButtonClass}
              >
                {t('homework.review.return')}
              </button>
            ) : null}
            <button
              type="button"
              aria-disabled={publishing !== null}
              aria-busy={publishing === 'check'}
              onClick={() => {
                void publish('check');
              }}
              className={primaryButtonClass}
            >
              {t('homework.review.check')}
            </button>
          </div>
        </div>
      ) : showTotal ? (
        <p className="text-strong">
          {t('homework.review.total')}:{' '}
          {scoreText(t, me.language, submission.finalPoints, submission.maxPoints, submission.percentage)}
        </p>
      ) : null}
      <p>
        <NextLink submission={submission} filter={filter} />
      </p>
    </>
  );
}

function ReviewContent({
  submissionId,
  filter,
}: {
  readonly submissionId: string;
  readonly filter: QueueFilter;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const query = useQuery({
    queryKey: reviewSubmissionKey(submissionId),
    queryFn: ({ signal }) => fetchReviewSubmission(client, submissionId, signal),
    retry: false,
  });
  const queryError = query.isError ? asApiError(query.error) : null;
  const gone = queryError?.status === 404 && query.data !== undefined;
  const notFound = queryError?.status === 404 && query.data === undefined;
  const submission = gone || notFound ? undefined : query.data;

  if (notFound) {
    return (
      <Page title="page.notFound" focusTitle>
        <NotFoundContent />
      </Page>
    );
  }

  function body(): ReactElement {
    if (query.data === undefined) {
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
      <>
        <ReviewForm submission={query.data} filter={filter} gone={gone} />
        {gone ? (
          <>
            <GoneNotice />
            <p>
              <Link to={queueAddress(filter)} className={linkClass}>
                {t('homework.review.back')}
              </Link>
            </p>
          </>
        ) : null}
      </>
    );
  }

  return (
    <Page {...(submission === undefined ? {} : { headingText: submission.title, headingDir: 'auto' as const })}>
      <p>
        <Link to={queueAddress(filter)} className={linkClass}>
          {t('homework.review.back')}
        </Link>
      </p>
      {queryError !== null && queryError.status !== 404 && query.data !== undefined ? (
        <RefetchFailed
          error={queryError}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : null}
      {body()}
    </Page>
  );
}

function GoneNotice(): ReactElement {
  const { t } = useTranslation();
  const element = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (focusIsLost()) {
      element.current?.focus();
    }
  }, []);
  return (
    <p ref={element} role="status" tabIndex={-1} className="text-text outline-none">
      {t('homework.gone')}
    </p>
  );
}

export function ReviewPage(): ReactElement {
  const { submissionId = '' } = useParams();
  const [params] = useSearchParams();
  const texts = useFeatureTexts('homework', homeworkTextLoaders);
  const me = useSignedInAccount();
  const filter = filterFrom(params);
  if (!texts.ready || !me.email_verified) {
    return (
      <Page>
        {!texts.ready ? (
          texts.failed ? (
            <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} />
          ) : (
            <DelayedLoading />
          )
        ) : (
          <ConfirmEmailPrompt />
        )}
      </Page>
    );
  }
  return <ReviewContent key={`${me.id}:${submissionId}`} submissionId={submissionId} filter={filter} />;
}
