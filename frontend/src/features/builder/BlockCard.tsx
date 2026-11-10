import { type ReactElement, useEffect, useId, useRef, useState, useSyncExternalStore } from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';

import { ActionButton, fieldClass, linkClass, quietButtonClass, textButtonClass } from '../../components/ui/Form.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { downloadAddress } from '../../lib/files/downloadAddress.ts';
import {
  type Block,
  type ChoiceBlock,
  type ChoiceOption,
  type FileKind,
  type QuestionBlock,
  cutToCodePoints,
  fileKinds,
  isChoice,
  isQuestion,
  limits,
} from './blocks.ts';
import type { ImageInfo, MaterialInfo, PictureUpload, ProblemCode } from './display.ts';
import { Group, IntegerField, TextArea, TextInput, Warnings, focusId } from './fields.tsx';

export interface BlockActions {
  readonly change: (block: Block) => void;
  readonly move: (offset: -1 | 1, button: string) => void;
  readonly duplicate: () => void;
  readonly remove: () => void;
  readonly chooseMaterial: (opener: HTMLElement) => void;
  readonly addPicture: (file: File) => void;
  readonly removePicture: (attachmentId: string) => void;
  readonly dismissUpload: (key: string) => void;
  readonly focus: (id: string) => void;
  readonly focused: () => void;
  readonly dragStart: () => void;
  readonly drop: () => void;
}

export interface BlockContext {
  readonly prefix: string;
  readonly origin: string;
  readonly index: number;
  readonly count: number;
  readonly problems: readonly ProblemCode[];
  readonly materials: ReadonlyMap<string, MaterialInfo>;
  readonly images: ReadonlyMap<string, ImageInfo>;
  readonly uploads: readonly PictureUpload[];
  readonly canAddPictures: boolean;
  readonly full: boolean;
  readonly addressFailed: (url: string) => void;
}

const optionProblems: readonly ProblemCode[] = [
  'too_few_options',
  'option_empty',
  'correct_count',
  'no_correct_option',
];
const answerProblems: readonly ProblemCode[] = ['no_accepted_answer', 'accepted_answer_empty'];

function useProblemText(problems: readonly ProblemCode[]): (codes: readonly ProblemCode[]) => string[] {
  const { t } = useTranslation();
  return (codes) => problems.filter((code) => codes.includes(code)).map((code) => t(`builder.problemCodes.${code}`));
}

