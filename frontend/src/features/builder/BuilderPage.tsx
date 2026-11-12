import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  type ReactElement,
  type RefObject,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { ActionMenu } from '../../components/ui/ActionMenu.tsx';
import {
  ActionButton,
  FormAlert,
  fieldClass,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { type ApiError, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { parseStudentView, type StudentAssignment } from '../assignment/studentView.ts';
import { assignmentTextLoaders } from '../assignment/texts.ts';
import { IssueDialogHost, IssuedNotice, homeworkTextLoaders } from '../homework/IssueAssignment.tsx';
import { fetchFolders, folderTree, libraryFoldersKey, libraryListsKey } from '../library/api.ts';
import { folderAddress } from '../library/LibraryTable.tsx';
import { LibraryUpload } from '../library/upload.ts';
import { fetchStudentView, fetchTemplate, saveTemplate, templateKey } from './api.ts';
import { type AutosaveStatus, type SaveOutcome, type Template, TemplateAutosave } from './autosave.ts';
import { BlockCard, type BlockActions } from './BlockCard.tsx';
import {
  type Block,
  type NewBlockType,
  blockInput,
  blockTypes,
  cutToCodePoints,
  duplicateBlock,
  isQuestion,
  limits,
  moveBlock,
  newBlock,
  newMaterialBlock,
  parseBlock,
  placeBlock,
} from './blocks.ts';
import {
  type DisplayEntries,
  type PictureUpload,
  type ProblemCode,
  displayEntries,
  earliestExpiry,
  noDisplayEntries,
  problemsByBlock,
} from './display.ts';
import { focusId } from './fields.tsx';
import { type ChosenMaterial, MaterialPicker } from './MaterialPicker.tsx';
import { PreviewDialog } from './PreviewDialog.tsx';
import { builderTextLoaders } from './texts.ts';

function IssueButton({
  busy,
  buttonRef,
  titleId,
  onIssue,
}: {
  readonly busy: boolean;
  readonly buttonRef: RefObject<HTMLButtonElement | null>;
  readonly titleId: string;
  readonly onIssue: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const heldFocus = useRef(false);

  useLayoutEffect(() => {
    const element = buttonRef.current;
    return (): void => {
      heldFocus.current = element !== null && element === document.activeElement;
    };
  }, [buttonRef]);

  useEffect(
    () => (): void => {
      if (heldFocus.current) {
        document.getElementById(titleId)?.focus();
      }
    },
    [titleId],
  );

  return (
    <ActionButton busy={busy} onClick={onIssue} className={quietButtonClass} ref={buttonRef}>
      {t('homework.issue')}
    </ActionButton>
  );
}

const expiryMarginMilliseconds = 30_000;
const minimumRefetchMilliseconds = 5_000;

type PickerTarget =
  { readonly kind: 'insert'; readonly at: number } | { readonly kind: 'replace'; readonly blockId: string };

interface Summary {
  readonly problems: ReadonlyMap<string | null, readonly ProblemCode[]>;
  readonly count: number;
  readonly ready: boolean;
}

function summaryOf(template: Template): Summary {
  return { problems: problemsByBlock(template), count: template.problems.length, ready: template.ready };
}

function useFocusRequests(): { readonly request: (id: string) => void } {
  const pending = useRef<string | null>(null);
  const [, setRequested] = useState(0);
  useEffect(() => {
    const target = pending.current;
    if (target === null) {
      return;
    }
    const element = document.getElementById(target);
    if (element !== null) {
      pending.current = null;
      element.focus();
    }
  });
  return {
    request: (id: string): void => {
      pending.current = id;
      setRequested((value) => value + 1);
    },
  };
}

function statusText(t: TFunction, status: AutosaveStatus): string | null {
  switch (status.kind) {
    case 'saving':
      return t('builder.saving');
    case 'saved':
      return t('builder.saved');
    case 'failed':
      return t('builder.notSaved');
    case 'editing':
    case 'blankTitle':
    case 'conflict':
      return null;
  }
}

function failureText(t: TFunction, status: AutosaveStatus): string | null {
  if (status.kind !== 'failed') {
    return null;
  }
  switch (status.reason) {
    case 'tooLarge':
      return t('builder.tooLarge');
    case 'storageLimit':
      return t('builder.storageLimit');
    default:
      return errorText(t, status.error);
  }
}

function errorText(t: TFunction, error: ApiError): string {
  return refusalText(t, error, null, undefined);
}

function Breadcrumbs({ folderId }: { readonly folderId: string | null }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const folders = useQuery({
    queryKey: libraryFoldersKey,
    queryFn: ({ signal }) => fetchFolders(client, signal),
    retry: false,
  });
  const node = folderTree(folders.data ?? []).find((entry) => entry.folder.id === folderId);
  return (
    <nav aria-label={t('builder.path')}>
      <ol className="flex flex-wrap items-center gap-1 text-small">
        <li>
          <Link to={folderAddress(null)} className={linkClass}>
            {t('builder.root')}
          </Link>
        </li>
        {(node?.path ?? []).map((folder) => (
          <li key={folder.id} className="flex items-center gap-1">
            <span aria-hidden="true">/</span>
            <Link to={folderAddress(folder.id)} dir="auto" className={`${linkClass} break-words`}>
              {folder.name}
            </Link>
          </li>
        ))}
      </ol>
    </nav>
  );
}

function ConflictNotice({ onReload }: { readonly onReload: () => Promise<boolean> }): ReactElement {
  const { t } = useTranslation();
  const notice = useRef<HTMLParagraphElement>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(0);
  useEffect(() => {
    notice.current?.focus();
  }, []);
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-warning p-4">
      <div className="flex flex-wrap items-center gap-3">
        <p ref={notice} role="status" tabIndex={-1} className="text-strong outline-none">
          {t('builder.conflict')}
        </p>
        <ActionButton
          busy={busy}
          onClick={() => {
            setBusy(true);
            void onReload().then((reloaded) => {
              setBusy(false);
              if (!reloaded) {
                setFailed((count) => count + 1);
              }
            });
          }}
          className={primaryButtonClass}
        >
          {t('builder.reload')}
        </ActionButton>
      </div>
      {failed === 0 ? null : <FormAlert key={failed}>{t('errors.internal')}</FormAlert>}
    </div>
  );
}

function Editor({
  initial,
  latest,
  focusTitle,
  onReload,
}: {
  readonly initial: Template;
  readonly latest: Template;
  readonly focusTitle: boolean;
  readonly onReload: () => Promise<boolean>;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const session = useSession();
  const me = useSignedInAccount();
  const scheduler = useScheduler();
  const queryClient = useQueryClient();
  const prefix = useId();
  const titleId = `${prefix}-title`;
  const titleHintId = `${prefix}-title-hint`;
  const { request: focus } = useFocusRequests();
  const [title, setTitle] = useState(initial.title);
  const [blocks, setBlocks] = useState<readonly Block[]>(() => initial.blocks.map(parseBlock));
  const [picked, setPicked] = useState<DisplayEntries>(noDisplayEntries);
  const display = useMemo(() => displayEntries(latest, picked), [latest, picked]);
  const summary = useMemo(() => summaryOf(latest), [latest]);
  const [uploads, setUploads] = useState<readonly PictureUpload[]>([]);
  const [picker, setPicker] = useState<PickerTarget | null>(null);
  const [preview, setPreview] = useState<StudentAssignment | null>(null);
  const [previewBusy, setPreviewBusy] = useState(false);
  const [previewAlert, setPreviewAlert] = useState<string | null>(null);
  const pickerOpener = useRef<HTMLElement | null>(null);
  const previewButton = useRef<HTMLButtonElement>(null);
  const issueButton = useRef<HTMLButtonElement>(null);
  const homeworkTexts = useFeatureTexts('homework', homeworkTextLoaders);
  const [issueBusy, setIssueBusy] = useState(false);
  const [issuing, setIssuing] = useState<{ readonly id: string; readonly title: string } | null>(null);
  const [issued, setIssued] = useState<{ readonly issueId: string; readonly count: number } | null>(null);
  const addBlockButton = useRef<HTMLButtonElement>(null);
  const lastFocusedBlock = useRef<string | null>(null);
  const dragged = useRef<string | null>(null);
  const failedAddresses = useRef(new Set<string>());
  const titleValue = useRef(title);
  const blocksValue = useRef(blocks);
  const [autosave] = useState(
    () =>
      new TemplateAutosave({
        scheduler,
        version: initial.version,
        newSaveId: (): string => crypto.randomUUID(),
        send: (request): Promise<SaveOutcome> => saveTemplate(client, initial.id, request),
        onSaved: (template): void => {
          queryClient.setQueryData(templateKey(initial.id), template);
          void queryClient.invalidateQueries({ queryKey: libraryListsKey });
        },
      }),
  );
  const status = useSyncExternalStore(autosave.subscribe, autosave.snapshot);

  useEffect(
    () => (): void => {
      autosave.leave();
    },
    [autosave],
  );

  const uploadsValue = useRef(uploads);
  useEffect(() => {
    uploadsValue.current = uploads;
  }, [uploads]);
  useEffect(
    () => (): void => {
      for (const entry of uploadsValue.current) {
        entry.upload.cancel();
      }
    },
    [],
  );

  useEffect(() => {
    if (focusTitle) {
      document.getElementById(titleId)?.focus();
    }
  }, [focusTitle, titleId]);

  useEffect(() => {
    const expiry = earliestExpiry(display);
    if (expiry === null) {
      return undefined;
    }
    const wait = Math.max(minimumRefetchMilliseconds, expiry - expiryMarginMilliseconds - scheduler.wallClock());
    return scheduler.schedule(wait, () => {
      void queryClient.invalidateQueries({ queryKey: templateKey(initial.id) });
    });
  }, [display, scheduler, queryClient, initial.id]);

  function dropUploads(matches: (entry: PictureUpload) => boolean): void {
    for (const entry of uploadsValue.current.filter(matches)) {
      entry.upload.cancel();
    }
    setUploads((entries) => entries.filter((entry) => !matches(entry)));
  }

  function addressFailed(url: string): void {
    if (failedAddresses.current.has(url)) {
      return;
    }
    failedAddresses.current.add(url);
    void queryClient.invalidateQueries({ queryKey: templateKey(initial.id) });
  }

  function commit(nextTitle: string, nextBlocks: readonly Block[]): void {
    titleValue.current = nextTitle;
    blocksValue.current = nextBlocks;
    setTitle(nextTitle);
    setBlocks(nextBlocks);
    autosave.changed({ title: nextTitle, blocks: nextBlocks.map(blockInput) });
  }

  function changeBlocks(change: (current: readonly Block[]) => readonly Block[]): void {
    commit(titleValue.current, change(blocksValue.current));
  }

  function insertAt(index: number, block: Block): void {
    changeBlocks((current) => [...current.slice(0, index), block, ...current.slice(index)]);
    focus(focusId(prefix, block.id, 'first'));
  }

  function insertionIndex(): number {
    const current = blocksValue.current;
    const focusedIndex = current.findIndex((block) => block.id === lastFocusedBlock.current);
    return focusedIndex === -1 ? current.length : focusedIndex + 1;
  }

  function addBlock(type: NewBlockType | 'material'): void {
    if (type === 'material') {
      pickerOpener.current = null;
      setPicker({ kind: 'insert', at: insertionIndex() });
      return;
    }
    insertAt(
      insertionIndex(),
      newBlock(type, () => crypto.randomUUID()),
    );
  }

  function chooseMaterial(material: ChosenMaterial): void {
    const target = picker;
    setPicker(null);
    if (target === null) {
      return;
    }
    setPicked((previous) => ({
      ...previous,
      materials: new Map(previous.materials).set(material.materialId, {
        attachmentId: material.attachmentId,
        title: material.title,
        filename: material.filename,
        deleted: false,
        url: null,
        expiresAt: null,
      }),
    }));
    if (target.kind === 'insert') {
      insertAt(
        target.at,
        newMaterialBlock(material.materialId, () => crypto.randomUUID()),
      );
      return;
    }
    changeBlocks((current) =>
      current.map((block) =>
        block.id === target.blockId && block.type === 'material'
          ? { ...block, materialId: material.materialId }
          : block,
      ),
    );
  }

  function closePicker(): void {
    const target = picker;
    setPicker(null);
    if (target?.kind === 'insert') {
      focus(`${prefix}-add-block`);
    }
  }

  function addPicture(blockId: string, file: File): void {
    const upload = new LibraryUpload(client, session, file, 'prompt_image');
    const entry: PictureUpload = { key: crypto.randomUUID(), blockId, upload };
    setUploads((current) => [...current, entry]);
    const unsubscribe = upload.subscribe(() => {
      const state = upload.snapshot();
      if (state.kind !== 'uploaded') {
        return;
      }
      unsubscribe();
      setUploads((current) => current.filter((candidate) => candidate.key !== entry.key));
      setPicked((previous) => ({
        ...previous,
        images: new Map(previous.images).set(state.file.id, {
          filename: state.file.filename,
          width: null,
          height: null,
          url: null,
          expiresAt: null,
        }),
      }));
      changeBlocks((current) =>
        current.map((block) =>
          block.id === blockId && isQuestion(block) && block.imageIds.length < limits.images
            ? { ...block, imageIds: [...block.imageIds, state.file.id] }
            : block,
        ),
      );
    });
  }

  function blockActions(block: Block, index: number): BlockActions {
    return {
      change: (next): void => {
        changeBlocks((current) => current.map((entry) => (entry.id === block.id ? next : entry)));
      },
      move: (offset, button): void => {
        changeBlocks((current) =>
          moveBlock(
            current,
            current.findIndex((entry) => entry.id === block.id),
            offset,
          ),
        );
        focus(focusId(prefix, block.id, button));
      },
      duplicate: (): void => {
        const copy = duplicateBlock(block, () => crypto.randomUUID());
        changeBlocks((current) => {
          const position = current.findIndex((entry) => entry.id === block.id);
          return [...current.slice(0, position + 1), copy, ...current.slice(position + 1)];
        });
        focus(focusId(prefix, copy.id, 'first'));
      },
      remove: (): void => {
        const current = blocksValue.current;
        const position = current.findIndex((entry) => entry.id === block.id);
        const neighbour = current[position + 1] ?? current[position - 1];
        changeBlocks((entries) => entries.filter((entry) => entry.id !== block.id));
        dropUploads((entry) => entry.blockId === block.id);
        focus(neighbour === undefined ? `${prefix}-add-block` : focusId(prefix, neighbour.id, 'heading'));
      },
      chooseMaterial: (opener): void => {
        pickerOpener.current = opener;
        setPicker({ kind: 'replace', blockId: block.id });
      },
      addPicture: (file): void => {
        addPicture(block.id, file);
      },
      removePicture: (attachmentId): void => {
        changeBlocks((current) =>
          current.map((entry) =>
            entry.id === block.id && isQuestion(entry)
              ? { ...entry, imageIds: entry.imageIds.filter((id) => id !== attachmentId) }
              : entry,
          ),
        );
      },
      dismissUpload: (key): void => {
        dropUploads((entry) => entry.key === key);
        focus(focusId(prefix, block.id, 'add-picture'));
      },
      focus,
      focused: (): void => {
        lastFocusedBlock.current = block.id;
      },
      dragStart: (): void => {
        dragged.current = block.id;
      },
      drop: (): void => {
        const from = dragged.current;
        dragged.current = null;
        if (from === null || from === block.id) {
          return;
        }
        changeBlocks((current) =>
          placeBlock(
            current,
            current.findIndex((entry) => entry.id === from),
            index,
          ),
        );
      },
    };
  }

  async function openPreview(): Promise<void> {
    setPreviewAlert(null);
    setPreviewBusy(true);
    try {
      const clean = await autosave.flush();
      if (!clean) {
        return;
      }
      setPreview(parseStudentView(await fetchStudentView(client, initial.id)));
    } catch (error: unknown) {
      setPreviewAlert(errorText(t, asApiError(error)));
    } finally {
      setPreviewBusy(false);
    }
  }

  async function openIssue(): Promise<void> {
    issueButton.current?.focus();
    setIssued(null);
    setIssueBusy(true);
    try {
      const clean = await autosave.flush();
      const saved = queryClient.getQueryData<Template>(templateKey(initial.id));
      if (clean && saved?.ready === true) {
        setIssuing({ id: saved.id, title: saved.title });
      }
    } finally {
      setIssueBusy(false);
    }
  }

  const issueShown = homeworkTexts.ready && me.email_verified && summary.ready;
  const blankTitle = title.trim() === '';
  const shownStatus = statusText(t, status);
  const failure = failureText(t, status);
  const templateProblems = summary.problems.get(null) ?? [];

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3 border-b border-border pb-4">
        <Breadcrumbs folderId={latest.folder_id} />
        <div className="flex flex-col gap-1">
          <label htmlFor={titleId} className="text-strong">
            {t('builder.title')}
          </label>
          <input
            id={titleId}
            type="text"
            value={title}
            dir="auto"
            aria-invalid={blankTitle}
            aria-describedby={blankTitle ? titleHintId : undefined}
            onChange={(event) => {
              commit(cutToCodePoints(event.target.value, limits.title), blocksValue.current);
            }}
            className={`${fieldClass} font-serif text-title`}
          />
          {blankTitle ? (
            <p id={titleHintId} className="text-small text-danger">
              {t('builder.blankTitle')}
            </p>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <LiveNotice message={shownStatus} />
          {status.kind === 'failed' ? (
            <ActionButton
              busy={false}
              onClick={() => {
                autosave.retry();
              }}
              className={textButtonClass}
            >
              {t('builder.retry')}
            </ActionButton>
          ) : null}
          <p className={summary.ready ? 'text-success' : 'text-warning'}>
            {summary.ready ? t('builder.readyToIssue') : t('builder.problems', { count: summary.count })}
          </p>
          <ActionButton
            busy={previewBusy}
            onClick={() => {
              void openPreview();
            }}
            className={quietButtonClass}
            ref={previewButton}
          >
            {t('builder.preview')}
          </ActionButton>
          {issueShown ? (
            <IssueButton
              busy={issueBusy}
              buttonRef={issueButton}
              titleId={titleId}
              onIssue={() => {
                void openIssue();
              }}
            />
          ) : null}
        </div>
        {issued === null ? null : <IssuedNotice key={issued.count} issueId={issued.issueId} />}
        {failure === null ? null : <FormAlert key={`${status.kind}:${failure}`}>{failure}</FormAlert>}
        {previewAlert === null ? null : <FormAlert>{previewAlert}</FormAlert>}
        {status.kind === 'conflict' ? <ConflictNotice onReload={onReload} /> : null}
      </div>
      <section aria-label={t('builder.blocks')} className="flex flex-col gap-2">
        {templateProblems.length === 0 ? null : (
          <p className="text-small text-warning">
            {templateProblems.map((code) => t(`builder.problemCodes.${code}`)).join(' ')}
          </p>
        )}
        <div className="flex flex-col divide-y divide-border">
          {blocks.map((block, index) => (
            <BlockCard
              key={block.id}
              block={block}
              context={{
                prefix,
                origin: window.location.origin,
                index,
                count: blocks.length,
                problems: summary.problems.get(block.id) ?? [],
                materials: display.materials,
                images: display.images,
                uploads: uploads.filter((entry) => entry.blockId === block.id),
                canAddPictures: me.email_verified,
                full: blocks.length >= limits.blocks,
                addressFailed,
              }}
              actions={blockActions(block, index)}
            />
          ))}
        </div>
        <div>
          <ActionMenu
            label={t('builder.addBlock')}
            unavailable={blocks.length >= limits.blocks}
            buttonRef={addBlockButton}
            buttonId={`${prefix}-add-block`}
            align="start"
            items={blockTypes.map((type) => ({
              label: t(`builder.types.${type}`),
              onChoose: (): void => {
                addBlock(type);
              },
            }))}
          >
            {t('builder.addBlock')}
          </ActionMenu>
        </div>
      </section>
      <MaterialPicker
        open={picker !== null}
        returnFocus={pickerOpener}
        onChoose={chooseMaterial}
        onClose={closePicker}
      />
      <PreviewDialog
        assignment={preview}
        returnFocus={previewButton}
        onClose={() => {
          setPreview(null);
        }}
      />
      {homeworkTexts.ready ? (
        <IssueDialogHost
          open={issuing !== null}
          preset={issuing === null ? {} : { template: issuing }}
          returnFocus={issueButton}
          onClose={() => {
            setIssuing(null);
          }}
          onIssued={(issue) => {
            setIssuing(null);
            setIssued((previous) => ({ issueId: issue.id, count: (previous?.count ?? 0) + 1 }));
          }}
        />
      ) : null}
    </div>
  );
}

function BuilderContent({ templateId }: { readonly templateId: string }): ReactElement {
  const client = useApiClient();
  const builderTexts = useFeatureTexts('builder', builderTextLoaders);
  const assignmentTexts = useFeatureTexts('assignment', assignmentTextLoaders);
  const [reloaded, setReloaded] = useState<{ readonly epoch: number; readonly template: Template } | null>(null);
  const template = useQuery({
    queryKey: templateKey(templateId),
    queryFn: ({ signal }) => fetchTemplate(client, templateId, signal),
    retry: false,
  });

  const loaded = reloaded ?? (template.data === undefined ? null : { epoch: 0, template: template.data });

  const textsFailed = builderTexts.failed || assignmentTexts.failed;
  const missing = template.isError && asApiError(template.error).status === 404;

  if (missing) {
    return (
      <Page title="page.notFound" focusTitle>
        <NotFoundContent />
      </Page>
    );
  }

  return (
    <Page>
      {textsFailed ? (
        <LoadFailed
          error={asApiError(undefined)}
          onRetry={() => {
            builderTexts.retry();
            assignmentTexts.retry();
          }}
        />
      ) : null}
      {template.isError && template.data === undefined ? (
        <LoadFailed
          error={asApiError(template.error)}
          onRetry={() => {
            void template.refetch();
          }}
        />
      ) : null}
      {!builderTexts.ready || !assignmentTexts.ready || loaded === null || template.data === undefined ? (
        textsFailed || template.isError ? null : (
          <DelayedLoading />
        )
      ) : (
        <Editor
          key={loaded.epoch}
          initial={loaded.template}
          latest={template.data}
          focusTitle={loaded.epoch > 0}
          onReload={async (): Promise<boolean> => {
            const result = await template.refetch();
            if (result.isError || result.data === undefined) {
              return false;
            }
            setReloaded({ epoch: loaded.epoch + 1, template: result.data });
            return true;
          }}
        />
      )}
    </Page>
  );
}

export function BuilderPage(): ReactElement {
  const { templateId } = useParams();
  const me = useSignedInAccount();
  if (templateId === undefined) {
    throw new Error('The builder route rendered without a template identifier');
  }
  return <BuilderContent key={`${me.id}:${templateId}`} templateId={templateId} />;
}
