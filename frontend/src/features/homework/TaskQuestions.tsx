import { type ReactElement, type ReactNode, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  SelectChevron,
  fieldClass,
  quietButtonClass,
  selectClass,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { type ChoiceBlock, type QuestionBlock, clampInteger, cutToCodePoints, limits } from '../builder/blocks.ts';
import {
  type QuestionField,
  type QuestionType,
  automaticTypes,
  countsOfQuestions,
  isManualType,
  isQuestionType,
  manualTypes,
  problemKey,
  resultHintOf,
} from './newTask.ts';

export function questionFieldId(questionId: string, field: QuestionField): string {
  return `question-${questionId}-${field}`;
}

function headingId(questionId: string): string {
  return `question-${questionId}-heading`;
}

function FieldError({ id, message }: { readonly id: string; readonly message: string | undefined }): ReactNode {
  return message === undefined ? null : (
    <p id={id} className="text-small text-danger">
      {message}
    </p>
  );
}

function described(errorId: string, message: string | undefined, hintId?: string): string | undefined {
  const ids = [hintId, message === undefined ? undefined : errorId].filter((id) => id !== undefined);
  return ids.length === 0 ? undefined : ids.join(' ');
}

function ChoiceFields({
  question,
  error,
  locked,
  onChange,
}: {
  readonly question: ChoiceBlock;
  readonly error: string | undefined;
  readonly locked: boolean;
  readonly onChange: (question: QuestionBlock) => void;
}): ReactElement {
  const { t } = useTranslation();
  const legendId = useId();
  const errorId = useId();
  const single = question.type === 'single_choice';
  const pendingFocus = useRef<string | null>(null);

  useEffect(() => {
    const target = pendingFocus.current;
    pendingFocus.current = null;
    if (target !== null) {
      document.getElementById(target)?.focus();
    }
  });

  function optionInputId(optionId: string, index: number): string {
    return index === 0 ? questionFieldId(question.id, 'options') : `question-${question.id}-option-${optionId}`;
  }

  function removeOption(index: number): void {
    const options = question.options.filter((_, position) => position !== index);
    const takesPlace = Math.min(index, options.length - 1);
    const target = options[takesPlace];
    pendingFocus.current = target === undefined ? null : optionInputId(target.id, takesPlace);
    onChange({ ...question, options });
  }

  return (
    <fieldset aria-labelledby={legendId} aria-describedby={described(errorId, error)} className="flex flex-col gap-2">
      <legend id={legendId} className="text-label">
        {t('homework.newTask.options')}
      </legend>
      {question.options.map((choice, index) => (
        <div key={choice.id} className="flex flex-wrap items-center gap-3">
          <input
            id={optionInputId(choice.id, index)}
            aria-label={t('homework.newTask.option', { number: index + 1 })}
            aria-invalid={error === undefined ? undefined : true}
            value={choice.text}
            readOnly={locked}
            onChange={(event) => {
              const text = cutToCodePoints(event.target.value, limits.option);
              onChange({
                ...question,
                options: question.options.map((other) => (other.id === choice.id ? { ...other, text } : other)),
              });
            }}
            className={`${fieldClass} min-w-0 flex-1`}
          />
          <label className="inline-flex min-h-11 items-center gap-2 text-small">
            <input
              type={single ? 'radio' : 'checkbox'}
              name={single ? `correct-${question.id}` : undefined}
              checked={choice.correct}
              disabled={locked}
              aria-label={t('homework.newTask.correctOption', { number: index + 1 })}
              onChange={(event) => {
                const checked = event.target.checked;
                onChange({
                  ...question,
                  options: question.options.map((other) =>
                    other.id === choice.id
                      ? { ...other, correct: checked }
                      : single
                        ? { ...other, correct: false }
                        : other,
                  ),
                });
              }}
              className="size-4 accent-accent"
            />
            {t('homework.newTask.correct')}
          </label>
          {locked || question.options.length <= 2 ? null : (
            <button
              type="button"
              onClick={() => {
                removeOption(index);
              }}
              className={textButtonClass}
            >
              {t('homework.newTask.removeOption', { number: index + 1 })}
            </button>
          )}
        </div>
      ))}
      {locked || question.options.length >= limits.options ? null : (
        <div>
          <button
            type="button"
            onClick={() => {
              onChange({
                ...question,
                options: [...question.options, { id: crypto.randomUUID(), text: '', correct: false }],
              });
            }}
            className={quietButtonClass}
          >
            {t('homework.newTask.addOption')}
          </button>
        </div>
      )}
      <FieldError id={errorId} message={error} />
    </fieldset>
  );
}

function TextInput({
  id,
  label,
  hint,
  value,
  error,
  locked,
  multiline = false,
  inputMode,
  onChange,
}: {
  readonly id: string;
  readonly label: string;
  readonly hint?: string;
  readonly value: string;
  readonly error: string | undefined;
  readonly locked: boolean;
  readonly multiline?: boolean;
  readonly inputMode?: 'decimal';
  readonly onChange: (value: string) => void;
}): ReactElement {
  const errorId = useId();
  const hintId = useId();
  const shared = {
    id,
    value,
    readOnly: locked,
    'aria-invalid': error === undefined ? undefined : true,
    'aria-describedby': described(errorId, error, hint === undefined ? undefined : hintId),
    className: fieldClass,
  } as const;
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-label">
        {label}
      </label>
      {hint === undefined ? null : (
        <p id={hintId} className="text-small text-text-muted">
          {hint}
        </p>
      )}
      {multiline ? (
        <textarea
          {...shared}
          rows={3}
          onChange={(event) => {
            onChange(event.target.value);
          }}
        />
      ) : (
        <input
          {...shared}
          inputMode={inputMode}
          onChange={(event) => {
            onChange(event.target.value);
          }}
        />
      )}
      <FieldError id={errorId} message={error} />
    </div>
  );
}

