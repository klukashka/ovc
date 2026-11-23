import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { type ReactElement, type SyntheticEvent, useRef } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import { ErrorSummary, type FieldProblem, namedField, useFieldProblems } from './FormProblems.tsx';

const fields = ['name', 'email', 'city', 'phone'] as const;

const requiredMessages: Readonly<Record<(typeof fields)[number], string>> = {
  name: 'Enter a name.',
  email: 'Enter an email address.',
  city: 'Enter a city.',
  phone: 'Enter a phone number.',
};

function clientProblem(field: string, value: string): string | undefined {
  const known = fields.find((candidate) => candidate === field);
  return known !== undefined && value.trim() === '' ? requiredMessages[known] : undefined;
}

function FourFieldForm({ serverRefusal }: { readonly serverRefusal: FieldProblem | null }): ReactElement {
  const form = useRef<HTMLFormElement>(null);
  const problems = useFieldProblems();

  function submit(event: SyntheticEvent<HTMLFormElement>): void {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const missing = fields.flatMap((field) => {
      const value = data.get(field);
      const message = clientProblem(field, typeof value === 'string' ? value : '');
      return message === undefined ? [] : [{ field, message, source: 'client' as const }];
    });
    problems.refuse(missing.length > 0 || serverRefusal === null ? missing : [serverRefusal]);
  }

  return (
    <main>
      <ErrorSummary
        attempt={problems.attempt}
        problems={problems.listed(fields).map((problem) => ({
          key: problem.field,
          message: problem.message,
          target: () => namedField(form.current, problem.field),
        }))}
      />
      <form
        ref={form}
        noValidate
        onSubmit={submit}
        onInput={(event) => {
          const target = event.target;
          if (target instanceof HTMLInputElement) {
            problems.edited(target.name, clientProblem(target.name, target.value));
          }
        }}
      >
        {fields.map((field) => (
          <div key={field}>
            <label htmlFor={field}>{field}</label>
            <input id={field} name={field} aria-invalid={problems.message(field) === undefined ? undefined : true} />
            {problems.message(field) === undefined ? null : <p data-inline={field}>{problems.message(field)}</p>}
          </div>
        ))}
        <button type="submit">Send</button>
      </form>
    </main>
  );
}

async function renderForm(serverRefusal: FieldProblem | null = null): Promise<void> {
  const i18n = await createI18n('en');
  render(
    <I18nextProvider i18n={i18n}>
      <FourFieldForm serverRefusal={serverRefusal} />
    </I18nextProvider>,
  );
}

function fill(field: string, value: string): void {
  fireEvent.input(screen.getByLabelText(field), { target: { value } });
}

async function submit(): Promise<void> {
  await act(async () => {
    screen.getByRole('button', { name: 'Send' }).click();
    await Promise.resolve();
  });
}

function inlineMessage(field: string): string | null {
  return document.querySelector(`[data-inline="${field}"]`)?.textContent ?? null;
}

afterEach(() => {
  cleanup();
});

describe('the error summary of a form with four fields', () => {
  test('a refused submit focuses the summary, which lists each problem with its inline wording', async () => {
    await renderForm();
    fill('name', 'Ann');

    await submit();

    const summary = screen.getByRole('alert', { name: 'Check these fields' });
    expect(document.activeElement).toBe(summary);
    expect(screen.getAllByRole('link').map((link) => link.textContent)).toEqual([
      'Enter an email address.',
      'Enter a city.',
      'Enter a phone number.',
    ]);
    expect(inlineMessage('city')).toBe('Enter a city.');
  });

  test('each summary link moves focus to its field', async () => {
    await renderForm();
    await submit();

    screen.getByRole('link', { name: 'Enter a city.' }).click();
    const cityFocused = document.activeElement;
    screen.getByRole('link', { name: 'Enter a phone number.' }).click();

    expect(cityFocused).toBe(screen.getByLabelText('city'));
    expect(document.activeElement).toBe(screen.getByLabelText('phone'));
  });

  test('an inline error disappears on the first valid input, before the next submit', async () => {
    await renderForm();
    await submit();

    fill('email', 'a');

    expect(inlineMessage('email')).toBeNull();
    expect(screen.getByLabelText('email').getAttribute('aria-invalid')).toBeNull();
    expect(screen.queryByRole('link', { name: 'Enter an email address.' })).toBeNull();
    expect(inlineMessage('city')).toBe('Enter a city.');
  });

  test('a server refusal stays until its field changes', async () => {
    await renderForm({ field: 'email', message: 'This address is already used.', source: 'server' });
    for (const field of fields) {
      fill(field, 'x');
    }
    await submit();

    fill('name', 'Anna');
    const afterOtherField = inlineMessage('email');
    fill('email', 'y');

    expect(afterOtherField).toBe('This address is already used.');
    expect(inlineMessage('email')).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  test('a second refused submit focuses the summary again', async () => {
    await renderForm();
    await submit();
    screen.getByRole('link', { name: 'Enter a name.' }).click();

    await submit();

    expect(document.activeElement).toBe(screen.getByRole('alert', { name: 'Check these fields' }));
  });
});
