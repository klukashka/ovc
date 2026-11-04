import { cancellationAfterNavigationAction } from '../e2e/request-failures.ts';
import { expect, test } from 'vitest';

import { AccountSwitchBoundary, NavigationReadiness, ownedApiPath } from '../e2e/navigation-readiness.ts';

const requiredQuery = '/api/v1/notifications';

function openDocument(state: NavigationReadiness, navigation: number, socket: number): void {
  state.navigate(navigation, null);
  state.commit(navigation);
  state.socketOpened(socket);
  state.socketReady(socket);
}

function finishQuery(state: NavigationReadiness, request: number): void {
  state.requestStarted(request, requiredQuery);
  state.response(request, 200);
  state.requestFinished(request);
}

test('a new document discards old pending work without inventing its terminal event', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);
  state.requestStarted(3, requiredQuery);

  openDocument(state, 4, 5);
  finishQuery(state, 6);

  expect(state.readyReceived()).toBe(true);
  expect(state.unsettled()).toEqual([]);
  expect(state.snapshot()).toEqual({ generation: 2, pending: 0, phase: 'committed' });
});

test('old ready and completed requests cannot satisfy a new document', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);
  state.requestStarted(3, requiredQuery);
  state.navigate(4, null);
  state.commit(4);

  state.socketReady(2);
  state.response(3, 200);
  state.requestFinished(3);

  expect(state.readyReceived()).toBe(false);
  expect(state.unsettled()).toEqual(['required_read_missing']);
  expect(state.snapshot().pending).toBe(0);
});

test('departing document requests and sockets cannot enroll before a new commit', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);
  state.navigate(3, null);

  const requestAccepted = state.requestStarted(4, requiredQuery);
  const socketAccepted = state.socketOpened(5);
  state.socketReady(5);
  state.response(4, 200);
  state.requestFinished(4);

  expect(requestAccepted).toBe(false);
  expect(socketAccepted).toBe(false);
  expect(state.readyReceived()).toBe(false);
  expect(state.snapshot().pending).toBe(0);
});

test('redirects keep one navigation generation and require its final commit', () => {
  const state = new NavigationReadiness([requiredQuery]);
  state.navigate(1, null);

  state.navigate(2, 1);
  state.navigate(3, 2);
  state.commit(3);
  state.socketOpened(4);
  state.socketReady(4);
  finishQuery(state, 5);

  expect(state.snapshot().generation).toBe(1);
  expect(state.unsettled()).toEqual([]);
});

test('a delayed redirect of another navigation cannot replace the current owner', () => {
  const state = new NavigationReadiness([requiredQuery]);
  state.navigate(1, null);
  state.navigate(2, null);

  const accepted = state.navigate(3, 1);
  state.commit(2);
  state.socketOpened(4);
  state.socketReady(4);
  finishQuery(state, 5);

  expect(accepted).toBe(false);
  expect(state.snapshot().generation).toBe(2);
  expect(state.unsettled()).toEqual([]);
});

test('a failed main navigation cannot be turned into a successful commit', () => {
  const state = new NavigationReadiness([requiredQuery]);
  state.navigate(1, null);

  state.navigationFailed(1);
  state.commit(1);
  state.socketOpened(2);
  state.socketReady(2);
  finishQuery(state, 3);

  expect(state.readyReceived()).toBe(false);
  expect(state.unsettled()).toContain('document_not_ready');
});

test('a same-document transition preserves pending work and socket ownership', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);
  state.requestStarted(3, requiredQuery);

  state.checkDocument();
  const checkingReady = state.readyReceived();
  state.sameDocument();
  const pending = state.unsettled();
  state.response(3, 200);
  state.requestFinished(3);

  expect(checkingReady).toBe(false);
  expect(pending).toEqual(['pending_api_request', 'required_read_missing']);
  expect(state.snapshot().generation).toBe(1);
  expect(state.readyReceived()).toBe(true);
  expect(state.unsettled()).toEqual([]);
});

test('a query completed before ready does not replace a required post-ready query', () => {
  const state = new NavigationReadiness([requiredQuery]);
  state.navigate(1, null);
  state.commit(1);
  state.socketOpened(2);

  finishQuery(state, 3);
  state.socketReady(2);

  expect(state.readyReceived()).toBe(true);
  expect(state.unsettled()).toEqual(['required_read_missing']);
});

for (const outcome of ['failed', 'http_failure', 'missing_terminal', 'missing_response'] as const) {
  test(`a current ${outcome} cannot create readiness`, () => {
    const state = new NavigationReadiness([requiredQuery]);
    openDocument(state, 1, 2);
    state.requestStarted(3, requiredQuery);

    if (outcome === 'failed') state.requestFailed(3);
    if (outcome === 'http_failure') {
      state.response(3, 503);
      state.requestFinished(3);
    }
    if (outcome === 'missing_terminal') state.response(3, 200);
    if (outcome === 'missing_response') state.requestFinished(3);

    expect(state.unsettled()).not.toEqual([]);
  });
}

test('a request from an earlier socket ready epoch cannot satisfy a reconnect', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);
  state.requestStarted(3, requiredQuery);

  state.socketOpened(4);
  state.socketReady(4);
  state.response(3, 200);
  state.requestFinished(3);

  expect(state.readyReceived()).toBe(true);
  expect(state.unsettled()).toEqual(['required_read_missing']);
});

test('repeated document changes never revive an earlier socket or query', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);
  state.requestStarted(3, requiredQuery);

  for (let generation = 1; generation <= 20; generation += 1) {
    openDocument(state, generation * 10, generation * 10 + 1);
    state.socketReady(2);
    state.response(3, 200);
    state.requestFinished(3);
  }
  finishQuery(state, 300);

  expect(state.snapshot()).toEqual({ generation: 21, pending: 0, phase: 'committed' });
  expect(state.unsettled()).toEqual([]);
});