function TypeFields({
  question,
  message,
  locked,
  onChange,
}: {
  readonly question: QuestionBlock;
  readonly message: (field: QuestionField) => string | undefined;
  readonly locked: boolean;
  readonly onChange: (question: QuestionBlock) => void;
}): ReactNode {
  const { t } = useTranslation();
  switch (question.type) {
    case 'single_choice':
    case 'multiple_choice':
      return <ChoiceFields question={question} error={message('options')} locked={locked} onChange={onChange} />;
    case 'short_text':
      return (
        <TextInput
          id={questionFieldId(question.id, 'answers')}
          label={t('homework.newTask.acceptedAnswers')}
          hint={t('homework.newTask.acceptedAnswersHint')}
          value={question.acceptedAnswers.join('\n')}
          error={message('answers')}
          locked={locked}
          multiline
          onChange={(value) => {
            onChange({
              ...question,
              acceptedAnswers: value
                .split('\n')
                .slice(0, limits.acceptedAnswers)
                .map((answer) => cutToCodePoints(answer, limits.acceptedAnswer)),
            });
          }}
        />
      );
    case 'short_number':
      return (
        <div className="grid grid-cols-2 gap-3">
          <TextInput
            id={questionFieldId(question.id, 'value')}
            label={t('homework.newTask.value')}
            value={question.value}
            error={message('value')}
            locked={locked}
            inputMode="decimal"
            onChange={(value) => {
              onChange({ ...question, value: cutToCodePoints(value.trim(), limits.decimal) });
            }}
          />
          <TextInput
            id={questionFieldId(question.id, 'tolerance')}
            label={t('homework.newTask.tolerance')}
            value={question.tolerance}
            error={message('tolerance')}
            locked={locked}
            inputMode="decimal"
            onChange={(value) => {
              onChange({ ...question, tolerance: cutToCodePoints(value.trim(), limits.decimal) });
            }}
          />
        </div>
      );
    case 'long_text':
    case 'file':
      return null;
  }
}

