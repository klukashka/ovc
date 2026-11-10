import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { fieldClass, linkClass } from '../../components/ui/Form.tsx';
import { studentFileAddress } from '../../lib/files/downloadAddress.ts';
import { Markdown } from '../../lib/markdown/Markdown.tsx';
import { formatStorage } from '../settings/storageAmount.ts';
import type { StudentAssignment, StudentBlock, StudentFile, StudentMaterial } from './studentView.ts';

const shortAnswerLimit = 500;

const acceptedFileTypes: Readonly<Record<string, string>> = {
  images: 'image/jpeg,image/png,image/webp,image/gif',
  pdf: 'application/pdf',
  office: '.doc,.docx,.xls,.xlsx,.ppt,.pptx,.odt,.ods,.odp',
  audio: 'audio/*',
};

function Pictures({
  images,
  origin,
}: {
  readonly images: readonly StudentFile[];
  readonly origin: string;
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

function AnswerField({ block }: { readonly block: StudentBlock }): ReactElement | null {
  const { t } = useTranslation();
  const fieldId = useId();
  const hintId = useId();
  switch (block.type) {
    case 'single_choice':
    case 'multiple_choice':
      return (
        <div className="flex flex-col gap-2">
          {block.options.map((option) => (
            <label key={option.id} className="flex min-h-11 items-center gap-3">
              <input type={block.type === 'single_choice' ? 'radio' : 'checkbox'} name={block.id} value={option.id} />
              <span dir="auto" className="break-words">
                {option.text}
              </span>
            </label>
          ))}
        </div>
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
            maxLength={shortAnswerLimit}
            dir="auto"
            className={fieldClass}
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
          />
          <p id={hintId} className="text-small text-text-muted">
            {t('assignment.lengthHint', { count: block.maxLength })}
          </p>
        </div>
      );
    case 'file':
      return (
        <div className="flex flex-col gap-1">
          <label htmlFor={fieldId}>{t('assignment.attach')}</label>
          <input
            id={fieldId}
            type="file"
            multiple={block.maxFiles > 1}
            accept={block.allowedKinds.map((kind) => acceptedFileTypes[kind] ?? '').join(',')}
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
    case 'instruction':
    case 'material':
      return null;
  }
}

function questionNumbers(blocks: readonly StudentBlock[]): ReadonlyMap<string, number> {
  const numbers = new Map<string, number>();
  for (const block of blocks) {
    if (block.type !== 'instruction' && block.type !== 'material') {
      numbers.set(block.id, numbers.size + 1);
    }
  }
  return numbers;
}

function StudentBlockView({
  block,
  number,
  origin,
}: {
  readonly block: StudentBlock;
  readonly number: number;
  readonly origin: string;
}): ReactElement {
  const { t } = useTranslation();
  if (block.type === 'instruction') {
    return <Markdown source={block.text} />;
  }
  if (block.type === 'material') {
    return block.material === null ? <></> : <MaterialEntry material={block.material} origin={origin} />;
  }
  return (
    <fieldset className="flex min-w-0 flex-col gap-3">
      <legend className="mb-2 text-small text-text-muted">{t('assignment.question', { number })}</legend>
      <Markdown source={block.prompt} />
      <Pictures images={block.images} origin={origin} />
      <AnswerField block={block} />
    </fieldset>
  );
}

export function StudentBlocks({
  assignment,
  origin,
}: {
  readonly assignment: StudentAssignment;
  readonly origin: string;
}): ReactElement {
  const numbers = questionNumbers(assignment.blocks);
  return (
    <div className="flex flex-col gap-6">
      <h3 dir="auto" className="font-serif text-title break-words">
        {assignment.title}
      </h3>
      <ol className="flex flex-col gap-6">
        {assignment.blocks.map((block) => (
          <li key={block.id} className="border-t border-border pt-4">
            <StudentBlockView block={block} number={numbers.get(block.id) ?? 0} origin={origin} />
          </li>
        ))}
      </ol>
    </div>
  );
}
