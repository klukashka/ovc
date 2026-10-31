import { describe, expect, test } from 'vitest';

import { manualScheduler } from '../lib/api/fakeTransport.ts';
import { ConnectionState } from './connectionState.ts';

describe('connection state', () => {
  test('waits five uninterrupted seconds, including initial readiness', () => {
    const clock = manualScheduler();
    const state = new ConnectionState(clock);
    const changes: string[] = [];
    state.subscribe(() => changes.push(state.snapshot()));
    state.account(1);

    clock.pass(4999);
    const before = state.snapshot();
    state.readiness(1, false);
    clock.pass(1);
    state.readiness(1, false);
    clock.pass(5000);

    expect(before).toBe('waiting');
    expect(state.snapshot()).toBe('offline');
    expect(changes).toEqual(['offline']);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('a fast recovery cancels the grace without an announcement', () => {
    const clock = manualScheduler();
    const state = new ConnectionState(clock);
    const changes: string[] = [];
    state.subscribe(() => changes.push(state.snapshot()));
    state.account(1);
    state.readiness(1, true);

    state.readiness(1, false);
    clock.pass(4999);
    state.readiness(1, true);
    clock.pass(5000);

    expect(state.snapshot()).toBe('waiting');
    expect(changes).toEqual([]);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('announces each visible outage and recovery once', () => {
    const clock = manualScheduler();
    const state = new ConnectionState(clock);
    const changes: string[] = [];
    state.subscribe(() => changes.push(state.snapshot()));
    state.account(1);

    clock.pass(5000);
    state.readiness(1, true);
    state.readiness(1, true);
    state.readiness(1, false);
    clock.pass(5000);
    state.readiness(1, true);

    expect(changes).toEqual(['offline', 'recovered', 'offline', 'recovered']);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('account changes reject old readiness and reset the grace', () => {
    const clock = manualScheduler();
    const state = new ConnectionState(clock);
    state.account(1);
    clock.pass(5000);

    state.account(2);
    state.readiness(1, true);
    clock.pass(4999);
    const before = state.snapshot();
    clock.pass(1);
    state.readiness(1, true);

    expect(before).toBe('waiting');
    expect(state.snapshot()).toBe('offline');
  });

  test('sign-out clears timers and never announces recovery', () => {
    const clock = manualScheduler();
    const state = new ConnectionState(clock);
    const changes: string[] = [];
    state.subscribe(() => changes.push(state.snapshot()));
    state.account(1);
    clock.pass(5000);

    state.account(null);
    state.readiness(1, true);
    clock.pass(10000);

    expect(changes).toEqual(['offline', 'waiting']);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('page suspension cancels timers and resume starts one grace', () => {
    const clock = manualScheduler();
    const state = new ConnectionState(clock);
    state.account(1);
    clock.pass(4000);

    state.suspend();
    clock.pass(10000);
    const hidden = state.snapshot();
    state.resume();
    state.resume();
    clock.pass(4999);
    const before = state.snapshot();
    clock.pass(1);

    expect(hidden).toBe('waiting');
    expect(before).toBe('waiting');
    expect(state.snapshot()).toBe('offline');
    expect(clock.pendingDelays()).toEqual([]);
  });
});
