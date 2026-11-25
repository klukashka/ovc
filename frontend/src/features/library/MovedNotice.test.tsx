import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { type ReactElement, type ReactNode, useRef } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { useAction } from '../../components/ui/action.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { type Library, type LibraryFolder, folderTree } from './api.ts';
import { type LibraryTarget, MoveForm } from './LibraryDialogs.tsx';
import type { Moved } from './moveUndo.ts';
import { MovedNotice } from './MovedNotice.tsx';
import { libraryTexts } from './texts.en.ts';

const origin = 'https://ovc.example';
const tutorEmail = 'maria@example.org';
const password = 'a long password';

interface SeededShelf {
  readonly server: FakeAuthServer;
  readonly session: Session;
  readonly essay: LibraryTarget;
  readonly folders: readonly LibraryFolder[];
  readonly unitOne: string;
}

async function seededShelf(): Promise<SeededShelf> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Maria', email: tutorEmail, password });
  const seeded = server.seedLibrary(tutorEmail, { folderName: 'Unit 1', templateTitle: 'Essay' });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(tutorEmail, password);
  const root = (await (await session.authorizedFetch(`${origin}/api/v1/library`)).json()) as Library;
  const listed = (await (await session.authorizedFetch(`${origin}/api/v1/library/folders`)).json()) as {
    readonly folders: readonly LibraryFolder[];
  };
  const item = root.items.find((candidate) => candidate.id === seeded.templateId);
  if (item === undefined) {
    throw new Error('the seeded template is not in the root listing');
  }
  return { server, session, essay: { kind: 'item', item }, folders: listed.folders, unitOne: seeded.folderId };
}

async function shown(session: Session, child: ReactNode): Promise<void> {
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { library: libraryTexts }, true, true);
  await act(async () => {
    render(
      <SessionContext value={session}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <QueryClientProvider client={new QueryClient()}>
            <I18nextProvider i18n={i18n}>{child}</I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>,
    );
    await Promise.resolve();
  });
}

function MoveFormHarness({ shelf, moves }: { readonly shelf: SeededShelf; readonly moves: Moved[] }): ReactElement {
  const action = useAction();
  const picker = useRef<HTMLSelectElement>(null);
  return (
    <MoveForm
      target={shelf.essay}
      nodes={folderTree(shelf.folders)}
      headingId="move-heading"
      picker={picker}
      refusals={0}
      action={action}
      onCancel={() => undefined}
      onMovedTo={(moved) => {
        moves.push(moved);
      }}
    />
  );
}

function folderName(shelf: SeededShelf): (id: string | null) => string {
  return (id) => shelf.folders.find((folder) => folder.id === id)?.name ?? 'Library';
}

afterEach(() => {
  cleanup();
});

describe('the reversal of a library move whose answer is unknown', () => {
  test('a move whose answer is lost offers no Undo', async () => {
    const shelf = await seededShelf();
    const moves: Moved[] = [];
    await shown(shelf.session, <MoveFormHarness shelf={shelf} moves={moves} />);
    fireEvent.change(screen.getByRole('combobox', { name: 'Destination' }), { target: { value: shelf.unitOne } });
    shelf.server.loseNextLibraryMove();

    fireEvent.click(screen.getByRole('button', { name: 'Move' }));

    await screen.findByRole('alert');
    const folder = (await (
      await shelf.session.authorizedFetch(`${origin}/api/v1/library?folder=${shelf.unitOne}`)
    ).json()) as Library;
    expect(moves).toEqual([]);
    expect(folder.items.map((item) => item.title)).toEqual(['Essay']);
  });

  test('an Undo of a move whose answer is lost confirms after the refetch', async () => {
    const shelf = await seededShelf();
    const moves: Moved[] = [];
    await shown(shelf.session, <MoveFormHarness shelf={shelf} moves={moves} />);
    fireEvent.change(screen.getByRole('combobox', { name: 'Destination' }), { target: { value: shelf.unitOne } });
    fireEvent.click(screen.getByRole('button', { name: 'Move' }));
    await waitFor(() => {
      expect(moves).toHaveLength(1);
    });
    cleanup();
    const [moved] = moves;
    if (moved === undefined) {
      throw new Error('the move was not reported');
    }
    await shown(shelf.session, <MovedNotice moved={moved} folderName={folderName(shelf)} />);
    shelf.server.loseNextLibraryMove();

    fireEvent.click(screen.getByRole('button', { name: 'Undo' }));

    await screen.findByText(/^Moved back:/);
    expect(screen.queryByText('Can’t undo: the item changed since the move.')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
  });
});