test('idempotent disposal releases pending references and refuses further enrollment', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);
  state.requestStarted(3, requiredQuery);

  state.dispose();
  state.dispose();
  const requestAccepted = state.requestStarted(4, requiredQuery);
  const socketAccepted = state.socketOpened(5);
  state.socketReady(2);

  expect(requestAccepted).toBe(false);
  expect(socketAccepted).toBe(false);
  expect(state.snapshot().pending).toBe(0);
  expect(state.readyReceived()).toBe(false);
});

test.each([
  ['https://foreign.example/api/v1/notifications', true],
  ['https://ovc.example/api/v1/notifications', false],
  ['https://ovc.example:444/api/v1/notifications', true],
  ['http://ovc.example/api/v1/notifications', true],
] as const)('a foreign scope cannot enroll a readiness query: %s main=%s', (address, mainDocument) => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);

  const path = ownedApiPath(address, 'https://ovc.example/today', mainDocument);
  if (path !== null) state.requestStarted(3, path);
  state.response(3, 200);
  state.requestFinished(3);

  expect(path).toBeNull();
  expect(state.snapshot().pending).toBe(0);
  expect(state.unsettled()).toEqual(['required_read_missing']);
});

test('a same-origin main-document read retains its exact internal path', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);

  const path = ownedApiPath('https://ovc.example/api/v1/notifications', 'https://ovc.example/today', true);
  if (path !== null) state.requestStarted(3, path);
  state.response(3, 200);
  state.requestFinished(3);

  expect(path).toBe(requiredQuery);
  expect(state.unsettled()).toEqual([]);
});

test('a successful write cannot substitute for a required GET and still counts as pending work', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);

  state.requestStarted(3, requiredQuery, false);
  const pending = state.unsettled();
  state.response(3, 200);
  state.requestFinished(3);

  expect(pending).toEqual(['pending_api_request', 'required_read_missing']);
  expect(state.unsettled()).toEqual(['required_read_missing']);
});

test('readiness output contains no parameterized path or identifier', () => {
  const path = '/api/v1/channels/private-channel/messages';
  const state = new NavigationReadiness([path]);
  openDocument(state, 1, 2);

  state.requestStarted(3, path);
  const rendered = JSON.stringify(state.unsettled());

  expect(rendered).not.toContain(path);
  expect(rendered).not.toContain('private-channel');
  expect(state.unsettled()).toEqual(['pending_api_request', 'required_read_missing']);
});

test('diagnostic evidence preserves the first failure and exports counts without paths', () => {
  const state = new NavigationReadiness(['/api/v1/private/account-identifier']);
  openDocument(state, 1, 2);
  state.requestStarted(3, '/api/v1/private/account-identifier');

  state.requestFailed(3);
  state.fail('observer_operation_failed');

  expect(state.readyReceived()).toBe(false);
  expect(state.evidence()).toEqual({
    generation: 1,
    pending: 1,
    phase: 'failed',
    missing: 1,
    reason: 'request_failed',
    opens: 1,
    ready: 1,
    closes: 0,
  });
  expect(JSON.stringify(state.evidence())).not.toContain('account-identifier');
});

test('account switch starts once after its own successful logout response', () => {
  const boundary = new AccountSwitchBoundary();
  boundary.request(1, 'logout', true);
  boundary.response(1, 204);

  const opened = boundary.request(2, 'confirmation', true);

  expect(opened).toBe(true);
  expect(boundary.valid()).toBe(true);
});

for (const invalid of [
  'foreign_logout',
  'foreign_confirmation',
  'wrong_response',
  'wrong_status',
  'early_confirmation',
  'duplicate_logout',
  'duplicate_confirmation',
] as const) {
  test(`account switch rejects ${invalid} without opening another observation window`, () => {
    const boundary = new AccountSwitchBoundary();
    boundary.request(1, 'logout', invalid !== 'foreign_logout');

    if (invalid === 'duplicate_logout') boundary.request(2, 'logout', true);
    if (invalid !== 'early_confirmation')
      boundary.response(invalid === 'wrong_response' ? 9 : 1, invalid === 'wrong_status' ? 401 : 204);
    boundary.request(3, 'confirmation', invalid !== 'foreign_confirmation');
    if (invalid === 'duplicate_confirmation') boundary.request(4, 'confirmation', true);

    expect(boundary.valid()).toBe(false);
  });
}

test('coherent readiness refuses a new document even when the old document had ready', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);
  finishQuery(state, 3);

  state.navigate(4, null);
  state.commit(4);
  state.socketOpened(5);
  state.socketReady(5);
  finishQuery(state, 6);

  expect(state.coherent(false)).toBe(false);
  expect(state.coherent(true)).toBe(true);
});

test('coherent readiness requires a current socket after all reads complete', () => {
  const state = new NavigationReadiness([requiredQuery]);
  openDocument(state, 1, 2);
  finishQuery(state, 3);

  state.socketClosed(2);

  expect(state.coherent(true)).toBe(false);
});

test('retired request cancellation requires unanswered exact action ordering', () => {
  const failed = { description: 'GET', reason: 'net::ERR_ABORTED', answered: false, started: 1, failed: 3 };

  const outcomes = [
    cancellationAfterNavigationAction(failed, 2),
    cancellationAfterNavigationAction({ ...failed, answered: true }, 2),
    cancellationAfterNavigationAction({ ...failed, reason: 'other' }, 2),
    cancellationAfterNavigationAction(failed, 0),
    cancellationAfterNavigationAction(failed, 4),
  ];

  expect(outcomes).toEqual([true, false, false, false, false]);
});