function OptionsEditor({
  block,
  context,
  actions,
}: {
  readonly block: ChoiceBlock;
  readonly context: BlockContext;
  readonly actions: BlockActions;
}): ReactElement {
  const { t } = useTranslation();
  const warnings = useProblemText(context.problems)(optionProblems);
  const warningId = useId();
  const id = (part: string): string => focusId(context.prefix, block.id, part);

  function withOptions(options: readonly ChoiceOption[]): void {
    actions.change({ ...block, options });
  }

  function mark(index: number, checked: boolean): void {
    withOptions(
      block.options.map((option, position) =>
        block.type === 'single_choice'
          ? { ...option, correct: position === index }
          : position === index
            ? { ...option, correct: checked }
            : option,
      ),
    );
  }

  function removeOption(index: number): void {
    const next = block.options[index + 1] ?? block.options[index - 1];
    withOptions(block.options.filter((_, position) => position !== index));
    actions.focus(next === undefined ? id('add-option') : id(`option-${next.id}`));
  }

  function addOption(): void {
    const option = { id: crypto.randomUUID(), text: '', correct: false };
    withOptions([...block.options, option]);
    actions.focus(id(`option-${option.id}`));
  }

  const full = block.options.length >= limits.options;
  return (
    <Group label={t('builder.options')}>
      <ol className="flex flex-col gap-2" aria-describedby={warnings.length === 0 ? undefined : warningId}>
        {block.options.map((option, index) => {
          const number = index + 1;
          return (
            <li key={option.id} className="flex items-center gap-2">
              <input
                type={block.type === 'single_choice' ? 'radio' : 'checkbox'}
                name={id('correct')}
                checked={option.correct}
                aria-label={t('builder.correct', { number })}
                onChange={(event) => {
                  mark(index, event.target.checked);
                }}
                className="size-5 shrink-0"
              />
              <input
                id={id(`option-${option.id}`)}
                type="text"
                value={option.text}
                dir="auto"
                aria-label={t('builder.option', { number })}
                onChange={(event) => {
                  withOptions(
                    block.options.map((entry, position) =>
                      position === index
                        ? { ...entry, text: cutToCodePoints(event.target.value, limits.option) }
                        : entry,
                    ),
                  );
                }}
                className={fieldClass}
              />
              <ActionButton
                busy={false}
                onClick={() => {
                  removeOption(index);
                }}
                className={textButtonClass}
              >
                {t('builder.removeOption', { number })}
              </ActionButton>
            </li>
          );
        })}
      </ol>
      <Warnings id={warningId} messages={warnings} />
      <div>
        <ActionButton
          busy={false}
          unavailable={full}
          unavailableReason={full ? t('builder.optionLimit') : undefined}
          onClick={addOption}
          className={quietButtonClass}
          id={id('add-option')}
        >
          {t('builder.addOption')}
        </ActionButton>
      </div>
      {block.type === 'multiple_choice' ? (
        <fieldset className="flex flex-col gap-1">
          <legend className="mb-1 text-strong">{t('builder.scoring')}</legend>
          {(['all_or_nothing', 'partial'] as const).map((scoring) => (
            <label key={scoring} className="flex min-h-11 items-center gap-2">
              <input
                type="radio"
                name={id('scoring')}
                checked={block.scoring === scoring}
                onChange={() => {
                  actions.change({ ...block, scoring });
                }}
                className="size-5"
              />
              {t(`builder.${scoring}`)}
            </label>
          ))}
        </fieldset>
      ) : null}
    </Group>
  );
}

function AnswersEditor({
  block,
  context,
  actions,
}: {
  readonly block: Extract<Block, { readonly type: 'short_text' }>;
  readonly context: BlockContext;
  readonly actions: BlockActions;
}): ReactElement {
  const { t } = useTranslation();
  const [typed, setTyped] = useState('');
  const warnings = useProblemText(context.problems)(answerProblems);
  const warningId = useId();
  const inputId = focusId(context.prefix, block.id, 'answer-input');
  const full = block.acceptedAnswers.length >= limits.acceptedAnswers;

  function add(): void {
    if (typed.trim() === '' || full) {
      return;
    }
    actions.change({ ...block, acceptedAnswers: [...block.acceptedAnswers, typed] });
    setTyped('');
  }

  return (
    <Group label={t('builder.acceptedAnswers')}>
      {block.acceptedAnswers.length === 0 ? null : (
        <ul className="flex flex-wrap gap-2">
          {block.acceptedAnswers.map((answer, index) => (
            <li
              key={`${String(index)}:${answer}`}
              className="inline-flex items-center gap-1 rounded-md border border-border-strong ps-3"
            >
              <span dir="auto" className="break-words">
                {answer}
              </span>
              <ActionButton
                busy={false}
                onClick={() => {
                  actions.change({
                    ...block,
                    acceptedAnswers: block.acceptedAnswers.filter((_, position) => position !== index),
                  });
                  actions.focus(inputId);
                }}
                className={textButtonClass}
              >
                {t('builder.removeAnswer', { answer })}
              </ActionButton>
            </li>
          ))}
        </ul>
      )}
      <div className="flex flex-wrap items-end gap-2">
        <div className="flex min-w-0 grow flex-col gap-1">
          <label htmlFor={inputId}>{t('builder.newAnswer')}</label>
          <input
            id={inputId}
            type="text"
            value={typed}
            dir="auto"
            aria-describedby={warnings.length === 0 ? undefined : warningId}
            onChange={(event) => {
              setTyped(cutToCodePoints(event.target.value, limits.acceptedAnswer));
            }}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault();
                add();
              }
            }}
            className={fieldClass}
          />
        </div>
        <ActionButton
          busy={false}
          unavailable={full}
          unavailableReason={full ? t('builder.answerLimit') : undefined}
          onClick={add}
          className={quietButtonClass}
        >
          {t('builder.addAnswer')}
        </ActionButton>
      </div>
      <Warnings id={warningId} messages={warnings} />
    </Group>
  );
}

