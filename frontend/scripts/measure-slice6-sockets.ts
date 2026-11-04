import { observeBrowserContext, installPolicyObserver } from '../e2e/browser-diagnostics.ts';
import { performance } from 'node:perf_hooks';

import { chromium } from '@playwright/test';

import { type TestAccount, seedAccount, signInThroughTheApi } from '../e2e/accounts.ts';
import {
  type Deployment,
  finishRun,
  prepareRun,
  readE2ESettings,
  startDeployment,
  takeRunLock,
} from '../e2e/deployment.ts';
import { channelOf } from '../e2e/messages.ts';
import { accessToken, callApi, createGroup, seedActiveStudent } from '../e2e/tutoring.ts';
import { authFrame, outgoingTypingFrame, parseServerFrame, pingFrame } from '../src/lib/realtime/frames.ts';
import type { RealtimeSocket } from '../src/lib/realtime/socket.ts';
import {
  recordMeasurement,
  measurementSocketFactory,
  measurementScope,
  measurementCount,
  measurementSmoke,
  requiredMeasurementString,
  untilMeasurementCondition,
} from './slice6-measurement-support.ts';

interface SocketGeneration {
  readonly ordinal: number;
  closeRequestedAt: number | null;
  closedAt: number | null;
  readyAt: number | null;
}

interface Actor {
  readonly account: TestAccount;
  token: string;
  readonly channel: string;
  readonly group: number;
  socket: RealtimeSocket | null;
  closed: boolean;
  generation: SocketGeneration | null;
}

interface Delivery {
  readonly started: number;
  readonly recipients: Set<number>;
  readonly expected: number;
  readonly intended: ReadonlySet<number>;
  readonly generations: ReadonlyMap<number, SocketGeneration>;
  readonly recovered: Set<number>;
  readonly interrupted: Set<number>;
  messageId: string | null;
  maximum: number;
}

async function seed(deployment: Deployment): Promise<Actor[]> {
  const actors: Actor[] = [];
  for (let group = 0; actors.length < measurementCount(2000, 60); group += 1) {
    const tutor = await seedAccount(deployment, 'Measurement tutor', 'tutor', 'en');
    const members: { readonly account: TestAccount; readonly card: string }[] = [];
    for (let index = 0; index < 29 && actors.length + members.length + 1 < measurementCount(2000, 60); index += 1) {
      const member = await seedActiveStudent(deployment, tutor, `Measurement student ${String(index)}`, 'en');
      members.push({ account: member.student, card: member.cardId });
    }
    const groupId = await createGroup(
      deployment,
      tutor,
      'Measurement group',
      members.map((member) => member.card),
    );
    const channel = await channelOf(deployment, tutor, `/groups/${groupId}`);
    actors.push({ account: tutor, token: '', channel, group, socket: null, closed: true, generation: null });
    for (const member of members) {
      actors.push({
        account: member.account,
        token: '',
        channel,
        group,
        socket: null,
        closed: true,
        generation: null,
      });
    }
  }
  return actors;
}

