import { formatDate } from '../../lib/format/dates.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import type { Message } from './api.ts';

export const groupingWindowMilliseconds = 60_000;

export interface MessagePlace {
  readonly day: string;
  readonly firstOfDay: boolean;
  readonly firstOfGroup: boolean;
  readonly lastOfGroup: boolean;
}

type GroupedMessage = Pick<
  Message,
  'own' | 'sender_label' | 'sender_label_kind' | 'created_at' | 'sent_before_reclaim'
>;

function senderOf(message: GroupedMessage): string | null {
  if (message.own) return 'own';
  if (message.sender_label_kind !== null || message.sender_label === null) return null;
  return message.sender_label;
}

function sameSender(previous: GroupedMessage, next: GroupedMessage): boolean {
  const sender = senderOf(previous);
  return sender !== null && sender === senderOf(next) && previous.sent_before_reclaim === next.sent_before_reclaim;
}

function continues(previous: GroupedMessage, next: GroupedMessage, previousDay: string, nextDay: string): boolean {
  const gap = Date.parse(next.created_at) - Date.parse(previous.created_at);
  return previousDay === nextDay && sameSender(previous, next) && gap >= 0 && gap <= groupingWindowMilliseconds;
}

export function messagePlaces(
  messages: readonly GroupedMessage[],
  language: Language,
  timeZone: string,
): readonly MessagePlace[] {
  const days = messages.map((message) => formatDate(message.created_at, language, timeZone));
  return messages.map((message, index) => {
    const day = days[index] ?? '';
    const previous = messages[index - 1];
    const next = messages[index + 1];
    const previousDay = days[index - 1];
    const nextDay = days[index + 1];
    return {
      day,
      firstOfDay: previousDay !== day,
      firstOfGroup: previous === undefined || !continues(previous, message, previousDay ?? '', day),
      lastOfGroup: next === undefined || !continues(message, next, day, nextDay ?? ''),
    };
  });
}