function uploadRefusal(t: TFunction, upload: PictureUpload['upload']): string | null {
  const state = upload.snapshot();
  if (state.kind === 'tooLarge') {
    return t('builder.pictureTooLarge');
  }
  if (state.kind === 'wrongType') {
    return t('builder.pictureType');
  }
  if (state.kind !== 'refused') {
    return null;
  }
  switch (state.error.code) {
    case 'payload_too_large':
      return t('builder.pictureTooLarge');
    case 'unsupported_media_type':
      return t('builder.pictureType');
    case 'quota_exceeded':
      return t('builder.storageFull');
    case 'email_not_verified':
      return t('confirmationPrompt.upload');
    default:
      return refusalText(t, state.error, state.retryAfter, undefined);
  }
}

function FocusedAlert({ message }: { readonly message: string }): ReactElement {
  const element = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    element.current?.focus();
  }, []);
  return (
    <p ref={element} role="alert" tabIndex={-1} className="text-danger outline-none">
      {message}
    </p>
  );
}

function UploadRow({
  entry,
  actions,
}: {
  readonly entry: PictureUpload;
  readonly actions: BlockActions;
}): ReactElement {
  const { t } = useTranslation();
  const state = useSyncExternalStore(entry.upload.subscribe, entry.upload.snapshot);
  const refusal = uploadRefusal(t, entry.upload);
  if (refusal !== null) {
    return (
      <li className="flex flex-wrap items-center gap-2">
        <FocusedAlert message={refusal} />
        <ActionButton
          busy={false}
          onClick={() => {
            actions.dismissUpload(entry.key);
          }}
          className={textButtonClass}
        >
          {t('builder.close')}
        </ActionButton>
      </li>
    );
  }
  const name = entry.upload.file.name;
  return (
    <li className="flex flex-col gap-1">
      <p className="text-small break-words text-text-muted">
        <span dir="auto">{t('builder.pictureUploading', { filename: name })}</span>
      </p>
      <progress
        aria-label={t('builder.pictureUploading', { filename: name })}
        value={state.kind === 'uploading' ? state.loaded : 1}
        max={state.kind === 'uploading' ? (state.total ?? entry.upload.file.size) : 1}
        className="w-full"
      />
    </li>
  );
}

