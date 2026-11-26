import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { type ApiClient, ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { type GroupDetail, type GroupMember, fetchGroup } from './api.ts';
import { AddBackNotice, ArchiveForm, MemberLabel, RemovalForm } from './GroupPage.tsx';
import { groupsTexts } from './texts.en.ts';

const origin = 'https://ovc.example';
const tutorEmail = 'pavel@example.org';
const password = 'a long password';

interface SeededGroup {
  readonly server: FakeAuthServer;
  readonly session: Session;
  readonly client: ApiClient;
  readonly group: GroupDetail;
}

interface Calls {
  readonly names: string[];
  readonly record: (name: string) => () => void;
}

function calls(): Calls {
  const names: string[] = [];
  return {
    names,
    record: (name) => () => {
      names.push(name);
    },
  };
}

async function seededGroup(members: readonly string[]): Promise<SeededGroup> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Pavel', email: tutorEmail, password });
  const { groupId } = server.seedGroup(tutorEmail, { name: 'Evening algebra', members });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(tutorEmail, password);
  const client = createApiClient(origin, session.authorizedFetch);
  return { server, session, client, group: await fetchGroup(client, groupId) };
}

async function show(seeded: SeededGroup, element: ReactElement): Promise<void> {
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { groups: groupsTexts }, true, true);
  await act(async () => {
    render(
      <SessionContext value={seeded.session}>
        <ApiClientContext value={seeded.client}>
          <QueryClientProvider client={new QueryClient()}>
            <I18nextProvider i18n={i18n}>{element}</I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>,
    );
    await Promise.resolve();
  });
}

function memberNamed(group: GroupDetail, name: string): GroupMember {
  const member = group.members.find((candidate) => candidate.display_name === name);
  if (member === undefined) {
    throw new Error(`no member named ${name}`);
  }
  return member;
}

async function showRemoval(seeded: SeededGroup, recorded: Calls): Promise<void> {
  await show(
    seeded,
    <RemovalForm
      groupId={seeded.group.id}
      groupName={seeded.group.name}
      member={memberNamed(seeded.group, 'Boris')}
      headingId="removal-heading"
      consequenceId="removal-consequence"
      cancelRef={{ current: null }}
      unavailable={false}
      onBusy={() => undefined}
      onRemoved={recorded.record('removed')}
      onChangedElsewhere={recorded.record('changedElsewhere')}
      onUnknownOutcome={recorded.record('unknownOutcome')}
      onCancel={recorded.record('cancel')}
    />,
  );
}

afterEach(() => {
  cleanup();
});

describe('the reversal of a group change whose answer is unknown', () => {
  test('a removal whose answer is lost reports an unknown outcome, so the page offers no Add back', async () => {
    const seeded = await seededGroup(['Anna', 'Boris']);
    const recorded = calls();
    await showRemoval(seeded, recorded);
    seeded.server.loseNextGroupWrite();

    fireEvent.click(screen.getByRole('button', { name: 'Remove from group' }));

    await waitFor(() => {
      expect(recorded.names).toEqual(['unknownOutcome']);
    });
    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(seeded.server.groupMemberIds(seeded.group.id)).toHaveLength(1);
  });

  test('a removal refused with 503 reports an unknown outcome and no removal, so the page offers no Add back', async () => {
    const seeded = await seededGroup(['Anna', 'Boris']);
    const recorded = calls();
    await showRemoval(seeded, recorded);
    seeded.server.failNextGroupWrite();

    fireEvent.click(screen.getByRole('button', { name: 'Remove from group' }));

    await screen.findByRole('alert');
    expect(recorded.names).toEqual(['unknownOutcome']);
    expect(seeded.server.groupMemberIds(seeded.group.id)).toHaveLength(2);
  });

  test('an archive whose answer is lost reports an unknown outcome, so the page offers no Undo', async () => {
    const seeded = await seededGroup(['Anna']);
    const recorded = calls();
    await show(
      seeded,
      <ArchiveForm
        group={seeded.group}
        cancelling={false}
        headingId="archive-heading"
        consequenceId="archive-consequence"
        cancelRef={{ current: null }}
        onBusy={() => undefined}
        onArchived={recorded.record('archived')}
        onChangedElsewhere={recorded.record('changedElsewhere')}
        onUnknownOutcome={recorded.record('unknownOutcome')}
        onCancel={recorded.record('cancel')}
      />,
    );
    seeded.server.loseNextGroupWrite();

    fireEvent.click(screen.getByRole('button', { name: 'Archive group' }));

    await waitFor(() => {
      expect(recorded.names).toEqual(['unknownOutcome']);
    });
    expect(await screen.findByRole('alert')).toBeTruthy();
  });

  test('an Add back whose answer is lost confirms after the refetch', async () => {
    const seeded = await seededGroup(['Anna', 'Boris']);
    const boris = memberNamed(seeded.group, 'Boris');
    await seeded.client.PATCH('/api/v1/groups/{id}', {
      params: { path: { id: seeded.group.id } },
      body: { remove: [boris.relationship_id] },
    });
    await show(
      seeded,
      <AddBackNotice
        groupId={seeded.group.id}
        relationshipId={boris.relationship_id}
        name="Boris"
        unavailable={false}
        onBusy={() => undefined}
        onChanged={() => undefined}
      />,
    );
    seeded.server.loseNextGroupWrite();

    fireEvent.click(screen.getByRole('button', { name: 'Add back' }));

    await screen.findByText(/^Back in the group:/);
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
    expect(seeded.server.groupMemberIds(seeded.group.id)).toHaveLength(2);
  });
});

describe('a member row', () => {
  test('a member whose card does not show the account shows only the card name', async () => {
    const seeded = await seededGroup(['Anna']);
    const hidden: GroupMember = { ...memberNamed(seeded.group, 'Anna'), name: null, avatar_id: null };

    await show(
      seeded,
      <MemoryRouter>
        <MemberLabel member={hidden} />
      </MemoryRouter>,
    );

    expect(screen.getByRole('link', { name: 'Anna' })).toBeDefined();
    expect(document.querySelector('.text-text-muted')).toBeNull();
  });

  test('a member whose card shows the account shows the account name under the card name', async () => {
    const seeded = await seededGroup(['Anna']);
    const shown = memberNamed(seeded.group, 'Anna');

    await show(
      seeded,
      <MemoryRouter>
        <MemberLabel member={shown} />
      </MemoryRouter>,
    );

    expect(screen.getByRole('link', { name: 'Anna' })).toBeDefined();
    expect(document.querySelector('.text-text-muted')?.textContent).toBe(shown.name);
  });
});
