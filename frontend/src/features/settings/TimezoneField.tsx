import { type KeyboardEvent, type ReactElement, useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useScheduler } from '../../app/clock.ts';
import { SelectChevron, fieldClass, selectClass } from '../../components/ui/Form.tsx';
import type { Language } from '../../lib/i18n/resources.ts';
import { matchingChoices, timezoneChoices } from './timezoneChoices.ts';

function keepFormFromSubmitting(event: KeyboardEvent<HTMLInputElement>): void {
  if (event.key === 'Enter') {
    event.preventDefault();
  }
}

export function TimezoneField({
  name,
  defaultValue,
  language,
  error,
  legendHidden = false,
}: {
  readonly name: string;
  readonly defaultValue: string;
  readonly language: Language;
  readonly error: string | undefined;
  readonly legendHidden?: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const scheduler = useScheduler();
  const searchId = useId();
  const legendId = useId();
  const emptyId = useId();
  const errorId = useId();
  const [openedAt] = useState(() => scheduler.wallClock());
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState(defaultValue);
  const choices = useMemo(() => timezoneChoices(defaultValue, language, openedAt), [defaultValue, language, openedAt]);
  const shown = matchingChoices(choices, search, selected);
  const nothingFound = search.trim() !== '' && shown.every((choice) => choice.zone === selected);
  const described = [nothingFound ? emptyId : null, error === undefined ? null : errorId]
    .filter((id) => id !== null)
    .join(' ');

  return (
    <fieldset className="flex w-full max-w-sm flex-col gap-1">
      <legend id={legendId} className={legendHidden ? 'sr-only' : 'mb-1 text-label'}>
        {t('settings.timezone')}
      </legend>
      <label htmlFor={searchId} className="sr-only">
        {t('settings.timezoneSearch')}
      </label>
      <input
        id={searchId}
        type="search"
        value={search}
        placeholder={t('settings.timezoneSearch')}
        autoComplete="off"
        onChange={(event) => {
          setSearch(event.target.value);
        }}
        onKeyDown={keepFormFromSubmitting}
        aria-describedby={nothingFound ? emptyId : undefined}
        className={fieldClass}
      />
      <div className="relative">
        <select
          aria-labelledby={legendId}
          name={name}
          value={selected}
          onChange={(event) => {
            setSelected(event.target.value);
          }}
          aria-invalid={error === undefined ? undefined : true}
          aria-describedby={described === '' ? undefined : described}
          className={selectClass}
        >
          {shown.map((choice) => (
            <option key={choice.zone} value={choice.zone}>
              {choice.label}
            </option>
          ))}
        </select>
        <SelectChevron />
      </div>
      <p id={emptyId} aria-live="polite" className={nothingFound ? 'text-small text-text-muted' : 'sr-only'}>
        {nothingFound ? t('settings.timezoneNothingFound') : null}
      </p>
      {error === undefined ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
    </fieldset>
  );
}
