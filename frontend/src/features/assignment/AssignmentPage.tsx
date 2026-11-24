import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  type ReactElement,
  type ReactNode,
  type RefObject,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  FormAlert,
  errorMessage,
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
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import {
  type Answer,
  type QuestionBlock,
  type SaveOutcome,
  type SentAnswer,
  type Submission,
  answered,
  assignmentsKey,
  emptyAnswer,
  fetchSubmission,
  isQuestion,
  sameAnswer,
  saveAnswers,
  submissionKey,
  submitWork,
} from './api.ts';
import { AnswerFiles, useFileDropGuard } from './AnswerFiles.tsx';
import { AnswerAutosave, type Refusal } from './autosave.ts';
import { firstFieldOf } from './focusQuestion.ts';
import { SaveStatus } from './SaveStatus.tsx';
import { DeadlineHint, ReclaimCue, StateLabel, TutorName, useNow } from './parts.tsx';
import { OverallFeedback, QuestionReview, RevealedPart } from './ReviewParts.tsx';
import { type Answering, StudentBlocks, questionNumbers } from './StudentBlocks.tsx';
import { assignmentTextLoaders } from './texts.ts';
import { useSeenMarker } from './seenMarker.ts';

const expiryMarginMilliseconds = 30_000;
const minimumRefetchMilliseconds = 30_000;

type NoticeKind = 'changed' | 'submitted';

type FocusTarget =
  | { readonly kind: 'useOrHeading' }
  | { readonly kind: 'useOrField'; readonly question: string }
  | { readonly kind: 'field'; readonly question: string };

type FileRefusalReason = 'storageFull' | 'unavailable';

interface FileRefusal {
  readonly reason: FileRefusalReason;
  readonly count: number;
}

function earliestExpiry(submission: Submission): number | null {
  const instants: string[] = [];
  for (const block of submission.blocks) {
    if (block.type === 'material' && block.material !== null) {
      instants.push(block.material.expiresAt);
    }
    if (isQuestion(block)) {
      instants.push(...block.images.map((image) => image.expiresAt));
    }
  }
  for (const answer of submission.answers.values()) {
    instants.push(...answer.files.map((file) => file.expiresAt));
  }
  for (const item of submission.review?.items.values() ?? []) {
    instants.push(...item.correctedFiles.map((file) => file.expiresAt));
  }
  const times = instants.map((instant) => Date.parse(instant)).filter((time) => Number.isFinite(time));
  return times.length === 0 ? null : Math.min(...times);
}

function withEntry(map: ReadonlyMap<string, Answer>, key: string, value: Answer): ReadonlyMap<string, Answer> {
  const next = new Map(map);
  next.set(key, value);
  return next;
}

function withoutEntry(map: ReadonlyMap<string, Answer>, key: string): ReadonlyMap<string, Answer> {
  const next = new Map(map);
  next.delete(key);
  return next;
}

function answerText(block: QuestionBlock, answer: Answer): string {
  switch (block.type) {
    case 'single_choice':
    case 'multiple_choice':
      return block.options
        .filter((option) => answer.optionIds.includes(option.id))
        .map((option) => option.text)
        .join(', ');
    case 'file':
      return answer.files.map((file) => file.filename).join(', ');
    case 'short_text':
    case 'short_number':
    case 'long_text':
      return answer.text;
  }
}

function PageNotice({
  message,
  whenFocusIsLost = false,
  onShown,
}: {
  readonly message: string;
  readonly whenFocusIsLost?: boolean;
  readonly onShown?: (notice: HTMLElement) => void;
}): ReactElement {
  const notice = useRef<HTMLParagraphElement>(null);
  const onShownValue = useRef(onShown);
  useEffect(() => {
    const element = notice.current;
    if (element === null) {
      return;
    }
    onShownValue.current?.(element);
    if (!whenFocusIsLost || focusIsLost()) {
      element.focus();
    }
  }, [whenFocusIsLost]);
  return (
    <p ref={notice} role="status" tabIndex={-1} className="text-text outline-none">
      {message}
    </p>
  );
}

