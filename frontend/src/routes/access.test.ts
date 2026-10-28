import { describe, expect, test } from 'vitest';

import type { Me, SessionState } from '../lib/api/session.ts';
import { type Access, type Address, type Audience, reachable, routeAccess } from './access.ts';

const origin = 'https://ovc.example';

function account(role: Me['role'], state: Me['state'], reclaimPending = false): Me {
  return {
    id: '00000000-0000-4000-8000-000000000001',
    name: 'A person',
    email: 'person@example.org',
    email_verified: state === 'active',
    timezone: 'Europe/Moscow',
    avatar_id: null,
    reclaim_pending: reclaimPending,
    role,
    state,
    language: 'en',
    theme: 'system',
  };
}

function signedIn(role: Me['role'], state: Me['state'] = 'active', reclaimPending = false): SessionState {
  return { kind: 'signedIn', me: account(role, state, reclaimPending) };
}

function at(pathname: string, search = ''): Address {
  return { pathname, search };
}

describe('route access before the session is known', () => {
  test.each<[SessionState, string]>([
    [{ kind: 'unknown' }, 'pending'],
    [{ kind: 'unavailable' }, 'unavailable'],
  ])('a session in state %j makes every address wait', (state, expected) => {
    const kinds = (['guest', 'public', 'tutor', 'student', 'shared', 'unmatched'] as const).map(
      (audience) => routeAccess(state, audience, at('/today'), origin).kind,
    );

    expect(new Set(kinds)).toEqual(new Set([expected]));
  });
});

describe('route access for anonymous visitors', () => {
  const anonymous: SessionState = { kind: 'signedOut', reason: 'anonymous' };

  test.each<Audience>(['tutor', 'student', 'shared'])('a private %s address redirects with next', (audience) => {
    const access = routeAccess(anonymous, audience, at('/schedule', '?week=3&view=list'), origin);

    expect(access).toEqual({ kind: 'redirect', to: '/?next=%2Fschedule%3Fweek%3D3%26view%3Dlist' });
  });

  test.each<SessionState>([
    { kind: 'signedOut', reason: 'signedOut' },
    { kind: 'signedOut', reason: 'blocked' },
    { kind: 'signedOut', reason: 'accountDeleted' },
  ])('a private address after an ended session redirects to sign-in without next', (state) => {
    const access = routeAccess(state, 'tutor', at('/today'), origin);

    expect(access).toEqual({ kind: 'redirect', to: '/' });
  });

  test.each<Audience>(['guest', 'public'])('a %s address is shown', (audience) => {
    const access = routeAccess(anonymous, audience, at('/'), origin);

    expect(access).toEqual({ kind: 'public' });
  });

  test('an unknown address is not found without a frame account', () => {
    const access = routeAccess(anonymous, 'unmatched', at('/no/such/page'), origin);

    expect(access).toEqual({ kind: 'notFound', me: null });
  });
});