function PicturesEditor({
  block,
  context,
  actions,
}: {
  readonly block: QuestionBlock;
  readonly context: BlockContext;
  readonly actions: BlockActions;
}): ReactElement | null {
  const { t } = useTranslation();
  const fileInput = useRef<HTMLInputElement>(null);
  const addId = focusId(context.prefix, block.id, 'add-picture');
  if (block.imageIds.length === 0 && context.uploads.length === 0 && !context.canAddPictures) {
    return null;
  }
  const full = block.imageIds.length + context.uploads.length >= limits.images;

  function removePicture(index: number): void {
    const attachmentId = block.imageIds[index];
    if (attachmentId === undefined) {
      return;
    }
    const next = block.imageIds[index + 1];
    actions.removePicture(attachmentId);
    const fallback = context.canAddPictures ? addId : focusId(context.prefix, block.id, 'first');
    actions.focus(next === undefined ? fallback : focusId(context.prefix, block.id, `remove-picture-${next}`));
  }

  return (
    <Group label={t('builder.pictures')}>
      {block.imageIds.length === 0 ? null : (
        <ul className="flex flex-wrap gap-3">
          {block.imageIds.map((attachmentId, index) => {
            const image = context.images.get(attachmentId);
            const filename = image?.filename ?? '';
            const address =
              image?.url === null || image === undefined
                ? null
                : downloadAddress(attachmentId, image.url, context.origin);
            return (
              <li key={attachmentId} className="flex max-w-48 flex-col items-start gap-1">
                {address === null ? (
                  <p dir="auto" className="text-small break-words text-text-muted">
                    {t('builder.pictureSaving', { filename })}
                  </p>
                ) : (
                  <img
                    src={address}
                    alt={filename}
                    onError={() => {
                      context.addressFailed(address);
                    }}
                    referrerPolicy="no-referrer"
                    className="h-auto max-h-32 max-w-full rounded-md border border-border"
                  />
                )}
                <ActionButton
                  busy={false}
                  onClick={() => {
                    removePicture(index);
                  }}
                  className={textButtonClass}
                  id={focusId(context.prefix, block.id, `remove-picture-${attachmentId}`)}
                >
                  {t('builder.removePicture', { filename })}
                </ActionButton>
              </li>
            );
          })}
        </ul>
      )}
      {context.uploads.length === 0 ? null : (
        <ul className="flex flex-col gap-2">
          {context.uploads.map((entry) => (
            <UploadRow key={entry.key} entry={entry} actions={actions} />
          ))}
        </ul>
      )}
      {context.canAddPictures ? (
        <div>
          <input
            ref={fileInput}
            type="file"
            accept="image/jpeg,image/png,image/webp,image/gif"
            tabIndex={-1}
            aria-hidden="true"
            className="sr-only"
            onChange={(event) => {
              const file = event.target.files?.item(0) ?? null;
              event.target.value = '';
              if (file !== null) {
                actions.addPicture(file);
              }
            }}
          />
          <ActionButton
            busy={false}
            unavailable={full}
            onClick={() => {
              fileInput.current?.click();
            }}
            className={quietButtonClass}
            id={addId}
          >
            {t('builder.addPicture')}
          </ActionButton>
        </div>
      ) : null}
    </Group>
  );
}

function Explanation({
  block,
  context,
  actions,
}: {
  readonly block: QuestionBlock;
  readonly context: BlockContext;
  readonly actions: BlockActions;
}): ReactElement {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const regionId = useId();
  return (
    <div className="flex flex-col gap-2">
      <div>
        <button
          type="button"
          aria-expanded={open}
          aria-controls={regionId}
          onClick={() => {
            setOpen((value) => !value);
          }}
          className={textButtonClass}
        >
          {t('builder.explanation')}
        </button>
      </div>
      <div id={regionId} hidden={!open}>
        {open ? (
          <TextArea
            id={focusId(context.prefix, block.id, 'explanation')}
            label={t('builder.explanation')}
            hint={t('builder.explanationHint')}
            value={block.explanation}
            limit={limits.text}
            warnings={[]}
            onChange={(explanation) => {
              actions.change({ ...block, explanation });
            }}
          />
        ) : null}
      </div>
    </div>
  );
}

function FileKinds({
  block,
  actions,
}: {
  readonly block: Extract<Block, { readonly type: 'file' }>;
  readonly actions: BlockActions;
}): ReactElement {
  const { t } = useTranslation();

  function toggle(kind: FileKind, checked: boolean): void {
    const next = checked
      ? fileKinds.filter((entry) => entry === kind || block.allowedKinds.includes(entry))
      : block.allowedKinds.filter((entry) => entry !== kind);
    if (next.length > 0) {
      actions.change({ ...block, allowedKinds: next });
    }
  }

  return (
    <Group label={t('builder.allowedKinds')}>
      {fileKinds.map((kind) => (
        <label key={kind} className="flex min-h-11 items-center gap-2">
          <input
            type="checkbox"
            checked={block.allowedKinds.includes(kind)}
            onChange={(event) => {
              toggle(kind, event.target.checked);
            }}
            className="size-5"
          />
          {t(`builder.kinds.${kind}`)}
        </label>
      ))}
    </Group>
  );
}