async function sample(sampleNumber: number): Promise<void> {
  await measurementScope(async (cleanup) => {
    const deployment = await startDeployment(readE2ESettings(process.env), 0, `socket-${String(sampleNumber)}`, {
      synchronousCommit: 'on',
    });
    cleanup(() => deployment.stop());
    const actors: Actor[] = [];
    const deliveries = new Map<string, Delivery>();
    let frames = 0;
    let errors = 0;
    let unexpectedCloses = 0;
    let shuttingDown = false;
    const expectedCloses = new Set<number>();
    const churn: Promise<void>[] = [];
    let churnTimer: ReturnType<typeof setInterval> | undefined;
    let reconnects = 0;
    const browser = await chromium.launch();
    cleanup(() => browser.close());
    cleanup(async () => {
      shuttingDown = true;
      clearInterval(churnTimer);
      await measurementScope(async (socketCleanup) => {
        socketCleanup(() => untilMeasurementCondition(() => actors.every((actor) => actor.closed), 10_000));
        for (const actor of actors)
          socketCleanup(() => {
            actor.socket?.close(1000);
            return Promise.resolve();
          });
        const outcomes = await Promise.allSettled(churn);
        const problems = outcomes
          .filter((outcome) => outcome.status === 'rejected')
          .map((outcome) => outcome.reason as unknown);
        if (problems.length > 0) throw new AggregateError(problems, 'measurement churn cleanup failed');
      });
    });
    const peer = await deployment.startMeasurementPeer?.();
    if (peer === undefined) {
      throw new Error('measurement peer unavailable');
    }
    actors.push(...(await seed(deployment)));
    const connect = async (index: number): Promise<void> => {
      const actor = actors[index];
      if (actor === undefined) {
        throw new Error('measurement actor missing');
      }
      actor.token = await accessToken(deployment, actor.account);
      const host = index % 2 === 0 ? deployment : { ...deployment, apiUrl: peer.apiUrl };
      const state = { ready: false, closed: false, closeCode: 0 };
      actor.closed = false;
      const generation: SocketGeneration = {
        ordinal: (actor.generation?.ordinal ?? 0) + 1,
        closeRequestedAt: null,
        closedAt: null,
        readyAt: null,
      };
      actor.generation = generation;
      const socket = measurementSocketFactory(host)({
        onOpen: () => {
          socket.send(authFrame(actor.token));
        },
        onText: (text) => {
          frames += 1;
          const frame = parseServerFrame(text);
          if (frame?.type === 'ready') {
            state.ready = true;
            generation.readyAt = performance.now();
            recordMeasurement({
              event: 'measurement.socket_ready',
              sample: sampleNumber,
              actor: index,
              generation: generation.ordinal,
              readyAt: generation.readyAt,
            });
          } else if (frame?.type === 'error' || frame === null) {
            errors += 1;
          } else if (frame.type === 'message.new') {
            const delivery = deliveries.get(`${frame.channelId}:${String(frame.seq)}`);
            if (delivery?.intended.has(index)) {
              delivery.recipients.add(index);
              delivery.maximum = Math.max(delivery.maximum, performance.now() - delivery.started);
            }
          }
        },
        onBinary: () => {
          errors += 1;
        },
        onClose: (code) => {
          state.closed = true;
          generation.closedAt = performance.now();
          state.closeCode = code;
          recordMeasurement({
            event: 'measurement.socket_closed',
            sample: sampleNumber,
            actor: index,
            generation: generation.ordinal,
            closeRequestedAt: generation.closeRequestedAt ?? 'not_requested',
            closedAt: generation.closedAt,
            readyAt: generation.readyAt ?? 'not_ready',
            closeCode: code,
          });
          if (actor.socket === socket) actor.closed = true;
          if (!shuttingDown && !expectedCloses.has(index)) unexpectedCloses += 1;
        },
      });
      actor.socket = socket;
      await untilMeasurementCondition(() => state.ready || state.closed, 10_000);
      if (!state.ready)
        recordMeasurement({
          event: 'measurement.socket_refused',
          sample: sampleNumber,
          actor: index,
          closeCode: state.closeCode,
        });
      if (!state.ready) {
        throw new Error('measurement socket was refused');
      }
    };
    for (let index = 0; index < actors.length; index += 1) {
      await connect(index);
      if (index % 50 === 0) {
        for (const connected of actors.slice(0, index + 1)) connected.socket?.send(pingFrame('measurement_setup'));
      }
    }
    const contexts = await Promise.all(
      [0, measurementCount(1200, 30)].map(async (index) => {
        const actor = actors[index];
        if (actor === undefined) {
          throw new Error('browser observer actor missing');
        }
        const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
        const diagnostics = observeBrowserContext(context, 'chromium');
        await installPolicyObserver(context);
        const page = await context.newPage();
        await signInThroughTheApi(page, actor.account);
        await page.goto(`/messages/${actor.channel}`);
        await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
        return { context, page, actor, diagnostics };
      }),
    );
    const rendered: Promise<void>[] = [];
    const seq = new Map<string, number>();
    const started = performance.now();
    let sent = 0;
    let refused = 0;
    let heartbeatAt = started;
    const churnFirst = measurementCount(1700, 30);
    let churnCursor = churnFirst;
    churnTimer = setInterval(
      () => {
        const indices = Array.from({ length: 10 }, () => {
          const index = churnCursor;
          churnCursor = churnFirst + ((churnCursor - churnFirst + 1) % (actors.length - churnFirst));
          return index;
        });
        churn.push(
          Promise.allSettled(
            indices.map(async (index): Promise<void> => {
              const actor = actors[index];
              if (actor === undefined || expectedCloses.has(index)) {
                recordMeasurement({
                  event: 'measurement.reconnect_actor_failed',
                  sample: sampleNumber,
                  actor: index,
                  stage: 'select',
                  kind: 'actor_unavailable',
                });
                throw new Error('churn actor unavailable');
              }
              expectedCloses.add(index);
              const before = performance.now();
              let stage = 'close';
              try {
                if (actor.generation === null) throw new Error('churn generation missing');
                actor.generation.closeRequestedAt = performance.now();
                for (const delivery of deliveries.values()) {
                  if (delivery.generations.get(index) === actor.generation && !delivery.recipients.has(index))
                    delivery.interrupted.add(index);
                }
                actor.socket?.close(1000);
                await untilMeasurementCondition(() => actor.closed, 10_000);
                stage = 'authenticate';
                await connect(index);
                reconnects += 1;
                recordMeasurement({
                  event: 'measurement.reconnect',
                  sample: sampleNumber,
                  ordinal: reconnects,
                  durationMs: performance.now() - before,
                });
              } catch {
                recordMeasurement({
                  event: 'measurement.reconnect_actor_failed',
                  sample: sampleNumber,
                  actor: index,
                  stage,
                  kind: 'condition_or_connection_failed',
                });
                throw new Error('measurement reconnect actor failed');
              } finally {
                expectedCloses.delete(index);
              }
            }),
          ).then((outcomes) => {
            const failedActors = outcomes.filter((outcome) => outcome.status === 'rejected').length;
            if (failedActors > 0) {
              errors += 1;
              recordMeasurement({ event: 'measurement.reconnect_failed', sample: sampleNumber, failedActors });
            }
          }),
        );
      },
      measurementCount(5000, 1000),
    );
    while (performance.now() - started < measurementCount(90_000, 3000)) {
      const actor = actors[sent % actors.length];
      if (actor === undefined) {
        throw new Error('measurement sender missing');
      }
      const now = performance.now();
      if (now >= heartbeatAt) {
        for (const [index, candidate] of actors.entries()) {
          candidate.socket?.send(pingFrame('measurement'));
          if (index % 10 === 0) {
            candidate.socket?.send(outgoingTypingFrame(candidate.channel, true));
          }
        }
        heartbeatAt = now + 20_000;
      }
      const next = (seq.get(actor.channel) ?? 0) + 1;
      const intended = new Set(
        actors.flatMap((candidate, index) =>
          candidate.channel === actor.channel && !candidate.closed && !expectedCloses.has(index) ? [index] : [],
        ),
      );
      const delivery: Delivery = {
        started: performance.now(),
        recipients: new Set(),
        expected: intended.size,
        intended,
        generations: new Map(
          [...intended].map((index) => {
            const generation = actors[index]?.generation;
            if (generation === undefined || generation === null) throw new Error('intended generation missing');
            return [index, generation];
          }),
        ),
        interrupted: new Set(),
        recovered: new Set(),
        messageId: null,
        maximum: 0,
      };
      deliveries.set(`${actor.channel}:${String(next)}`, delivery);
      const messageBody = `Measurement ${String(sent)}`;
      const ordinal = sent;
      for (const observer of contexts.filter((candidate) => candidate.actor.channel === actor.channel)) {
        rendered.push(
          (async (): Promise<void> => {
            try {
              await observer.page
                .getByRole('list', { name: 'Messages', exact: true })
                .getByText(messageBody, { exact: true })
                .waitFor();
              recordMeasurement({
                event: 'measurement.render_delivery',
                sample: sampleNumber,
                ordinal,
                phase: delivery.started - started < measurementCount(30_000, 1000) ? 'warmup' : 'steady',
                durationMs: performance.now() - delivery.started,
              });
            } catch {
              errors += 1;
              recordMeasurement({ event: 'measurement.render_missing', sample: sampleNumber, ordinal });
            }
          })(),
        );
      }
      const answer = await callApi(deployment, actor.token, 'POST', `/channels/${actor.channel}/messages`, {
        body: messageBody,
        attachment_ids: [],
      });
      if (answer.status !== 201) {
        refused += 1;
        recordMeasurement({
          event: 'measurement.send_refused',
          sample: sampleNumber,
          status: answer.status,
          ordinal: sent,
        });
      } else {
        seq.set(actor.channel, next);
        delivery.messageId = requiredMeasurementString(answer.body, 'id');
        recordMeasurement({
          event: 'measurement.send',
          sample: sampleNumber,
          ordinal: sent,
          phase: now - started < measurementCount(30_000, 1000) ? 'warmup' : 'steady',
          durationMs: performance.now() - delivery.started,
        });
      }
      sent += 1;
      const nextAt = started + sent * 50;
      if (performance.now() < nextAt) {
        await new Promise<void>((resolve) => setTimeout(resolve, nextAt - performance.now()));
      }
    }
    clearInterval(churnTimer);
    churnTimer = undefined;
    await Promise.all(churn);
    const deliveryDeadline = performance.now() + 10_000;
    try {
      await untilMeasurementCondition(async () => {
        for (const [key, delivery] of deliveries) {
          const channel = key.slice(0, key.indexOf(':'));
          const sequence = Number(key.slice(key.indexOf(':') + 1));
          for (const index of delivery.intended) {
            if (delivery.recipients.has(index) || delivery.recovered.has(index)) continue;
            const original = delivery.generations.get(index);
            const actor = actors[index];
            if (
              !delivery.interrupted.has(index) ||
              original?.closedAt === null ||
              original?.closedAt === undefined ||
              actor?.generation?.readyAt === null ||
              actor?.generation?.readyAt === undefined ||
              actor.generation.ordinal <= original.ordinal
            )
              continue;
            const recoveryStarted = performance.now();
            const remaining = Math.floor(deliveryDeadline - recoveryStarted);
            if (remaining <= 0) throw new Error('measurement recovery deadline exceeded');
            const response = await fetch(
              `${deployment.apiUrl}/api/v1/channels/${channel}/messages?after_seq=${String(sequence - 1)}&limit=1`,
              {
                headers: { Authorization: `Bearer ${actor.token}`, Origin: deployment.origin },
                signal: AbortSignal.timeout(remaining),
              },
            );
            const body: unknown = await response.json();
            if (
              response.status !== 200 ||
              typeof body !== 'object' ||
              body === null ||
              !('items' in body) ||
              !Array.isArray(body.items) ||
              body.items.length !== 1 ||
              requiredMeasurementString(body.items[0], 'id') !== delivery.messageId
            )
              throw new Error('measurement recovery did not find intended message');
            if (performance.now() > deliveryDeadline) throw new Error('measurement recovery deadline exceeded');
            delivery.recovered.add(index);
            recordMeasurement({
              event: 'measurement.custom_rest_recovery',
              sample: sampleNumber,
              actor: index,
              originalGeneration: original.ordinal,
              generation: actor.generation.ordinal,
              ordinal: sequence,
              durationMs: performance.now() - recoveryStarted,
              productionRefetch: false,
            });
          }
        }
        return [...deliveries.values()].every((delivery) =>
          [...delivery.intended].every((index) => delivery.recipients.has(index) || delivery.recovered.has(index)),
        );
      }, 10_000);
    } catch {
      errors += 1;
      recordMeasurement({ event: 'measurement.delivery_deadline', sample: sampleNumber });
    }
    for (const [key, delivery] of deliveries) {
      for (const [actor, generation] of delivery.generations) {
        recordMeasurement({
          event: 'measurement.delivery_recipient',
          sample: sampleNumber,
          ordinal: key.slice(key.indexOf(':') + 1),
          actor,
          generation: generation.ordinal,
          closeRequestedAt: generation.closeRequestedAt ?? 'not_requested',
          closedAt: generation.closedAt ?? 'not_closed',
          readyAt: generation.readyAt ?? 'not_ready',
          frameReceived: delivery.recipients.has(actor),
          interrupted: delivery.interrupted.has(actor),
          recovered: delivery.recovered.has(actor),
        });
      }
      recordMeasurement({
        event: 'measurement.delivery',
        sample: sampleNumber,
        phase: delivery.started - started < measurementCount(30_000, 1000) ? 'warmup' : 'steady',
        ordinal: key.slice(key.indexOf(':') + 1),
        maximumRecipientMs:
          delivery.interrupted.size === 0 && delivery.recipients.size === delivery.expected
            ? delivery.maximum
            : 'incomplete',
        observedFrameMaximumMs: delivery.maximum,
        frameDeliveryComplete: delivery.interrupted.size === 0 && delivery.recipients.size === delivery.expected,
        interruptedRecipients: delivery.interrupted.size,
        recoveredRecipients: delivery.recovered.size,
        missingFrameRecipients: [...delivery.intended].filter((index) => !delivery.recipients.has(index)).length,
        missingRecipients: [...delivery.intended].filter(
          (index) => !delivery.recipients.has(index) && !delivery.recovered.has(index),
        ).length,
        recipients: delivery.recipients.size,
        disconnectedAtSend:
          actors.filter((actor) => actor.channel === key.slice(0, key.indexOf(':'))).length - delivery.expected,
        expected: delivery.expected,
      });
    }
    await Promise.all(rendered);
    for (const observer of contexts) {
      const before = performance.now();
      const count = await observer.page.getByRole('list', { name: 'Messages' }).getByRole('listitem').count();
      recordMeasurement({
        event: 'measurement.browser_observer',
        sample: sampleNumber,
        renderedRows: count,
        inspectMs: performance.now() - before,
        renderLatency: 'recorded_per_observed_message',
      });
      await observer.context.close();
      recordMeasurement({
        event: 'measurement.browser_diagnostics',
        consoleWarnings: observer.diagnostics.warnings(),
        consoleErrors: observer.diagnostics.errors(),
      });
      observer.diagnostics.verify();
    }
    recordMeasurement({
      event: 'measurement.socket_summary',
      sample: sampleNumber,
      sockets: actors.length,
      browserObservers: 2,
      apiProcesses: 2,
      frames,
      errors,
      unexpectedCloses,
      sent,
      refused,
      elapsedMs: performance.now() - started,
      restRefetchLatency: 'unverified',
      writerQueue: 'unverified',
      listenerQueue: 'unverified',
      reconnects,
      signOutCommitLatency: 'unverified',
    });
    if (errors !== 0 || refused !== 0 || unexpectedCloses !== 0) {
      throw new Error('measurement load has errors; retain this sample');
    }
  });
}

async function run(): Promise<void> {
  takeRunLock(process.env);
  try {
    prepareRun();
    recordMeasurement({
      event: 'measurement.protocol',
      mode: measurementSmoke ? 'smoke_not_performance_evidence' : 'full',
      seed: 'slice6-sockets-1',
      samples: measurementCount(3, 1),
      warmupSeconds: measurementCount(30, 1),
      steadySeconds: measurementCount(60, 2),
      messagesPerSecond: 20,
      messageLimitPerMinute: 30,
      lightweightSockets: measurementCount(2000, 60),
      browserObservers: 2,
      typingAccounts: measurementCount(200, 6),
      referenceCertification: false,
    });
    for (let index = 1; index <= measurementCount(3, 1); index += 1) {
      await sample(index);
    }
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'measurement.failed', kind: 'socket_run_failed' });
  process.exitCode = 1;
}