describe('route access for signed-in accounts', () => {
  test.each<[Me['role'], Me['state'], string]>([
    ['tutor', 'active', '/today'],
    ['tutor', 'unverified', '/today'],
    ['student', 'active', '/home'],
    ['student', 'unverified', '/home'],
  ])('a %s (%s) at sign-in goes home to %s', (role, state, home) => {
    const access = routeAccess(signedIn(role, state), 'guest', at('/'), origin);

    expect(access).toEqual({ kind: 'redirect', to: home });
  });

  test.each<[string, string]>([
    ['?next=%2Fschedule%3Fweek%3D3', '/schedule?week=3'],
    ['?next=%2F%2Fevil.example', '/today'],
    ['?next=https%3A%2F%2Fevil.example%2Ftoday', '/today'],
    ['?next=%2F%5Cevil.example', '/today'],
  ])('sign-in with %s continues to %s', (search, expected) => {
    const access = routeAccess(signedIn('tutor'), 'guest', at('/', search), origin);

    expect(access).toEqual({ kind: 'redirect', to: expected });
  });

  test('next is honoured only on the sign-in address', () => {
    const access = routeAccess(signedIn('tutor'), 'guest', at('/signup', '?next=%2Fschedule'), origin);

    expect(access).toEqual({ kind: 'redirect', to: '/today' });
  });

  test.each<[Me['role'], Audience]>([
    ['tutor', 'student'],
    ['student', 'tutor'],
  ])('a %s opening a %s address sees not found in their own frame', (role, audience) => {
    const state = signedIn(role);

    const access = routeAccess(state, audience, at('/somewhere'), origin);

    expect(access).toEqual({ kind: 'notFound', me: state.kind === 'signedIn' ? state.me : null });
  });

  test.each<[Me['role'], Audience]>([
    ['tutor', 'tutor'],
    ['tutor', 'shared'],
    ['student', 'student'],
    ['student', 'shared'],
  ])('a %s opens a %s address in their own frame', (role, audience) => {
    const access = routeAccess(signedIn(role), audience, at('/somewhere'), origin);

    expect(access.kind).toBe('signedIn');
  });

  test('an unverified tutor keeps every tutor address', () => {
    const access = routeAccess(signedIn('tutor', 'unverified'), 'tutor', at('/students'), origin);

    expect(access.kind).toBe('signedIn');
  });

  test.each<[Audience, string]>([
    ['student', '/homework'],
    ['student', '/homework/00000000-0000-4000-8000-000000000003'],
    ['shared', '/schedule'],
    ['shared', '/lessons/00000000-0000-4000-8000-000000000004'],
    ['shared', '/messages'],
    ['shared', '/notifications'],
    ['student', '/homey'],
    ['tutor', '/students'],
  ])('an unverified student is sent from the %s address %s to the confirmation page', (audience, pathname) => {
    const access = routeAccess(signedIn('student', 'unverified'), audience, at(pathname), origin);

    expect(access).toEqual({ kind: 'redirect', to: '/verify-email' });
  });

  test.each<[Audience, string]>([
    ['student', '/home'],
    ['student', '/tutors'],
    ['shared', '/settings/profile'],
    ['shared', '/settings/account'],
    ['shared', '/settings/notifications'],
  ])('an unverified student reaches the %s address %s', (audience, pathname) => {
    const access = routeAccess(signedIn('student', 'unverified'), audience, at(pathname), origin);

    expect(access.kind).toBe('signedIn');
  });

  test.each<[string, boolean, Access['kind']]>([
    ['/schedule', true, 'signedIn'],
    ['/lessons/00000000-0000-4000-8000-000000000004', true, 'signedIn'],
    ['/messages', true, 'redirect'],
    ['/notifications', true, 'redirect'],
    ['/schedule', false, 'redirect'],
    ['/lessons/00000000-0000-4000-8000-000000000004', false, 'redirect'],
  ])(
    'an unverified student opening the shared address %s with a provisional link %s gets %s',
    (pathname, provisionalLink, expected) => {
      const state = signedIn('student', 'unverified');

      const access = routeAccess(state, 'shared', at(pathname), origin, provisionalLink);

      expect(access.kind).toBe(expected);
    },
  );

  test.each<[Me['role'], Me['state'], string, boolean]>([
    ['student', 'unverified', '/schedule', true],
    ['student', 'unverified', '/lessons', true],
    ['student', 'unverified', '/homework', false],
    ['student', 'unverified', '/messages', false],
  ])('with a provisional link, for a %s (%s) the address %s is reachable: %s', (role, state, pathname, expected) => {
    const me = account(role, state);

    const result = reachable(me, pathname, true);

    expect(result).toBe(expected);
  });

  test('an unverified student opening a tutor settings section sees not found, not the confirmation page', () => {
    const access = routeAccess(signedIn('student', 'unverified'), 'tutor', at('/settings/teaching'), origin);

    expect(access.kind).toBe('notFound');
  });

  test.each<[Me['role'], Me['state'], string, boolean]>([
    ['student', 'unverified', '/home', true],
    ['student', 'unverified', '/tutors', true],
    ['student', 'unverified', '/settings/account', true],
    ['student', 'unverified', '/schedule', false],
    ['student', 'unverified', '/notifications', false],
    ['student', 'active', '/schedule', true],
    ['tutor', 'unverified', '/schedule', true],
  ])('for a %s (%s) the address %s is reachable: %s', (role, state, pathname, expected) => {
    const me = account(role, state);

    const result = reachable(me, pathname);

    expect(result).toBe(expected);
  });

  test('an unverified student is sent from an unknown address to the confirmation page as from a known one', () => {
    const access = routeAccess(signedIn('student', 'unverified'), 'unmatched', at('/no/such/page'), origin);

    expect(access).toEqual({ kind: 'redirect', to: '/verify-email' });
  });

  test.each<Me['role']>(['tutor', 'student'])(
    'the invitation page stays public for a signed-in %s, in or out of a pending choice',
    (role) => {
      const states = [
        signedIn(role),
        signedIn(role, 'active', true),
        { kind: 'signedOut', reason: 'anonymous' } as const,
      ];

      const kinds = states.map((state) => routeAccess(state, 'public', at('/invite'), origin).kind);

      expect(kinds).toEqual(['public', 'public', 'public']);
    },
  );

  test('a public address stays public for a signed-in account', () => {
    const access = routeAccess(signedIn('tutor'), 'public', at('/verify-email'), origin);

    expect(access).toEqual({ kind: 'public' });
  });
});

describe('route access while a reclaim choice is pending', () => {
  test.each<[string, string]>([
    ['', '/reclaim'],
    ['?next=%2Fschedule', '/reclaim'],
  ])('sign-in with %j lands on the choice page', (search, expected) => {
    const access = routeAccess(signedIn('student', 'active', true), 'guest', at('/', search), origin);

    expect(access).toEqual({ kind: 'redirect', to: expected });
  });

  test('the choice page is shown in the signed-in frame', () => {
    const state = signedIn('student', 'active', true);

    const access = routeAccess(state, 'reclaim', at('/reclaim'), origin);

    expect(access).toEqual({ kind: 'signedIn', me: state.kind === 'signedIn' ? state.me : null });
  });

  test('other pages stay reachable during a pending choice', () => {
    const access = routeAccess(signedIn('student', 'active', true), 'shared', at('/schedule'), origin);

    expect(access.kind).toBe('signedIn');
  });

  test('without a pending choice a student still opens the choice page, which says nothing waits', () => {
    const state = signedIn('student');

    const access = routeAccess(state, 'reclaim', at('/reclaim'), origin);

    expect(access).toEqual({ kind: 'signedIn', me: state.kind === 'signedIn' ? state.me : null });
  });

  test('a tutor opening the choice page sees not found in their own frame', () => {
    const state = signedIn('tutor');

    const access = routeAccess(state, 'reclaim', at('/reclaim'), origin);

    expect(access).toEqual({ kind: 'notFound', me: state.kind === 'signedIn' ? state.me : null });
  });

  test('an unconfirmed student opening the choice page is sent to the confirmation page', () => {
    const access = routeAccess(signedIn('student', 'unverified'), 'reclaim', at('/reclaim'), origin);

    expect(access).toEqual({ kind: 'redirect', to: '/verify-email' });
  });

  test('an anonymous visitor of the choice page is sent to sign in with next', () => {
    const access = routeAccess({ kind: 'signedOut', reason: 'anonymous' }, 'reclaim', at('/reclaim'), origin);

    expect(access).toEqual({ kind: 'redirect', to: '/?next=%2Freclaim' });
  });
});