function QuestionEditor({
  block,
  context,
  actions,
}: {
  readonly block: QuestionBlock;
  readonly context: BlockContext;
  readonly actions: BlockActions;
}): ReactElement {
  const { t } = useTranslation();
  const problemText = useProblemText(context.problems);
  const id = (part: string): string => focusId(context.prefix, block.id, part);
  return (
    <div className="flex flex-col gap-4">
      <TextArea
        id={id('first')}
        label={t('builder.prompt')}
        hint={t('builder.markdownHint')}
        value={block.prompt}
        limit={limits.text}
        warnings={problemText(['prompt_missing'])}
        onChange={(prompt) => {
          actions.change({ ...block, prompt });
        }}
      />
      <PicturesEditor block={block} context={context} actions={actions} />
      {isChoice(block) ? <OptionsEditor block={block} context={context} actions={actions} /> : null}
      {block.type === 'short_text' ? <AnswersEditor block={block} context={context} actions={actions} /> : null}
      {block.type === 'short_number' ? (
        <div className="flex flex-wrap gap-4">
          <TextInput
            id={id('value')}
            label={t('builder.value')}
            value={block.value}
            limit={limits.decimal}
            inputMode="decimal"
            warnings={problemText(['value_invalid'])}
            onChange={(value) => {
              actions.change({ ...block, value });
            }}
          />
          <TextInput
            id={id('tolerance')}
            label={t('builder.tolerance')}
            value={block.tolerance}
            limit={limits.decimal}
            inputMode="decimal"
            warnings={problemText(['tolerance_invalid'])}
            onChange={(tolerance) => {
              actions.change({ ...block, tolerance });
            }}
          />
        </div>
      ) : null}
      {block.type === 'long_text' ? (
        <IntegerField
          id={id('max-length')}
          label={t('builder.maxLength')}
          value={block.maxLength}
          range={limits.maxLength}
          rangeMessage={t('builder.maxLengthRange')}
          onChange={(maxLength) => {
            actions.change({ ...block, maxLength });
          }}
        />
      ) : null}
      {block.type === 'file' ? (
        <>
          <FileKinds block={block} actions={actions} />
          <IntegerField
            id={id('max-files')}
            label={t('builder.maxFiles')}
            value={block.maxFiles}
            range={limits.maxFiles}
            rangeMessage={t('builder.maxFilesRange')}
            onChange={(maxFiles) => {
              actions.change({ ...block, maxFiles });
            }}
          />
        </>
      ) : null}
      <IntegerField
        id={id('points')}
        label={t('builder.points')}
        value={block.points}
        range={limits.points}
        rangeMessage={t('builder.pointsRange')}
        onChange={(points) => {
          actions.change({ ...block, points });
        }}
      />
      <Explanation block={block} context={context} actions={actions} />
    </div>
  );
}

function MaterialEditor({
  block,
  context,
  actions,
}: {
  readonly block: Extract<Block, { readonly type: 'material' }>;
  readonly context: BlockContext;
  readonly actions: BlockActions;
}): ReactElement {
  const { t } = useTranslation();
  const material = context.materials.get(block.materialId);
  const url = material?.url ?? null;
  const address =
    material === undefined || url === null ? null : downloadAddress(material.attachmentId, url, context.origin);
  return (
    <div className="flex flex-col items-start gap-2">
      {material === undefined ? null : (
        <div className="flex flex-col gap-1">
          <p dir="auto" className="text-strong break-words">
            {material.title}
          </p>
          <p dir="auto" className="text-small break-words text-text-muted">
            {material.filename}
          </p>
          {material.deleted ? <p className="text-small text-warning">{t('builder.materialDeleted')}</p> : null}
          {address === null ? null : (
            <a
              href={address}
              download={material.filename}
              rel="noopener noreferrer"
              referrerPolicy="no-referrer"
              className={linkClass}
            >
              {t('builder.download')}
            </a>
          )}
        </div>
      )}
      <button
        id={focusId(context.prefix, block.id, 'first')}
        type="button"
        onClick={(event) => {
          actions.chooseMaterial(event.currentTarget);
        }}
        className={quietButtonClass}
      >
        {t('builder.changeMaterial')}
      </button>
    </div>
  );
}