function ConflictBanner({
  busy,
  failure,
  onLoad,
  onShown,
}: {
  readonly busy: boolean;
  readonly failure: ApiError | null;
  readonly onLoad: () => void;
  readonly onShown: (banner: HTMLElement) => void;
}): ReactElement {
  const { t } = useTranslation();
  const banner = useRef<HTMLDivElement>(null);
  const onShownValue = useRef(onShown);
  useEffect(() => {
    const element = banner.current;
    if (element !== null) {
      onShownValue.current(element);
      element.focus();
    }
  }, []);
  return (
    <div
      ref={banner}
      role="alert"
      tabIndex={-1}
      className="flex flex-col items-start gap-3 rounded-md border border-warning p-4 outline-none"
    >
      <p className="text-strong">{t('assignment.conflict.banner')}</p>
      {failure === null ? null : <p className="text-danger">{errorMessage(t, failure, null)}</p>}
      <button
        type="button"
        aria-disabled={busy}
        aria-busy={busy}
        onClick={() => {
          if (!busy) {
            onLoad();
          }
        }}
        className={quietButtonClass}
      >
        {t('assignment.conflict.load')}
      </button>
    </div>
  );
}

function SubmitDialog({
  open,
  busy,
  unanswered,
  failure,
  retryAfter,
  returnFocus,
  onConfirm,
  onClose,
}: {
  readonly open: boolean;
  readonly busy: boolean;
  readonly unanswered: readonly number[];
  readonly failure: ApiError | null;
  readonly retryAfter: number | null;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onConfirm: () => void;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const titleId = useId();
  const cancel = useRef<HTMLButtonElement>(null);
  return (
    <ModalDialog
      open={open}
      busy={busy}
      labelledBy={titleId}
      returnFocus={returnFocus}
      initialFocus={cancel}
      onClose={onClose}
    >
      <div className="flex flex-col gap-4">
        <h2 id={titleId} className="text-heading">
          {t('assignment.submit.title')}
        </h2>
        {unanswered.length > 0 ? (
          <div className="flex flex-col gap-1">
            <p>{t('assignment.submit.unanswered')}</p>
            <ul className="list-disc ps-5">
              {unanswered.map((number) => (
                <li key={number}>{t('assignment.question', { number })}</li>
              ))}
            </ul>
          </div>
        ) : null}
        <p className="text-text-muted">{t('assignment.submit.warning')}</p>
        {failure === null ? null : <FormAlert>{errorMessage(t, failure, retryAfter)}</FormAlert>}
        <div className="flex flex-wrap justify-end gap-2">
          <button
            ref={cancel}
            type="button"
            aria-disabled={busy}
            onClick={() => {
              if (!busy) {
                onClose();
              }
            }}
            className={quietButtonClass}
          >
            {t('assignment.submit.cancel')}
          </button>
          <button
            type="button"
            aria-disabled={busy}
            aria-busy={busy}
            onClick={() => {
              if (!busy) {
                onConfirm();
              }
            }}
            className={primaryButtonClass}
          >
            {t('assignment.submit.confirm')}
          </button>
        </div>
      </div>
    </ModalDialog>
  );
}

function AssignmentForm({
  submission,
  notice,
  onNotice,
  gone,
}: {
  readonly submission: Submission;
  readonly gone: boolean;
  readonly notice: { readonly kind: NoticeKind; readonly count: number } | null;
  readonly onNotice: (kind: NoticeKind | null) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const session = useSession();
  const [sessionEpoch] = useState(() => session.currentEpoch());
  useFileDropGuard();
  const scheduler = useScheduler();
  const me = useSignedInAccount();
  const now = useNow();
  const key = submissionKey(submission.id);
  const [local, setLocal] = useState<ReadonlyMap<string, Answer>>(new Map());
  const [kept, setKept] = useState<ReadonlyMap<string, Answer>>(new Map());
  const [invalid, setInvalid] = useState<ReadonlyMap<string, ApiError>>(new Map());
  const [fileRefusals, setFileRefusals] = useState<ReadonlyMap<string, FileRefusal>>(new Map());
  const [pageRefusal, setPageRefusal] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadFailure, setLoadFailure] = useState<ApiError | null>(null);
  const [submitOpen, setSubmitOpen] = useState(false);
  const [submitBusy, setSubmitBusy] = useState(false);
  const [submitFailure, setSubmitFailure] = useState<ApiError | null>(null);
  const [submitRetryAfter, setSubmitRetryAfter] = useState<number | null>(null);
  const [preparing, setPreparing] = useState(false);
  const [uploading, setUploading] = useState<ReadonlySet<string>>(new Set());
  const uploadingChanged = useCallback((question: string, busy: boolean): void => {
    setUploading((previous) => {
      if (previous.has(question) === busy) {
        return previous;
      }
      const next = new Set(previous);
      if (busy) {
        next.add(question);
      } else {
        next.delete(question);
      }
      return next;
    });
  }, []);
  const submitButton = useRef<HTMLButtonElement>(null);
  const dialogReturn = useRef<HTMLElement | null>(null);
  const [focusRequest, setFocusRequest] = useState<{ readonly target: FocusTarget; readonly count: number } | null>(
    null,
  );
  const requestFocus = useCallback((target: FocusTarget): void => {
    setFocusRequest((previous) => ({ target, count: (previous?.count ?? 0) + 1 }));
  }, []);
  useEffect(() => {
    if (focusRequest === null) {
      return;
    }
    const target = focusRequest.target;
    const use = document.querySelector<HTMLElement>('[data-use-answer]');
    switch (target.kind) {
      case 'useOrHeading':
        (use ?? document.querySelector<HTMLElement>('main h1'))?.focus();
        return;
      case 'useOrField':
        (use ?? firstFieldOf(target.question))?.focus();
        return;
      case 'field':
        firstFieldOf(target.question)?.focus();
    }
  }, [focusRequest]);
  const failedAddresses = useRef(new Set<string>());

  const displayed = useMemo((): ReadonlyMap<string, Answer> => {
    const answers = new Map(submission.answers);
    for (const [question, answer] of local) {
      answers.set(question, answer);
    }
    return answers;
  }, [submission.answers, local]);
  const displayedValue = useRef(displayed);
  const submissionValue = useRef(submission);
  useEffect(() => {
    displayedValue.current = displayed;
    submissionValue.current = submission;
  }, [displayed, submission]);

  const questions = useMemo(() => submission.blocks.filter(isQuestion), [submission.blocks]);
  const numbers = useMemo(() => questionNumbers(submission.blocks), [submission.blocks]);

  const reload = useCallback(async (): Promise<Submission> => {
    const fresh = await queryClient.query({
      queryKey: key,
      queryFn: ({ signal }) => fetchSubmission(client, submission.id, signal),
      staleTime: 0,
      retry: false,
    });
    return fresh;
  }, [queryClient, key, client, submission.id]);

  const [autosave] = useState(
    () =>
      new AnswerAutosave({
        scheduler,
        version: submission.version,
        newSaveId: (): string => crypto.randomUUID(),
        collect: (ids): readonly SentAnswer[] =>
          submissionValue.current.blocks
            .filter(isQuestion)
            .filter((block) => ids.includes(block.id))
            .map((block) => ({ block, answer: displayedValue.current.get(block.id) ?? emptyAnswer })),
        send: (request): Promise<SaveOutcome> => saveAnswers(client, submission.id, request),
        onSaved: (saved, sent): void => {
          void queryClient.cancelQueries({ queryKey: key });
          queryClient.setQueryData(key, saved);
          setPageRefusal(null);
          setLocal((previous) => {
            let next = previous;
            for (const entry of sent) {
              const current = next.get(entry.block.id);
              if (current !== undefined && sameAnswer(entry.block, current, entry.answer)) {
                next = withoutEntry(next, entry.block.id);
              }
            }
            return next;
          });
        },
        onSavedAfterLeaving: (saved): void => {
          if (session.currentEpoch() !== sessionEpoch) {
            return;
          }
          queryClient.setQueryData<Submission>(key, (cached) =>
            cached === undefined || cached.version < saved.version ? saved : cached,
          );
        },
        onRefused: (refusal: Refusal, sent): void => {
          refusalHandler.current(refusal, sent);
        },
      }),
  );
  const status = useSyncExternalStore(autosave.subscribe, autosave.snapshot);
  const refusalHandler = useRef<(refusal: Refusal, sent: readonly SentAnswer[]) => void>(() => undefined);
  useEffect(() => {
    refusalHandler.current = handleRefusal;
  });

  function revertFiles(sent: readonly SentAnswer[]): void {
    const files = sent.filter((entry) => entry.block.type === 'file');
    setLocal((previous) => {
      let next = previous;
      for (const entry of files) {
        const current = next.get(entry.block.id);
        if (current !== undefined && sameAnswer(entry.block, current, entry.answer)) {
          next = withoutEntry(next, entry.block.id);
        }
      }
      return next;
    });
    autosave.forget(files.map((entry) => entry.block.id));
  }

  function flagFiles(sent: readonly SentAnswer[], reason: FileRefusalReason): void {
    const files = sent.filter((entry) => entry.block.type === 'file');
    setFileRefusals((previous) => {
      const next = new Map(previous);
      for (const entry of files) {
        next.set(entry.block.id, { reason, count: (previous.get(entry.block.id)?.count ?? 0) + 1 });
      }
      return next;
    });
  }

  function handleRefusal(refusal: Refusal, sent: readonly SentAnswer[]): void {
    switch (refusal.kind) {
      case 'changed':
        setLocal(new Map());
        setKept(new Map());
        setInvalid(new Map());
        onNotice('changed');
        void reload().then(
          (fresh) => {
            autosave.reloaded(fresh.version);
          },
          () => {
            autosave.reloaded(submissionValue.current.version);
            void queryClient.invalidateQueries({ queryKey: key });
          },
        );
        return;
      case 'gone':
        revertFiles(sent);
        void reload().then(
          () => {
            flagFiles(sent, 'unavailable');
          },
          () => {
            flagFiles(sent, 'unavailable');
          },
        );
        return;
      case 'invalid': {
        const problems = new Map<string, ApiError>();
        for (const question of refusal.questions) {
          problems.set(question, refusal.error);
        }
        setInvalid(problems);
        setPageRefusal(problems.size === 0 ? refusal.error : null);
        const first = questions.find((block) => problems.has(block.id));
        if (first !== undefined) {
          requestFocus({ kind: 'field', question: first.id });
        }
        return;
      }
      case 'storageFull':
        revertFiles(sent);
        flagFiles(sent, 'storageFull');
    }
  }

  useEffect(() => {
    autosave.adopt(submission.version);
  }, [autosave, submission.version]);

  useEffect(() => {
    autosave.resume();
    return (): void => {
      autosave.leave();
    };
  }, [autosave]);

  useEffect(() => {
    if (gone) {
      autosave.drop();
    }
  }, [autosave, gone]);

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

  function change(block: QuestionBlock, answer: Answer): void {
    setLocal((previous) => withEntry(previous, block.id, answer));
    setFileRefusals((previous) => {
      if (!previous.has(block.id)) {
        return previous;
      }
      const next = new Map(previous);
      next.delete(block.id);
      return next;
    });
    setInvalid((previous) => {
      if (!previous.has(block.id)) {
        return previous;
      }
      const next = new Map(previous);
      next.delete(block.id);
      return next;
    });
    displayedValue.current = withEntry(displayedValue.current, block.id, answer);
    autosave.changed(block.id);
  }

  async function loadLatest(): Promise<void> {
    setLoading(true);
    setLoadFailure(null);
    try {
      const fresh = await reload();
      const differing = new Map<string, Answer>();
      for (const block of fresh.blocks.filter(isQuestion)) {
        const mine = local.get(block.id);
        if (mine !== undefined && !sameAnswer(block, mine, fresh.answers.get(block.id) ?? emptyAnswer)) {
          differing.set(block.id, mine);
        }
      }
      setLocal(new Map());
      setKept(differing);
      autosave.reloaded(fresh.version);
      requestFocus({ kind: 'useOrHeading' });
    } catch (thrown: unknown) {
      setLoadFailure(asApiError(thrown));
    } finally {
      setLoading(false);
    }
  }

  async function openSubmit(): Promise<void> {
    if (preparing) {
      return;
    }
    setPreparing(true);
    const clean = await autosave.flush();
    setPreparing(false);
    if (clean) {
      setSubmitFailure(null);
      dialogReturn.current = submitButton.current;
      setSubmitOpen(true);
    }
  }

  async function confirmSubmit(): Promise<void> {
    setSubmitBusy(true);
    setSubmitFailure(null);
    const outcome = await submitWork(client, submission.id, autosave.currentVersion);
    setSubmitBusy(false);
    switch (outcome.kind) {
      case 'submitted':
        queryClient.setQueryData(key, outcome.submission);
        setLocal(new Map());
        setKept(new Map());
        autosave.reloaded(outcome.submission.version);
        setSubmitOpen(false);
        onNotice('submitted');
        void queryClient.invalidateQueries({ queryKey: assignmentsKey });
        return;
      case 'conflict':
        setSubmitOpen(false);
        autosave.conflicted();
        return;
      case 'changed':
        setSubmitOpen(false);
        handleRefusal({ kind: 'changed' }, []);
        return;
      case 'gone':
        setSubmitOpen(false);
        void queryClient.invalidateQueries({ queryKey: key });
        return;
      case 'failed':
        setSubmitFailure(outcome.error);
        setSubmitRetryAfter(outcome.retryAfterSeconds);
    }
  }

  const writable = ['not_started', 'draft', 'returned'].includes(submission.status);
  const total = questions.length;
  const answeredCount = questions.filter((block) => answered(block, displayed.get(block.id))).length;
  const unanswered = questions
    .filter((block) => !answered(block, displayed.get(block.id)))
    .map((block) => numbers.get(block.id) ?? 0);
  const deadlinePassed =
    (submission.status === 'not_started' || submission.status === 'draft') &&
    submission.deadlineAt !== null &&
    Date.parse(submission.deadlineAt) <= now;

  function below(block: QuestionBlock): ReactNode {
    const fileRefusal = fileRefusals.get(block.id);
    const problem = invalid.get(block.id);
    const mine = kept.get(block.id);
    const revealed = submission.revealed?.get(block.id);
    const reviewItem = submission.review?.items.get(block.id);
    return (
      <>
        {revealed === undefined ? null : <RevealedPart block={block} revealed={revealed} />}
        {reviewItem === undefined ? null : (
          <QuestionReview
            item={reviewItem}
            highlighted={submission.status === 'returned'}
            origin={window.location.origin}
          />
        )}
        {fileRefusal === undefined ? null : (
          <FormAlert key={fileRefusal.count}>{t(`assignment.files.${fileRefusal.reason}`)}</FormAlert>
        )}
        {problem === undefined ? null : (
          <p className="text-small text-danger">{t(`fieldErrors.${problem.fields[0]?.code ?? 'invalid'}`)}</p>
        )}
        {mine === undefined ? null : (
          <div className="flex flex-col items-start gap-1 rounded-md bg-hover p-3">
            <p dir="auto" className="break-words">
              {t('assignment.conflict.kept', { answer: answerText(block, mine) })}
            </p>
            <button
              type="button"
              data-use-answer=""
              onClick={() => {
                setKept((previous) => withoutEntry(previous, block.id));
                change(block, mine);
                requestFocus({ kind: 'useOrField', question: block.id });
              }}
              className={textButtonClass}
            >
              {t('assignment.conflict.use')}
            </button>
          </div>
        )}
      </>
    );
  }

  const answering: Answering = {
    answers: displayed,
    editable: writable ? submission.editable : new Set(),
    invalid: new Set(invalid.keys()),
    onChange: change,
    below,
    correctOptions: (block) => new Set(submission.revealed?.get(block.id)?.correctOptionIds ?? []),
    fileControl: (block, answer) => (
      <AnswerFiles
        submissionId={submission.id}
        block={block}
        files={answer.files}
        editable={writable && submission.editable.has(block.id)}
        confirmed={me.email_verified}
        onChange={(files) => {
          change(block, { ...answer, files });
        }}
        onUploading={(busy) => {
          uploadingChanged(block.id, busy);
        }}
      />
    ),
  };

  if (gone) {
    return <></>;
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <TutorName name={submission.tutor.name} colour={submission.tutor.colour} />
        <span className="text-small">
          <DeadlineHint deadline={submission.deadlineAt} now={now} />
        </span>
        <StateLabel status={submission.status} deadline={submission.deadlineAt} late={submission.late} now={now} />
      </div>
      {submission.issuedBeforeReclaim ? <ReclaimCue /> : null}
      {notice === null ? null : (
        <PageNotice
          key={notice.count}
          message={notice.kind === 'changed' ? t('assignment.changed') : t('assignment.submit.done')}
          onShown={(element) => {
            dialogReturn.current = element;
          }}
        />
      )}
      {submission.status === 'closed' ? <p>{t('assignment.closed')}</p> : null}
      {status.kind === 'conflict' ? (
        <ConflictBanner
          busy={loading}
          failure={loadFailure}
          onLoad={() => {
            void loadLatest();
          }}
          onShown={(element) => {
            dialogReturn.current = element;
          }}
        />
      ) : null}
      {submission.status === 'returned' ? (
        <p className="rounded-md border border-warning p-4 text-strong">{t('assignment.returnedBanner')}</p>
      ) : null}
      {submission.review === null || submission.review.feedback === '' ? null : (
        <OverallFeedback feedback={submission.review.feedback} />
      )}
      {writable ? (
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-small">{t('assignment.progress', { answered: answeredCount, total })}</p>
          <SaveStatus
            status={status}
            onRetry={() => {
              autosave.retryNow();
            }}
          />
        </div>
      ) : null}
      {pageRefusal === null ? null : <FormAlert>{errorMessage(t, pageRefusal, null)}</FormAlert>}
      <StudentBlocks
        assignment={{ title: submission.title, blocks: submission.blocks }}
        origin={window.location.origin}
        answering={answering}
        showTitle={false}
        onAddressFailed={(address) => {
          if (!failedAddresses.current.has(address)) {
            failedAddresses.current.add(address);
            void queryClient.invalidateQueries({ queryKey: key });
          }
        }}
      />
      {writable ? (
        <div className="flex flex-col items-start gap-3 border-t border-border pt-4 max-tablet:items-stretch">
          {deadlinePassed ? <p>{t('assignment.deadlinePassed')}</p> : null}
          <button
            ref={submitButton}
            type="button"
            aria-disabled={preparing || status.kind === 'conflict' || uploading.size > 0}
            aria-busy={preparing || uploading.size > 0}
            onClick={() => {
              if (status.kind !== 'conflict' && uploading.size === 0) {
                void openSubmit();
              }
            }}
            className={primaryButtonClass}
          >
            {submission.status === 'returned' ? t('assignment.submit.again') : t('assignment.submit.action')}
          </button>
        </div>
      ) : null}
      <SubmitDialog
        open={submitOpen}
        busy={submitBusy}
        unanswered={unanswered}
        failure={submitFailure}
        retryAfter={submitRetryAfter}
        returnFocus={dialogReturn}
        onConfirm={() => {
          void confirmSubmit();
        }}
        onClose={() => {
          setSubmitOpen(false);
        }}
      />
    </>
  );
}

function AssignmentContent({ submissionId }: { readonly submissionId: string }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const [notice, setNotice] = useState<{ readonly kind: NoticeKind; readonly count: number } | null>(null);
  const query = useQuery({
    queryKey: submissionKey(submissionId),
    queryFn: ({ signal }) => fetchSubmission(client, submissionId, signal),
    retry: false,
  });
  const queryError = query.isError ? asApiError(query.error) : null;
  const gone = queryError?.status === 404 && query.data !== undefined;
  const notFound = queryError?.status === 404 && query.data === undefined;
  const refetchError = queryError !== null && queryError.status !== 404 && query.data !== undefined;
  const submission = gone || notFound ? undefined : query.data;
  useSeenMarker(submission);

  if (notFound) {
    return (
      <Page title="page.notFound" focusTitle>
        <NotFoundContent />
      </Page>
    );
  }

  const form =
    query.data === undefined ? null : (
      <AssignmentForm
        submission={query.data}
        gone={gone}
        notice={notice}
        onNotice={(kind) => {
          setNotice((previous) => (kind === null ? null : { kind, count: (previous?.count ?? 0) + 1 }));
        }}
      />
    );

  function body(): ReactElement {
    if (gone) {
      return (
        <>
          {form}
          <PageNotice message={t('assignment.gone')} whenFocusIsLost />
          <p>
            <Link to="/homework" className={linkClass}>
              {t('assignment.list.allHomework')}
            </Link>
          </p>
        </>
      );
    }
    if (submission === undefined) {
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
    return <>{form}</>;
  }

  return (
    <Page
      width="reading"
      {...(submission === undefined ? {} : { headingText: submission.title, headingDir: 'auto' as const })}
    >
      <p>
        <Link to="/homework" className={linkClass}>
          {t('assignment.list.back')}
        </Link>
      </p>
      {refetchError ? (
        <RefetchFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : null}
      {body()}
    </Page>
  );
}

export function AssignmentPage(): ReactElement {
  const { submissionId = '' } = useParams();
  const texts = useFeatureTexts('assignment', assignmentTextLoaders);
  const me = useSignedInAccount();
  if (!texts.ready) {
    return (
      <Page>
        {texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />}
      </Page>
    );
  }
  return <AssignmentContent key={`${me.id}:${submissionId}`} submissionId={submissionId} />;
}