function QuestionCard({
  question,
  number,
  message,
  locked,
  onChange,
  onRemove,
}: {
  readonly question: QuestionBlock;
  readonly number: number;
  readonly message: (field: QuestionField) => string | undefined;
  readonly locked: boolean;
  readonly onChange: (question: QuestionBlock) => void;
  readonly onRemove: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const pointsId = useId();
  const manual = isManualType(question.type);
  return (
    <li className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 id={headingId(question.id)} tabIndex={-1} className="text-heading outline-none">
          {t('homework.newTask.questionHeading', { number })} · {t(`homework.newTask.types.${question.type}`)}
        </h3>
        <span className={`text-small ${manual ? 'text-accent-text' : 'text-info'}`}>
          {manual ? t('homework.newTask.checker.manual') : t('homework.newTask.checker.automatic')}
        </span>
      </div>
      <TextInput
        id={questionFieldId(question.id, 'prompt')}
        label={t('homework.newTask.prompt')}
        value={question.prompt}
        error={message('prompt')}
        locked={locked}
        multiline
        onChange={(value) => {
          onChange({ ...question, prompt: cutToCodePoints(value, limits.text) });
        }}
      />
      <TypeFields question={question} message={message} locked={locked} onChange={onChange} />
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-col gap-1">
          <label htmlFor={pointsId} className="text-label">
            {t('homework.newTask.points')}
          </label>
          <input
            id={pointsId}
            type="number"
            min={limits.points.min}
            max={limits.points.max}
            value={question.points}
            readOnly={locked}
            onChange={(event) => {
              const parsed = Number(event.target.value);
              onChange({ ...question, points: clampInteger(Number.isFinite(parsed) ? parsed : 0, limits.points) });
            }}
            className={`${fieldClass} w-24`}
          />
        </div>
        {locked ? null : (
          <button type="button" onClick={onRemove} className={textButtonClass}>
            {t('homework.newTask.removeQuestion', { number })}
          </button>
        )}
      </div>
    </li>
  );
}

function AddQuestion({
  typeSelect,
  disabled,
  onAdd,
}: {
  readonly typeSelect: RefObject<HTMLSelectElement | null>;
  readonly disabled: boolean;
  readonly onAdd: (type: QuestionType) => void;
}): ReactElement {
  const { t } = useTranslation();
  const selectId = useId();
  const [type, setType] = useState<QuestionType>('single_choice');
  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="flex flex-col gap-1">
        <label htmlFor={selectId} className="text-label">
          {t('homework.newTask.questionType')}
        </label>
        <div className="relative">
          <select
            ref={typeSelect}
            id={selectId}
            value={type}
            onChange={(event) => {
              const chosen = event.target.value;
              if (isQuestionType(chosen)) {
                setType(chosen);
              }
            }}
            className={selectClass}
          >
            <optgroup label={t('homework.newTask.groups.automatic')}>
              {automaticTypes.map((option) => (
                <option key={option} value={option}>
                  {t(`homework.newTask.types.${option}`)}
                </option>
              ))}
            </optgroup>
            <optgroup label={t('homework.newTask.groups.manual')}>
              {manualTypes.map((option) => (
                <option key={option} value={option}>
                  {t(`homework.newTask.types.${option}`)}
                </option>
              ))}
            </optgroup>
          </select>
          <SelectChevron />
        </div>
      </div>
      <button
        type="button"
        aria-disabled={disabled}
        onClick={() => {
          if (!disabled) {
            onAdd(type);
          }
        }}
        className={quietButtonClass}
      >
        {t('homework.newTask.addQuestion')}
      </button>
    </div>
  );
}

export function TaskQuestions({
  questions,
  problems,
  locked,
  full,
  onAdd,
  onChange,
  onRemove,
}: {
  readonly questions: readonly QuestionBlock[];
  readonly problems: (key: string) => string | undefined;
  readonly locked: boolean;
  readonly full: boolean;
  readonly onAdd: (type: QuestionType) => void;
  readonly onChange: (question: QuestionBlock) => void;
  readonly onRemove: (questionId: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  const labelId = useId();
  const typeSelect = useRef<HTMLSelectElement>(null);
  const pendingFocus = useRef<string | null>(null);
  const counts = countsOfQuestions(questions);

  useEffect(() => {
    const target = pendingFocus.current;
    pendingFocus.current = null;
    if (target === null) {
      return;
    }
    const element = target === '' ? typeSelect.current : document.getElementById(target);
    element?.focus();
  });

  function remove(index: number): void {
    const next = questions[index + 1] ?? questions[index - 1];
    pendingFocus.current = next === undefined ? '' : headingId(next.id);
    const removed = questions[index];
    if (removed !== undefined) {
      onRemove(removed.id);
    }
  }

  return (
    <section aria-labelledby={labelId} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 id={labelId} className="text-label">
          {t('homework.newTask.questions')} <span className="font-normal text-text-muted">{t('form.optional')}</span>
        </h2>
        <p className="text-small text-text-muted">
          {t('homework.newTask.counts', {
            total: counts.automatic + counts.manual,
            automatic: counts.automatic,
            manual: counts.manual,
          })}
        </p>
      </div>
      {questions.length === 0 ? (
        <p className="text-small text-text-muted">{t('homework.newTask.noQuestions')}</p>
      ) : (
        <ol className="flex flex-col gap-3">
          {questions.map((question, index) => (
            <QuestionCard
              key={question.id}
              question={question}
              number={index + 1}
              message={(field) => problems(problemKey(question.id, field))}
              locked={locked}
              onChange={onChange}
              onRemove={() => {
                remove(index);
              }}
            />
          ))}
        </ol>
      )}
      {locked ? null : <AddQuestion typeSelect={typeSelect} disabled={full} onAdd={onAdd} />}
      <p role="status" className="text-small text-text">
        {t(`homework.newTask.results.${resultHintOf(counts)}`, { automatic: counts.automatic, manual: counts.manual })}
      </p>
    </section>
  );
}
