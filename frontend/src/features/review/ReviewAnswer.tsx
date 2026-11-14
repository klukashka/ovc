import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { linkClass } from '../../components/ui/Form.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { downloadAddress } from '../../lib/files/downloadAddress.ts';
import { Markdown } from '../../lib/markdown/Markdown.tsx';
import { formatStorage } from '../settings/storageAmount.ts';
import type { ReviewAnswer, ReviewFile, ReviewQuestion } from './api.ts';

export function fileAddress(file: ReviewFile): string | null {
  return file.url === '' ? null : downloadAddress(file.attachmentId, file.url, window.location.origin);
}

function isPicture(file: ReviewFile): boolean {
  return file.mediaType.startsWith('image/') && file.width !== null && file.height !== null;
}

export function FileLink({ file }: { readonly file: ReviewFile }): ReactElement {
  const address = fileAddress(file);
  return address === null ? (
    <span dir="auto" className="min-w-0 break-words">
      {file.filename}
    </span>
  ) : (
    <a
      href={address}
      download={file.filename}
      rel="noopener noreferrer"
      referrerPolicy="no-referrer"
      dir="auto"
      className={`${linkClass} min-h-0 min-w-0 break-words`}
    >
      {file.filename}
    </a>
  );
}

function Picture({
  file,
  onFailed,
}: {
  readonly file: ReviewFile;
  readonly onFailed: (address: string) => void;
}): ReactElement | null {
  const address = fileAddress(file);
  if (address === null || !isPicture(file)) {
    return null;
  }
  return (
    <img
      src={address}
      alt=""
      width={file.width ?? undefined}
      height={file.height ?? undefined}
      loading="lazy"
      referrerPolicy="no-referrer"
      onError={() => {
        onFailed(address);
      }}
      className="h-auto max-h-96 w-auto max-w-full rounded-md border border-border"
    />
  );
}

function FileList({
  files,
  onFailed,
}: {
  readonly files: readonly ReviewFile[];
  readonly onFailed: (address: string) => void;
}): ReactElement {
  const { i18n } = useTranslation();
  return (
    <ul className="flex flex-col gap-3">
      {files.map((file) => (
        <li key={file.attachmentId} className="flex flex-col items-start gap-2">
          <span className="flex flex-wrap items-center gap-x-3">
            <FileLink file={file} />
            <span className="text-small text-text-muted">{formatStorage(file.sizeBytes, i18n.language)}</span>
          </span>
          <Picture file={file} onFailed={onFailed} />
        </li>
      ))}
    </ul>
  );
}

function correctAnswer(question: ReviewQuestion): string | null {
  switch (question.type) {
    case 'short_text':
      return question.acceptedAnswers.join(', ');
    case 'short_number':
      return question.tolerance === '' || question.tolerance === '0'
        ? question.value
        : `${question.value} ± ${question.tolerance}`;
    default:
      return null;
  }
}

function AnswerValue({
  question,
  answer,
  onFailed,
}: {
  readonly question: ReviewQuestion;
  readonly answer: ReviewAnswer | undefined;
  readonly onFailed: (address: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  switch (question.type) {
    case 'single_choice':
    case 'multiple_choice':
      return (
        <ul className="flex flex-col gap-1">
          {question.options.map((option) => (
            <li key={option.id} className="flex flex-wrap items-center gap-x-3">
              <span dir="auto" className="break-words">
                {option.text}
              </span>
              {answer?.optionIds.includes(option.id) === true ? (
                <StatusWord tone="info">{t('homework.review.chosen')}</StatusWord>
              ) : null}
              {option.correct ? <StatusWord tone="success">{t('homework.review.correct')}</StatusWord> : null}
            </li>
          ))}
        </ul>
      );
    case 'file':
      return answer === undefined || answer.files.length === 0 ? (
        <p className="text-text-muted">{t('homework.review.noAnswer')}</p>
      ) : (
        <FileList files={answer.files} onFailed={onFailed} />
      );
    case 'short_text':
    case 'short_number':
    case 'long_text':
      return answer === undefined || answer.text === '' ? (
        <p className="text-text-muted">{t('homework.review.noAnswer')}</p>
      ) : (
        <p dir="auto" className="break-words whitespace-pre-wrap">
          {answer.text}
        </p>
      );
  }
}

export function QuestionAnswer({
  headingId,
  number,
  question,
  answer,
  onFailed,
}: {
  readonly headingId: string;
  readonly number: number;
  readonly question: ReviewQuestion;
  readonly answer: ReviewAnswer | undefined;
  readonly onFailed: (address: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  const correct = correctAnswer(question);
  const normalised = answer?.normalised ?? null;
  return (
    <div className="flex min-w-0 flex-col gap-3">
      <h2 id={headingId} className="text-strong">
        {t('homework.review.question', { number })}
      </h2>
      <Markdown source={question.prompt} />
      {question.images.length === 0 ? null : <FileList files={question.images} onFailed={onFailed} />}
      <div className="flex flex-col gap-1">
        <span className="text-label text-text-muted">{t('homework.review.answer')}</span>
        <AnswerValue question={question} answer={answer} onFailed={onFailed} />
        {normalised === null || normalised === answer?.text ? null : (
          <p dir="auto" className="text-small break-words text-text-muted">
            {t('homework.review.normalised', { answer: normalised })}
          </p>
        )}
      </div>
      {correct === null ? null : (
        <div className="flex flex-col gap-1">
          <span className="text-label text-text-muted">{t('homework.review.correctAnswer')}</span>
          <p dir="auto" className="break-words">
            {correct}
          </p>
        </div>
      )}
    </div>
  );
}
