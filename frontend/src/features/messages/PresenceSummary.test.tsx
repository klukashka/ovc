import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { manualScheduler } from '../../lib/api/fakeTransport.ts';
import { ChatSignals } from '../../lib/realtime/chatSignals.ts';
import type { Channel } from './api.ts';
import { ChatSignalsContext, useChatSignals } from './presence.ts';
import { PresenceSummary } from './PresenceSummary.tsx';
import { TypingIndicator } from './TypingIndicator.tsx';

const channel: Channel = {
  id: 'group',
  kind: 'group',
  colour: null,
  label: 'Group',
  label_kind: null,
  last_message: null,
  online: true,
  tutor: { name: 'Tutor', colour: null },
  unconfirmed: false,
  unread_count: 0,
  user_id: 'tutor',
  writable: true,
  members: [{ user_id: 'other', label: 'Private card name', online: true }],
};

afterEach(cleanup);

test('a student renders anonymous typing without a roster and expiration does not move focus', () => {
  const signals = new ChatSignals();
  const clock = manualScheduler();
  function View(): React.JSX.Element {
    const presence = useChatSignals(channel, 'student', 'student', 1);
    return (
      <>
        <input aria-label="Message" />
        <PresenceSummary presence={presence} group viewerRole="student" onlineLabel="Online" membersLabel="Members" />
        <TypingIndicator
          presence={presence}
          someoneTyping="Someone is typing"
          namedTyping={(name) => `${name} is typing`}
        />
      </>
    );
  }
  render(
    <SchedulerContext value={clock}>
      <ChatSignalsContext value={signals}>
        <View />
      </ChatSignalsContext>
    </SchedulerContext>,
  );
  const composer = screen.getByRole('textbox');
  composer.focus();

  act(() => {
    signals.publish({ type: 'typing', channelId: 'group', userId: null });
  });
  const typing = screen.getByText('Someone is typing');
  const dotClasses = typing.querySelector('span')?.className;
  act(() => {
    clock.pass(5_000);
  });

  expect(screen.queryByText('Private card name')).toBeNull();
  expect(screen.queryByRole('list', { name: 'Members' })).toBeNull();
  expect(dotClasses).toBe('motion-safe:animate-pulse');
  expect(screen.queryByText('Someone is typing')).toBeNull();
  expect(document.activeElement).toBe(composer);
});
