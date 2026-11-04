import assert from 'node:assert/strict';
import { performance } from 'node:perf_hooks';
import { MessageChannel } from 'node:worker_threads';

import type { AuthTransport } from '../src/lib/api/transport.ts';
import { recordMeasurement } from './slice6-measurement-support.ts';

interface LifecycleScheduler {
  readonly schedule: (milliseconds: number, task: () => void) => () => void;
  readonly wallClock: () => number;
}
interface LifecycleLocks {
  request<Result>(name: string, callback: () => Promise<Result>): Promise<Result>;
}
interface BodyControl {
  readonly headers: Promise<void>;
  readonly reading: Promise<void>;
  readonly release: () => void;
  readonly releaseHeaders: () => void;
  readonly snapshot: () => {
    readonly active: number;
    readonly aborted: number;
    readonly cancelled: number;
    readonly completed: number;
    readonly readerLocked: boolean;
  };
}
interface LifecycleSession {
  readonly signIn: (email: string, password: string) => Promise<{ readonly kind: 'signedIn' | 'refused' }>;
  readonly refresh: () => Promise<{ readonly kind: 'ended' | 'renewed' | 'superseded' }>;
  readonly reloadAccount: () => Promise<void>;
  readonly endRevoked: (reason: 'signedOut') => void;
  readonly snapshot: () => { readonly kind: 'signedIn' | 'signedOut' | 'unknown' | 'unavailable' };
}
interface SessionModule {
  readonly Session: new (dependencies: {
    readonly origin: string;
    readonly transport: AuthTransport;
    readonly locks: LifecycleLocks;
    readonly scheduler: LifecycleScheduler;
  }) => LifecycleSession;
  readonly ownedResponseBodyLimit: number;
}
interface AccountBodies {
  readonly arm: (body: BodyControl) => void;
}
interface FakeModule {
  readonly AccountResponseBodies: new () => AccountBodies;
  readonly SignalAwareRefreshBodies: new (options: {
    readonly hold?: boolean;
    readonly bytes?: number;
    readonly holdHeaders?: boolean;
  }) => BodyControl;
  readonly FakeAuthServer: new (options: {
    readonly refreshBodies?: BodyControl;
    readonly accountBodies?: AccountBodies;
  }) => {
    readonly addAccount: (account: {
      readonly id: string;
      readonly name: string;
      readonly email: string;
      readonly password: string;
    }) => void;
    readonly transport: () => AuthTransport;
    readonly revokeSessionsOf: (email: string) => void;
    readonly refreshes: number;
  };
  readonly memoryLocks: () => LifecycleLocks;
  readonly manualScheduler: () => LifecycleScheduler;
}

async function run(): Promise<void> {
  const sessions = (await import(
    new URL('../src/lib/api/session.ts', import.meta.url).href
  )) as unknown as SessionModule;
  const fakes = (await import(
    new URL('../src/lib/api/fakeTransport.ts', import.meta.url).href
  )) as unknown as FakeModule;
  const channel = new MessageChannel();
  let unexpectedMessages = 0;
  channel.port1.on('message', () => {
    unexpectedMessages += 1;
  });
  try {
    assert.equal(sessions.ownedResponseBodyLimit, 64 * 1024);
    for (const control of ['limit', 'overflow', 'deadline'] as const) {
      const bytes = control === 'overflow' ? 64 * 1024 + 1 : 64 * 1024;
      const body = new fakes.SignalAwareRefreshBodies({ hold: control === 'deadline', bytes });
      const server = new fakes.FakeAuthServer({ refreshBodies: body });
      const account = {
        id: 'lifecycle-owner',
        name: 'Synthetic lifecycle owner',
        email: 'lifecycle@example.org',
        password: 'synthetic lifecycle password',
      };
      server.addAccount(account);
      const session = new sessions.Session({
        origin: 'https://fixture.example',
        transport: server.transport(),
        locks: fakes.memoryLocks(),
        scheduler: fakes.manualScheduler(),
      });
      assert.equal((await session.signIn(account.email, account.password)).kind, 'signedIn');
      server.revokeSessionsOf(account.email);
      const started = performance.now();
      let failed = false;
      try {
        await session.refresh();
      } catch (error: unknown) {
        if (!(error instanceof AggregateError) || error.message !== 'refresh_refusal_body_failed')
          throw new Error('refresh_lifecycle_unexpected_failure');
        failed = true;
      } finally {
        body.release();
      }
      const state = body.snapshot();
      assert.equal(failed, control !== 'limit');
      assert.equal(session.snapshot().kind, 'signedOut');
      assert.equal(server.refreshes, 1);
      assert.equal(state.active, 0);
      assert.equal(state.readerLocked, false);
      if (control === 'deadline') assert.equal(state.aborted, 1);
      if (control === 'overflow') assert.equal(state.cancelled, 1);
      if (control === 'limit') assert.equal(state.completed, 1);
      recordMeasurement({
        event: 'session.refresh_response_lifetime',
        control,
        failed,
        bodyBytes: bytes,
        activeBodies: state.active,
        readerLocked: state.readerLocked,
        aborted: state.aborted,
        cancelled: state.cancelled,
        completed: state.completed,
        elapsedMilliseconds: Math.round(performance.now() - started),
      });
    }
    for (const owner of ['loadMe', 'ownerOf'] as const) {
      const bodies = new fakes.AccountResponseBodies();
      const body = new fakes.SignalAwareRefreshBodies({ hold: true, holdHeaders: true });
      const server = new fakes.FakeAuthServer({ accountBodies: bodies });
      const account = {
        id: 'account-lifecycle-owner',
        name: 'Synthetic account owner',
        email: 'account-lifecycle@example.org',
        password: 'synthetic account password',
      };
      server.addAccount(account);
      const session = new sessions.Session({
        origin: 'https://fixture.example',
        transport: server.transport(),
        locks: fakes.memoryLocks(),
        scheduler: fakes.manualScheduler(),
      });
      assert.equal((await session.signIn(account.email, account.password)).kind, 'signedIn');
      bodies.arm(body);
      const started = performance.now();
      const pending = (owner === 'loadMe' ? session.reloadAccount() : session.refresh()).then(
        () => ({ failed: false }),
        (error: unknown) => {
          if (!(error instanceof AggregateError) || error.message !== 'account_body_failed')
            throw new Error('account_lifecycle_unexpected_failure');
          return { failed: true };
        },
      );
      await body.headers;
      session.endRevoked('signedOut');
      body.releaseHeaders();
      let result: { readonly failed: boolean };
      try {
        result = await pending;
      } finally {
        body.release();
      }
      const state = body.snapshot();
      assert.equal(result.failed, true);
      assert.equal(state.active, 0);
      assert.equal(state.aborted, 1);
      assert.equal(state.readerLocked, false);
      assert.equal(session.snapshot().kind, 'signedOut');
      assert.equal(server.refreshes, owner === 'ownerOf' ? 1 : 0);
      recordMeasurement({
        event: 'session.account_response_lifetime',
        owner,
        control: 'deadline',
        failed: result.failed,
        activeBodies: state.active,
        readerLocked: state.readerLocked,
        aborted: state.aborted,
        elapsedMilliseconds: Math.round(performance.now() - started),
      });
    }
    assert.equal(unexpectedMessages, 0);
  } finally {
    channel.port1.close();
    channel.port2.close();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'session.refresh_response_lifetime_failed', kind: 'verification_failed' });
  process.exitCode = 1;
}
