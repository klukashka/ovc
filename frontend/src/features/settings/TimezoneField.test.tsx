import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { type ManualScheduler, manualScheduler } from '../../lib/api/fakeTransport.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { TimezoneField } from './TimezoneField.tsx';
import { settingsTexts as englishSettingsTexts } from './settingsTexts.en.ts';
import { settingsTexts as russianSettingsTexts } from './settingsTexts.ru.ts';

interface SubmittedZones {
  readonly values: string[];
}

function schedulerAt(instant: number): ManualScheduler {
  const scheduler = manualScheduler();
  scheduler.pass(instant);
  return scheduler;
}

async function showField(language: Language, current: string): Promise<SubmittedZones> {
  const i18n = await createI18n(language);
  i18n.addResourceBundle('en', 'translation', { settings: englishSettingsTexts }, true, true);
  i18n.addResourceBundle('ru', 'translation', { settings: russianSettingsTexts }, true, true);
  const submitted: SubmittedZones = { values: [] };
  await act(async () => {
    render(
      <SchedulerContext value={schedulerAt(Date.UTC(2026, 10, 7))}>
        <I18nextProvider i18n={i18n}>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              const zone = new FormData(event.currentTarget).get('timezone');
              submitted.values.push(typeof zone === 'string' ? zone : '');
            }}
          >
            <TimezoneField name="timezone" defaultValue={current} language={language} error={undefined} />
            <button type="submit">Save</button>
          </form>
        </I18nextProvider>
      </SchedulerContext>,
    );
    await Promise.resolve();
  });
  return submitted;
}

function shownZones(select: HTMLSelectElement): string[] {
  return [...select.options].map((option) => option.value);
}

function offsetMinutesOf(label: string): number {
  const groups = /^UTC(?<sign>[+-])(?<hours>\d{2}):(?<minutes>\d{2}) · /.exec(label)?.groups;
  if (groups?.sign === undefined || groups.hours === undefined || groups.minutes === undefined) {
    throw new Error(`a label without an offset: ${label}`);
  }
  return (groups.sign === '-' ? -1 : 1) * (Number(groups.hours) * 60 + Number(groups.minutes));
}

afterEach(() => {
  cleanup();
});

describe('the timezone choice', () => {
  test('names a zone by its offset and its city in the nominative', async () => {
    await showField('ru', 'Europe/Moscow');

    const select = screen.getByLabelText<HTMLSelectElement>('Часовой пояс');

    expect(select.selectedOptions[0]?.textContent).toBe('UTC+03:00 · Москва');
    expect(select.value).toBe('Europe/Moscow');
  });

  test('a search narrows the list and the chosen zone is the one submitted', async () => {
    const submitted = await showField('ru', 'Europe/Moscow');
    const select = screen.getByLabelText<HTMLSelectElement>('Часовой пояс');

    fireEvent.change(screen.getByLabelText('Найти город или пояс'), { target: { value: 'Новосиб' } });
    fireEvent.change(select, { target: { value: 'Asia/Novosibirsk' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(shownZones(select)).toEqual(['Asia/Novosibirsk']);
    expect(submitted.values).toEqual(['Asia/Novosibirsk']);
  });

  test('the zone identifier finds a zone', async () => {
    await showField('en', 'Europe/Moscow');
    const select = screen.getByLabelText<HTMLSelectElement>('Timezone');

    fireEvent.change(screen.getByLabelText('Find a city or time zone'), { target: { value: 'america/new' } });

    expect(shownZones(select)).toContain('America/New_York');
    expect(shownZones(select).every((zone) => zone === 'Europe/Moscow' || zone.startsWith('America/New'))).toBe(true);
  });

  test('the English city finds a zone in the Russian interface', async () => {
    await showField('ru', 'Europe/Moscow');
    const select = screen.getByLabelText<HTMLSelectElement>('Часовой пояс');

    fireEvent.change(screen.getByLabelText('Найти город или пояс'), { target: { value: 'Novosibirsk' } });

    expect(shownZones(select)).toEqual(['Europe/Moscow', 'Asia/Novosibirsk']);
  });

  test('zones are ordered by offset and then by city', async () => {
    await showField('ru', 'Europe/Moscow');

    const select = screen.getByLabelText<HTMLSelectElement>('Часовой пояс');

    const offsets = [...select.options].map((option) => offsetMinutesOf(option.textContent));
    expect(offsets).toEqual([...offsets].sort((left, right) => left - right));
    expect(shownZones(select).indexOf('Europe/Moscow')).toBeLessThan(shownZones(select).indexOf('Europe/Istanbul'));
  });

  test('a saved zone this browser does not know is listed first by its identifier and stays chosen', async () => {
    const submitted = await showField('ru', 'Mars/Olympus');
    const select = screen.getByLabelText<HTMLSelectElement>('Часовой пояс');

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(select.options[0]?.value).toBe('Mars/Olympus');
    expect(select.options[0]?.textContent).toBe('Mars/Olympus');
    expect(submitted.values).toEqual(['Mars/Olympus']);
  });

  test('a search without matches keeps the chosen zone and says nothing was found', async () => {
    const submitted = await showField('ru', 'Europe/Moscow');
    const select = screen.getByLabelText<HTMLSelectElement>('Часовой пояс');

    fireEvent.change(screen.getByLabelText('Найти город или пояс'), { target: { value: 'нет такого города' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(shownZones(select)).toEqual(['Europe/Moscow']);
    expect(screen.getByText('Ничего не найдено. Выбранный пояс остаётся.').getAttribute('aria-live')).toBe('polite');
    expect(screen.queryAllByRole('status')).toEqual([]);
    expect(submitted.values).toEqual(['Europe/Moscow']);
  });

  test('Enter in the search field does not submit the form', async () => {
    const submitted = await showField('ru', 'Europe/Moscow');
    const search = screen.getByLabelText('Найти город или пояс');

    const allowed = fireEvent.keyDown(search, { key: 'Enter' });

    expect(allowed).toBe(false);
    expect(submitted.values).toEqual([]);
  });
});