export function BlockCard({
  block,
  context,
  actions,
}: {
  readonly block: Block;
  readonly context: BlockContext;
  readonly actions: BlockActions;
}): ReactElement {
  const { t } = useTranslation();
  const problemText = useProblemText(context.problems);
  const id = (part: string): string => focusId(context.prefix, block.id, part);
  const headingId = id('heading');
  const first = context.index === 0;
  const last = context.index === context.count - 1;

  const container = useRef<HTMLElement>(null);
  const latestActions = useRef(actions);
  useEffect(() => {
    latestActions.current = actions;
  });
  useEffect(() => {
    const element = container.current;
    if (element === null) {
      return undefined;
    }
    function focused(): void {
      latestActions.current.focused();
    }
    function dragOver(event: DragEvent): void {
      event.preventDefault();
      if (event.dataTransfer !== null) {
        event.dataTransfer.dropEffect = 'move';
      }
    }
    function drop(event: DragEvent): void {
      event.preventDefault();
      latestActions.current.drop();
    }
    element.addEventListener('focusin', focused);
    element.addEventListener('dragover', dragOver);
    element.addEventListener('drop', drop);
    return (): void => {
      element.removeEventListener('focusin', focused);
      element.removeEventListener('dragover', dragOver);
      element.removeEventListener('drop', drop);
    };
  }, []);

  return (
    <section
      ref={container}
      aria-labelledby={headingId}
      className="flex flex-col gap-4 rounded-lg p-4 focus-within:bg-surface"
    >
      <div className="flex flex-wrap items-center gap-2">
        <span
          draggable
          aria-hidden="true"
          title={t('builder.drag')}
          data-drag-handle=""
          onDragStart={(event) => {
            event.dataTransfer.effectAllowed = 'move';
            event.dataTransfer.setData('text/plain', block.id);
            actions.dragStart();
          }}
          className="inline-flex min-h-11 min-w-11 cursor-grab items-center justify-center text-icon select-none"
        >
          ⠿
        </span>
        <h3 id={headingId} tabIndex={-1} className="grow font-serif text-strong outline-none">
          {t('builder.blockHeading', { position: context.index + 1, type: t(`builder.types.${block.type}`) })}
        </h3>
        <div className="flex flex-wrap gap-1">
          <ActionButton
            busy={false}
            unavailable={first}
            onClick={() => {
              actions.move(-1, 'move-up');
            }}
            className={textButtonClass}
            id={id('move-up')}
          >
            {t('builder.moveUp')}
          </ActionButton>
          <ActionButton
            busy={false}
            unavailable={last}
            onClick={() => {
              actions.move(1, 'move-down');
            }}
            className={textButtonClass}
            id={id('move-down')}
          >
            {t('builder.moveDown')}
          </ActionButton>
          <ActionButton busy={false} unavailable={context.full} onClick={actions.duplicate} className={textButtonClass}>
            {t('builder.duplicate')}
          </ActionButton>
          <ActionButton busy={false} onClick={actions.remove} className={textButtonClass}>
            {t('builder.delete')}
          </ActionButton>
        </div>
      </div>
      {block.type === 'instruction' ? (
        <TextArea
          id={id('first')}
          label={t('builder.text')}
          hint={t('builder.markdownHint')}
          value={block.text}
          limit={limits.text}
          rows={6}
          warnings={problemText(['text_missing'])}
          onChange={(text) => {
            actions.change({ ...block, text });
          }}
        />
      ) : null}
      {block.type === 'material' ? <MaterialEditor block={block} context={context} actions={actions} /> : null}
      {isQuestion(block) ? <QuestionEditor block={block} context={context} actions={actions} /> : null}
    </section>
  );
}
