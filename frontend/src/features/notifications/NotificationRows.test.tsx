import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { Notification, NotificationEvent } from './api.ts';
import { type EventTextLoaders, NotificationList, NotificationTable } from './NotificationRows.tsx';
import { notificationEventTexts as englishEventTexts } from './texts.en.ts';
import { notificationEventTexts as russianEventTexts } from './texts.ru.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const student = { id: 'student-1', name: 'Anna', email: 'anna@example.org', password: 'a long password' };
const homeworkEvents: readonly NotificationEvent[] = ['issue.created', 'submission.checked', 'submission.returned'];

function homeworkRow(event: NotificationEvent, index: number): Notification {
  return {
    id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, '0')}`,
    event,
    created_at: new Date(Date.UTC(2026, 10, 1, 12, 0) - index * 60_000).toISOString(),
    read_at: null,
    label: `Past simple ${String(index + 1)}`,
    label_deleted: false,
    label_kind: null,
    colour: null,
    link: { kind: 'assignment', id: `00000000-0000-4000-9000-${String(index + 1).padStart(12, '0')}` },
  };
}

const rows = homeworkEvents.map(homeworkRow);

class ControlledLoaders {
  calls = 0;
  private settle: { resolve: () => void; reject: (reason: Error) => void } | null = null;

  readonly loaders: EventTextLoaders = {
    en: async () => {
      await this.pending();
      return englishEventTexts;
    },
    ru: async () => {
      await this.pending();
      return russianEventTexts;
    },
  };

  private pending(): Promise<void> {
    this.calls += 1;
    return new Promise((resolve, reject) => {
      const previous = this.settle;
      this.settle = {
        resolve: (): void => {
          previous?.resolve();
          resolve();
        },
        reject: (reason): void => {
          previous?.reject(reason);
          reject(reason);
        },
      };
    });
  }

  async load(): Promise<void> {
    await act(async () => {
      this.settle?.resolve();
      this.settle = null;
      await Promise.resolve();
    });
  }

  async fail(): Promise<void> {
    await act(async () => {
      this.settle?.reject(new Error('the chunk could not be loaded'));
      this.settle = null;
      await Promise.resolve();
    });
  }
}

async function showRows(list: ReactElement): Promise<void> {
  const server = new FakeAuthServer();
  server.addAccount({ ...student, role: 'student' });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(student.email, student.password);
  const router = createMemoryRouter(
    [
      {
        path: '/notifications',
        element: (
          <main>
            <h1 tabIndex={-1}>Notifications</h1>
            {list}
          </main>
        ),
      },
    ],
    {
      initialEntries: ['/notifications'],
    },
  );
  const i18n = await createI18n('en');
  render(
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <QueryClientProvider client={new QueryClient()}>
          <I18nextProvider i18n={i18n}>
            <RouterProvider router={router} />
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>
    </SessionContext>,
  );
  await screen.findAllByText('Past simple 1');
}

function listOf(loaders: ControlledLoaders): ReactElement {
  return (
    <NotificationList
      items={rows}
      label="Notifications"
      onChoose={() => undefined}
      focusIfLost={focusHeadingIfLost}
      textLoaders={loaders.loaders}
    />
  );
}

function rowLinkNames(): string[] {
  return screen.getAllByRole('link').map((link) => link.textContent);
}

describe('the notification rows before their event texts load', () => {
  test('a list of homework rows loads the texts once and names each row with the generic label meanwhile', async () => {
    const loaders = new ControlledLoaders();

    await showRows(listOf(loaders));

    expect(screen.getAllByRole('link', { name: /Homework/ })).toHaveLength(rows.length);
    expect(loaders.calls).toBe(2);
  });

  test('the table of the notifications page names each row with the generic label meanwhile', async () => {
    const loaders = new ControlledLoaders();

    await showRows(
      <NotificationTable
        items={rows}
        label="Notifications"
        bodyRef={{ current: null }}
        textLoaders={loaders.loaders}
      />,
    );

    expect(screen.getAllByRole('link', { name: 'Homework' })).toHaveLength(rows.length);
    expect(loaders.calls).toBe(2);
  });

  test('once the texts load, each row shows its own event text', async () => {
    const loaders = new ControlledLoaders();
    await showRows(listOf(loaders));

    await loaders.load();

    expect(rowLinkNames().map((name) => name.includes(englishEventTexts.issueCreated))).toEqual([true, false, false]);
    expect(screen.queryByRole('link', { name: /Homework/ })).toBeNull();
  });
});

describe('the notification rows after their event texts fail to load', () => {
  test('keep the generic label and offer a retry that loads the texts', async () => {
    const loaders = new ControlledLoaders();
    await showRows(listOf(loaders));
    await loaders.fail();
    const retry = screen.getByRole('button', { name: 'Try again' });

    fireEvent.click(retry);
    await loaders.load();

    expect(screen.getByRole('link', { name: new RegExp(englishEventTexts.submissionChecked) })).not.toBeNull();
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
    expect(loaders.calls).toBe(4);
  });

  test('show the load failure once for the whole list, with the rows still named', async () => {
    const loaders = new ControlledLoaders();
    await showRows(listOf(loaders));

    await loaders.fail();

    expect(screen.getAllByRole('alert')).toHaveLength(1);
    expect(screen.getAllByRole('button', { name: 'Try again' })).toHaveLength(1);
    expect(screen.getAllByRole('link', { name: /Homework/ })).toHaveLength(rows.length);
  });

  test('a retry that loads the texts moves focus to the heading once the failure is gone', async () => {
    const loaders = new ControlledLoaders();
    await showRows(listOf(loaders));
    await loaders.fail();
    const retry = screen.getByRole('button', { name: 'Try again' });
    retry.focus();

    fireEvent.click(retry);
    await loaders.load();

    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Notifications' }));
  });
});

describe('the label of a notification row', () => {
  test('a card whose name was erased is labelled with the fixed removed student text, not a name', async () => {
    const loaders = new ControlledLoaders();
    const removed: Notification = {
      ...homeworkRow('invitation.accepted', 1),
      label: null,
      label_kind: 'removed_student',
      link: null,
    };
    await showRows(
      <NotificationList
        items={[homeworkRow('issue.created', 0), removed]}
        label="Notifications"
        onChoose={() => undefined}
        focusIfLost={focusHeadingIfLost}
        textLoaders={loaders.loaders}
      />,
    );

    await loaders.load();

    expect(screen.getByText('Removed student')).toBeDefined();
  });

  test("a card whose name a student's purge erased is labelled with the fixed former student text", async () => {
    const loaders = new ControlledLoaders();
    const former: Notification = {
      ...homeworkRow('relationship.ended', 1),
      label: null,
      label_kind: 'former_student',
      link: null,
    };
    await showRows(
      <NotificationList
        items={[homeworkRow('issue.created', 0), former]}
        label="Notifications"
        onChoose={() => undefined}
        focusIfLost={focusHeadingIfLost}
        textLoaders={loaders.loaders}
      />,
    );

    await loaders.load();

    expect(screen.getByText('Former student')).toBeDefined();
  });
});
