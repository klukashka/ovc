import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import type { MyTutor } from '../tutors/api.ts';
import { TutorFilter } from './TutorFilter.tsx';

afterEach(() => {
  cleanup();
});

const names = ['Maria', 'Pavel', 'Olga', 'Ivan'] as const;
const refusedLink = '00000000-0000-4000-8000-000000000099';

function relationshipId(index: number): string {
  return `00000000-0000-4000-8000-00000000007${String(index)}`;
}

function tutors(count: number): MyTutor[] {
  return names.slice(0, count).map((name, index) => ({
    relationship_id: relationshipId(index),
    state: 'active',
    colour: 'moss',
    channel_id: null,
    account_closed_until: null,
    ended_at: null,
    ended_by: null,
    tutor: { name, avatar_id: null, subjects: ['English'], bio: '' },
  }));
}

interface ShownFilter {
  readonly choices: (string | null)[];
}

async function showFilter(listed: readonly MyTutor[], chosen: string | null): Promise<ShownFilter> {
  const i18n = await createI18n('en');
  const choices: (string | null)[] = [];
  render(
    <I18nextProvider i18n={i18n}>
      <TutorFilter
        tutors={listed}
        chosen={chosen}
        onChoose={(tutor) => {
          choices.push(tutor);
        }}
      />
    </I18nextProvider>,
  );
  return { choices };
}

async function showChangingFilter(
  listed: readonly MyTutor[],
  chosen: string | null,
): Promise<(next: readonly MyTutor[]) => void> {
  const i18n = await createI18n('en');
  const shown = (current: readonly MyTutor[]): ReactElement => (
    <I18nextProvider i18n={i18n}>
      <button type="button">Elsewhere</button>
      <TutorFilter tutors={current} chosen={chosen} onChoose={() => undefined} />
    </I18nextProvider>
  );
  const view = render(shown(listed));
  return (next: readonly MyTutor[]): void => {
    view.rerender(shown(next));
  };
}

function tutorSelect(): HTMLSelectElement {
  const select = screen.getByRole('combobox', { name: 'Tutor' });
  if (!(select instanceof HTMLSelectElement)) {
    throw new Error('the tutor filter is not a select');
  }
  return select;
}

function optionValues(): string[] {
  return Array.from(tutorSelect().options, (option) => option.value);
}

describe('the tutor filter', () => {
  test('three tutors make a segmented control of All and each tutor', async () => {
    const listed = tutors(3);

    await showFilter(listed, null);

    expect(screen.getByRole('group', { name: 'Tutor' })).toBeTruthy();
    expect(screen.queryByRole('combobox')).toBeNull();
    expect(screen.getAllByRole('button').map((button) => button.textContent)).toEqual([
      'All tutors',
      'Maria',
      'Pavel',
      'Olga',
    ]);
  });

  test('four tutors make a select whose values are exactly all and the relationship identifiers', async () => {
    const listed = tutors(4);

    await showFilter(listed, relationshipId(1));

    expect(screen.queryByRole('group', { name: 'Tutor' })).toBeNull();
    expect(optionValues()).toEqual(['all', ...listed.map((tutor) => tutor.relationship_id)]);
    expect(tutorSelect().value).toBe(relationshipId(1));
  });

  test('a chosen value outside the list shows a disabled placeholder without the value, so All stays a real choice', async () => {
    const listed = tutors(4);

    await showFilter(listed, refusedLink);

    expect(optionValues()).toEqual(['', 'all', ...listed.map((tutor) => tutor.relationship_id)]);
    expect(tutorSelect().value).toBe('');
    expect(tutorSelect().options[0]?.disabled).toBe(true);
    expect(document.body.innerHTML).not.toContain(refusedLink);
  });

  test('a chosen value outside the list adds no segment and presses none', async () => {
    const listed = tutors(3);

    await showFilter(listed, refusedLink);

    expect(screen.getAllByRole('button')).toHaveLength(4);
    expect(screen.queryAllByRole('button', { pressed: true })).toEqual([]);
    expect(document.body.innerHTML).not.toContain(refusedLink);
  });

  test('choosing All in the select emits null', async () => {
    const shown = await showFilter(tutors(4), relationshipId(2));

    fireEvent.change(tutorSelect(), { target: { value: 'all' } });

    expect(shown.choices).toEqual([null]);
  });

  test('choosing All in the segmented control emits null', async () => {
    const shown = await showFilter(tutors(3), relationshipId(2));

    fireEvent.click(screen.getByRole('button', { name: 'All tutors' }));

    expect(shown.choices).toEqual([null]);
  });
});

describe('the focus of the tutor filter when it changes form', () => {
  test('a fourth tutor turns the focused segments into the select, which takes the focus', async () => {
    const change = await showChangingFilter(tutors(3), relationshipId(1));
    screen.getByRole('button', { name: 'Pavel' }).focus();

    change(tutors(4));

    expect(document.activeElement).toBe(tutorSelect());
  });

  test('a tutor leaving turns the focused select into segments, and the pressed segment takes the focus', async () => {
    const change = await showChangingFilter(tutors(4), relationshipId(1));
    tutorSelect().focus();

    change(tutors(3));

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Pavel', pressed: true }));
  });

  test('a change of form while the focus is elsewhere leaves the focus where it was', async () => {
    const change = await showChangingFilter(tutors(3), null);
    screen.getByRole('button', { name: 'Elsewhere' }).focus();

    change(tutors(4));

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Elsewhere' }));
  });
});
