import { type ReactElement, type ReactNode, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { fieldClass, linkClass } from '../../components/ui/Form.tsx';
import { studentFileAddress } from '../../lib/files/downloadAddress.ts';
import { Markdown } from '../../lib/markdown/Markdown.tsx';
import { formatStorage } from '../settings/storageAmount.ts';
import { type Answer, type QuestionBlock, emptyAnswer, isQuestion } from './api.ts';
import { acceptList } from './fileKinds.ts';
import type { StudentAssignment, StudentBlock, StudentFile, StudentMaterial } from './studentView.ts';

const shortAnswerLimit = 500;

const shortNumberLimit = 64;

const noOptions: ReadonlySet<string> = new Set();

export interface Answering {
  readonly answers: ReadonlyMap<string, Answer>;
  readonly editable: ReadonlySet<string>;
  readonly onChange: (block: QuestionBlock, answer: Answer) => void;
  readonly fileControl: (block: Extract<QuestionBlock, { readonly type: 'file' }>, answer: Answer) => ReactNode;
  readonly below: (block: QuestionBlock) => ReactNode;
  readonly invalid: ReadonlySet<string>;
  readonly correctOptions: (block: QuestionBlock) => ReadonlySet<string>;
}

function Pictures({
  images,
  origin,
  onAddressFailed,
}: {
  readonly images: readonly StudentFile[];
  readonly origin: string;
  readonly onAddressFailed: ((address: string) => void) | null;
}): ReactElement | null {
  if (images.length === 0) {
    return null;
  }
  return (
    <div className="flex flex-wrap gap-3">
      {images.map((image, index) => {
        const address = studentFileAddress(image.url, origin);
        return address === null ? (
          <span key={index} dir="auto" className="text-small break-words text-text-muted">
            {image.filename}
          </span>
        ) : (
          <img
            key={index}
            src={address}
            alt={image.filename}
            width={image.width ?? undefined}
            height={image.height ?? undefined}
            referrerPolicy="no-referrer"
            onError={() => {
              onAddressFailed?.(address);
            }}
            className="h-auto max-h-80 max-w-full rounded-md border border-border"
          />
        );
      })}
    </div>
  );
}

function MaterialEntry({
  material,
  origin,
}: {
  readonly material: StudentMaterial;
  readonly origin: string;
}): ReactElement {
  const { t, i18n } = useTranslation();
  const address = studentFileAddress(material.url, origin);
  const details = `${material.filename} · ${formatStorage(material.sizeBytes, i18n.language)}`;
  return (
    <div className="flex flex-col gap-1">
      <p className="text-small text-text-muted">{t('assignment.material')}</p>
      {address === null ? (
        <p dir="auto" className="text-strong break-words">
          {material.title}
        </p>
      ) : (
        <a
          href={address}
          download={material.filename}
          rel="noopener noreferrer"
          referrerPolicy="no-referrer"
          dir="auto"
          className={`${linkClass} break-words`}
        >
          {material.title}
        </a>
      )}
      <p dir="auto" className="text-small break-words text-text-muted">
        {details}
      </p>
    </div>
  );
}

function ChoiceField({
  block,
  answer,
  editable,
  correct,
  onChange,
}: {
  readonly block: Extract<QuestionBlock, { readonly type: 'single_choice' | 'multiple_choice' }>;
  readonly answer: Answer | null;
  readonly editable: boolean;
  readonly correct: ReadonlySet<string>;
  readonly onChange: (answer: Answer) => void;
}): ReactElement {
  const { t } = useTranslation();
  const single = block.type === 'single_choice';
  return (
    <div className="flex flex-col gap-2">
      {block.options.map((option) => (
        <label key={option.id} className="flex min-h-11 items-center gap-3">
          <input
            type={single ? 'radio' : 'checkbox'}
            name={block.id}
            value={option.id}
            {...(answer === null
              ? {}
              : {
                  checked: answer.optionIds.includes(option.id),
                  disabled: !editable,
                  onChange: (event: { readonly currentTarget: HTMLInputElement }): void => {
                    const checked = event.currentTarget.checked;
                    const optionIds = single
                      ? [option.id]
                      : checked
                        ? [...answer.optionIds.filter((id) => id !== option.id), option.id]
                        : answer.optionIds.filter((id) => id !== option.id);
                    onChange({ ...answer, optionIds });
                  },
                })}
          />
          <span dir="auto" className="break-words">
            {option.text}
          </span>
          {correct.has(option.id) ? (
            <span data-correct-option="" className="text-small font-medium text-success">
              {t('assignment.revealed.correct')}
            </span>
          ) : null}
        </label>
      ))}
    </div>
  );
}

function AnswerField({
  block,
  answering,
}: {
  readonly block: QuestionBlock;
  readonly answering: Answering | null;
}): ReactElement | null {
  const { t } = useTranslation();
  const fieldId = useId();
  const hintId = useId();
  const answer = answering === null ? null : (answering.answers.get(block.id) ?? emptyAnswer);
  const editable = answering?.editable.has(block.id) ?? true;
  const invalid = answering?.invalid.has(block.id) === true;
  const change = (next: Answer): void => {
    answering?.onChange(block, next);
  };
  const textProps =
    answer === null
      ? {}
      : {
          value: answer.text,
          readOnly: !editable,
          'aria-invalid': invalid ? true : undefined,
          onChange: (event: { readonly currentTarget: HTMLInputElement | HTMLTextAreaElement }): void => {
            change({ ...answer, text: event.currentTarget.value });
          },
        };
  switch (block.type) {
    case 'single_choice':
    case 'multiple_choice':
      return (
        <ChoiceField
          block={block}
          answer={answer}
          editable={editable}
          correct={answering?.correctOptions(block) ?? noOptions}
          onChange={change}
        />
      );
    case 'short_text':
    case 'short_number':
      return (
        <div className="flex flex-col gap-1">
          <label htmlFor={fieldId}>{t('assignment.answer')}</label>
          <input
            id={fieldId}
            type="text"
            inputMode={block.type === 'short_number' ? 'decimal' : 'text'}
            maxLength={block.type === 'short_number' ? shortNumberLimit : shortAnswerLimit}
            dir="auto"
            className={fieldClass}
            {...textProps}
          />
        </div>
      );
    case 'long_text':
      return (
        <div className="flex flex-col gap-1">
          <label htmlFor={fieldId}>{t('assignment.answer')}</label>
          <textarea
            id={fieldId}
            maxLength={block.maxLength}
            rows={5}
            dir="auto"
            aria-describedby={hintId}
            className={`${fieldClass} py-2`}
            {...textProps}
          />
          <p id={hintId} className="text-small text-text-muted">
            {t('assignment.lengthHint', { count: block.maxLength })}
          </p>
        </div>
      );
    case 'file':
      if (answering !== null && answer !== null) {
        return <>{answering.fileControl(block, answer)}</>;
      }
      return (
        <div className="flex flex-col gap-1">
          <label htmlFor={fieldId}>{t('assignment.attach')}</label>
          <input
            id={fieldId}
            type="file"
            multiple={block.maxFiles > 1}
            accept={acceptList(block.allowedKinds)}
            aria-describedby={hintId}
          />
          <p id={hintId} className="text-small text-text-muted">
            {t('assignment.fileHint', {
              kinds: block.allowedKinds.map((kind) => t(`assignment.kinds.${kind}`)).join(', '),
              count: block.maxFiles,
            })}
          </p>
        </div>
      );
  }
}

export function questionNumbers(blocks: readonly StudentBlock[]): ReadonlyMap<string, number> {
  const numbers = new Map<string, number>();
  for (const block of blocks) {
    if (isQuestion(block)) {
      numbers.set(block.id, numbers.size + 1);
    }
  }
  return numbers;
}

function checkerOf(type: QuestionBlock['type']): 'automatic' | 'tutor' {
  switch (type) {
    case 'single_choice':
    case 'multiple_choice':
    case 'short_text':
    case 'short_number':
      return 'automatic';
    case 'long_text':
    case 'file':
      return 'tutor';
  }
}

function StudentBlockView({
  block,
  number,
  origin,
  answering,
  onAddressFailed,
}: {
  readonly block: StudentBlock;
  readonly number: number;
  readonly origin: string;
  readonly answering: Answering | null;
  readonly onAddressFailed: ((address: string) => void) | null;
}): ReactElement {
  const { t } = useTranslation();
  const promptId = useId();
  if (block.type === 'instruction') {
    return <Markdown source={block.text} />;
  }
  if (block.type === 'material') {
    return block.material === null ? <></> : <MaterialEntry material={block.material} origin={origin} />;
  }
  return (
    <fieldset aria-describedby={promptId} data-question={block.id} className="flex min-w-0 flex-col gap-3">
      <legend className="mb-2 text-small text-text-muted">
        {answering === null || answering.editable.size === 0
          ? t('assignment.question', { number })
          : `${t('assignment.question', { number })} · ${t(`assignment.checker.${checkerOf(block.type)}`)}`}
      </legend>
      <div id={promptId}>
        <Markdown source={block.prompt} />
      </div>
      <Pictures images={block.images} origin={origin} onAddressFailed={onAddressFailed} />
      <AnswerField block={block} answering={answering} />
      {answering === null ? null : answering.below(block)}
    </fieldset>
  );
}

export function StudentBlocks({
  assignment,
  origin,
  answering = null,
  showTitle = true,
  onAddressFailed = null,
}: {
  readonly assignment: StudentAssignment;
  readonly origin: string;
  readonly answering?: Answering | null;
  readonly showTitle?: boolean;
  readonly onAddressFailed?: ((address: string) => void) | null;
}): ReactElement {
  const numbers = questionNumbers(assignment.blocks);
  return (
    <div className="flex flex-col gap-6">
      {showTitle ? (
        <h3 dir="auto" className="font-serif text-title break-words">
          {assignment.title}
        </h3>
      ) : null}
      <ol className="flex flex-col gap-6">
        {assignment.blocks.map((block) => (
          <li key={block.id} className="border-t border-border pt-4">
            <StudentBlockView
              block={block}
              number={numbers.get(block.id) ?? 0}
              origin={origin}
              answering={answering}
              onAddressFailed={onAddressFailed}
            />
          </li>
        ))}
      </ol>
    </div>
  );
}
