import type { ChatSignal } from './client.ts';

export type ChatSignalEvent = ChatSignal | { readonly type: 'clear'; readonly channelId?: string };
export type TypingSender = (channelId: string, typing: boolean) => boolean;

export class ChatSignals {
  private sender: TypingSender | null = null;
  private readonly listeners = new Set<(event: ChatSignalEvent) => void>();

  subscribe(listener: (event: ChatSignalEvent) => void): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  connect(sender: TypingSender): void {
    this.sender = sender;
  }

  disconnect(): void {
    this.sender = null;
    this.clear();
  }

  clear(channelId?: string): void {
    this.publish({ type: 'clear', ...(channelId === undefined ? {} : { channelId }) });
  }

  publish(event: ChatSignalEvent): void {
    for (const listener of this.listeners) listener(event);
  }

  send(channelId: string, typing: boolean): boolean {
    return this.sender?.(channelId, typing) ?? false;
  }
}

export const chatSignals = new ChatSignals();
