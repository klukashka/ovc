import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';

import { linkClass } from '../../components/ui/Form.tsx';
import { downloadAddress } from '../../lib/files/downloadAddress.ts';
import { Markdown } from '../../lib/markdown/Markdown.tsx';
import { formatStorage } from '../settings/storageAmount.ts';
import type { AnswerFile, QuestionBlock, RevealedAnswer, ReviewItem } from './api.ts';

function withoutTolerance(tolerance: string | null): boolean {
  return tolerance === null || tolerance.trim() === '' || Number(tolerance) === 0;
}

export function RevealedPart({
  block,
  revealed,
}: {
  readonly block: QuestionBlock;
  readonly revealed: RevealedAnswer;
}): ReactElement | null {
  const { t } = useTranslation();
  const accepted = block.type === 'short_text' && revealed.acceptedAnswers.length > 0;
  const value = block.type === 'short_number' ? revealed.correctValue : null;
  if (!accepted && value === null && revealed.explanation === '') {
    return null;
  }
  return (
    <div data-revealed="" className="flex flex-col gap-1 rounded-md border border-border p-3">
      {accepted ? (
        <p dir="auto" className="break-words">
          {t('assignment.revealed.accepted', { answers: revealed.acceptedAnswers.join(', ') })}
        </p>
      ) : null}
      {value === null ? null : (
        <p dir="auto" className="break-words">
          {withoutTolerance(revealed.tolerance)
            ? t('assignment.revealed.value', { value })
            : t('assignment.revealed.valueWithTolerance', { value, tolerance: revealed.tolerance ?? '' })}
        </p>
      )}
      {revealed.explanation === '' ? null : (
        <div className="flex flex-col gap-1">
          <p className="text-small text-text-muted">{t('assignment.revealed.explanation')}</p>
          <Markdown source={revealed.explanation} />
        </div>
      )}
    </div>
  );
}

function CorrectedFile({ file, origin }: { readonly file: AnswerFile; readonly origin: string }): ReactElement {
  const { i18n } = useTranslation();
  const address = file.url === '' ? null : downloadAddress(file.attachmentId, file.url, origin);
  return (
    <li className="flex flex-wrap items-baseline gap-x-3">
      {address === null ? (
        <span dir="auto" className="break-words">
          {file.filename}
        </span>
      ) : (
        <a
          href={address}
          download={file.filename}
          rel="noopener noreferrer"
          referrerPolicy="no-referrer"
          dir="auto"
          className={`${linkClass} break-words`}
        >
          {file.filename}
        </a>
      )}
      <span className="text-small text-text-muted">{formatStorage(file.sizeBytes, i18n.language)}</span>
    </li>
  );
}

export function QuestionReview({
  item,
  highlighted,
  origin,
}: {
  readonly item: ReviewItem;
  readonly highlighted: boolean;
  readonly origin: string;
}): ReactElement {
  const { t } = useTranslation();
  const filesId = useId();
  return (
    <div
      data-review-item=""
      className={`flex flex-col gap-2 rounded-md p-3 ${highlighted ? 'border border-warning bg-hover' : 'bg-hover'}`}
    >
      {item.comment === '' ? null : (
        <div className="flex flex-col gap-1">
          <p className="text-small text-text-muted">{t('assignment.review.comment')}</p>
          <p dir="auto" className="break-words whitespace-pre-wrap">
            {item.comment}
          </p>
        </div>
      )}
      {item.correctedFiles.length === 0 ? null : (
        <div className="flex flex-col gap-1">
          <p id={filesId} className="text-small text-text-muted">
            {t('assignment.review.correctedFiles')}
          </p>
          <ul aria-labelledby={filesId} className="flex flex-col gap-1">
            {item.correctedFiles.map((file) => (
              <CorrectedFile key={file.attachmentId} file={file} origin={origin} />
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

export function OverallFeedback({ feedback }: { readonly feedback: string }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} data-feedback="" className="flex flex-col gap-2 rounded-md bg-hover p-4">
      <h2 id={headingId} className="text-heading">
        {t('assignment.review.feedback')}
      </h2>
      <Markdown source={feedback} />
    </section>
  );
}
