import type { Scheduler } from '../lib/api/session.ts';
import type { FollowingQueryClient } from './queryClient.ts';

export const resumeAfterBeforeUnloadMilliseconds = 2_000;

export type DocumentPhase = 'loading' | 'active' | 'departing' | 'hidden' | 'disposed';

export interface LifecyclePage {
  readonly events: EventTarget;
  readonly loaded: () => boolean;
}

export class DocumentLifecycle {
  private phase: DocumentPhase;
  private loaded: boolean;
  private cancelResume = (): void => undefined;
  private readonly listeners = new Set<(phase: DocumentPhase) => void>();
  private readonly page: LifecyclePage;
  private readonly scheduler: Scheduler;
  private readonly queries: Pick<FollowingQueryClient, 'suspendFollowUps' | 'resumeFollowUps'>;

  constructor(
    page: LifecyclePage,
    scheduler: Scheduler,
    queries: Pick<FollowingQueryClient, 'suspendFollowUps' | 'resumeFollowUps'>,
  ) {
    this.page = page;
    this.scheduler = scheduler;
    this.queries = queries;
    this.loaded = page.loaded();
    this.phase = this.loaded ? 'active' : 'loading';
    if (this.loaded) queries.resumeFollowUps();
    else queries.suspendFollowUps();
    page.events.addEventListener('load', this.onLoad);
    page.events.addEventListener('beforeunload', this.onBeforeUnload);
    page.events.addEventListener('pagehide', this.onPageHide);
    page.events.addEventListener('pageshow', this.onPageShow);
  }

  readonly snapshot = (): DocumentPhase => this.phase;

  readonly subscribe = (listener: (phase: DocumentPhase) => void): (() => void) => {
    if (this.phase === 'disposed') {
      listener(this.phase);
      return (): void => undefined;
    }
    this.listeners.add(listener);
    listener(this.phase);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  readonly dispose = (): void => {
    if (this.phase === 'disposed') return;
    this.cancelResume();
    this.page.events.removeEventListener('load', this.onLoad);
    this.page.events.removeEventListener('beforeunload', this.onBeforeUnload);
    this.page.events.removeEventListener('pagehide', this.onPageHide);
    this.page.events.removeEventListener('pageshow', this.onPageShow);
    this.publish('disposed');
    this.listeners.clear();
  };

  private publish(phase: DocumentPhase): void {
    this.phase = phase;
    if (phase === 'active') this.queries.resumeFollowUps();
    else this.queries.suspendFollowUps();
    for (const listener of this.listeners) listener(phase);
  }

  private readonly onLoad = (): void => {
    this.loaded = true;
    if (this.phase === 'loading') this.publish('active');
  };

  private readonly onBeforeUnload = (): void => {
    this.cancelResume();
    this.publish('departing');
    this.cancelResume = this.scheduler.schedule(resumeAfterBeforeUnloadMilliseconds, () => {
      this.cancelResume = (): void => undefined;
      this.publish(this.loaded ? 'active' : 'loading');
    });
  };

  private readonly onPageHide = (): void => {
    this.cancelResume();
    this.cancelResume = (): void => undefined;
    this.publish('hidden');
  };

  private readonly onPageShow = (event: Event): void => {
    if ('persisted' in event && event.persisted === true) {
      this.cancelResume();
      this.cancelResume = (): void => undefined;
      this.loaded = true;
      this.publish('active');
    }
  };
}
