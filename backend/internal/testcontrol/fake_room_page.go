package testcontrol

const fakeRoomPage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Fake meeting room</title>
    <link rel="icon" href="data:,">
    <link rel="stylesheet" href="/__test__/fake-room/room.css">
    <script src="/__test__/fake-room/room.js" defer></script>
  </head>
  <body>
    <main id="room" data-state="loading">
      <h1 id="room-name">Fake meeting room</h1>
      <p id="status" role="status">Connecting</p>
      <dl id="identity" hidden>
        <dt>Participant</dt>
        <dd id="participant-name"></dd>
        <dt>Role</dt>
        <dd id="participant-role"></dd>
      </dl>
      <button id="leave" type="button" hidden>Leave</button>
      <button id="enter" type="button" hidden>Enter again</button>
    </main>
  </body>
</html>
`

const fakeRoomStyle = `body {
  font-family: system-ui, sans-serif;
  margin: 0;
  padding: 1.5rem;
  background: #10151c;
  color: #eef2f7;
}

dt {
  font-weight: 600;
}

dd {
  margin: 0 0 0.75rem;
  overflow-wrap: anywhere;
}
`

const fakeRoomScript = `'use strict';
(function () {
  const refreshMilliseconds = 1000;
  const capabilityMaxLength = 256;
  const capability = window.location.hash.slice(1);
  const room = document.getElementById('room');
  const roomName = document.getElementById('room-name');
  const status = document.getElementById('status');
  const identity = document.getElementById('identity');
  const participantName = document.getElementById('participant-name');
  const participantRole = document.getElementById('participant-role');
  const leave = document.getElementById('leave');
  const enter = document.getElementById('enter');
  const messages = {
    entered: 'You are in the room',
    left: 'You left the room',
    ended: 'The meeting has ended',
    refused: 'This link does not open a room',
    unavailable: 'The provider is unavailable',
  };
  let admitted = false;
  let timer = null;
  let turn = 0;

  function stopRefreshing() {
    if (timer !== null) {
      window.clearTimeout(timer);
      timer = null;
    }
    turn += 1;
  }

  function show(state) {
    room.dataset.state = state;
    status.textContent = messages[state];
    leave.hidden = state !== 'entered';
    enter.hidden = state !== 'left';
    if (state === 'ended' || state === 'refused') {
      identity.hidden = true;
      roomName.textContent = 'Fake meeting room';
      participantName.textContent = '';
      participantRole.textContent = '';
    }
  }

  function showIdentity(view) {
    roomName.textContent = view.room_name;
    participantName.textContent = view.participant_name;
    participantRole.textContent = view.role;
    identity.hidden = false;
  }

  function apply(view) {
    switch (view.status) {
      case 'entered':
        admitted = true;
        showIdentity(view);
        show('entered');
        return true;
      case 'left':
        admitted = true;
        showIdentity(view);
        show('left');
        return false;
      case 'absent':
        show(admitted ? 'ended' : 'refused');
        return false;
      default:
        show('unavailable');
        return true;
    }
  }

  async function exchange(action) {
    const response = await window.fetch('/__test__/fake-room/' + action, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ capability: capability }),
      cache: 'no-store',
      credentials: 'omit',
      referrerPolicy: 'no-referrer',
    });
    if (!response.ok) {
      return { status: 'unavailable' };
    }
    return response.json();
  }

  async function step(action) {
    const current = turn;
    let view;
    try {
      view = await exchange(action);
    } catch (failure) {
      view = { status: 'unavailable' };
    }
    if (current !== turn) {
      return;
    }
    if (apply(view)) {
      timer = window.setTimeout(refresh, refreshMilliseconds);
    }
  }

  function refresh() {
    timer = null;
    void step(admitted ? 'state' : 'enter');
  }

  leave.addEventListener('click', function () {
    stopRefreshing();
    void step('leave');
  });
  enter.addEventListener('click', function () {
    stopRefreshing();
    void step('enter');
  });
  window.addEventListener('hashchange', function () {
    window.location.reload();
  });
  window.addEventListener('pagehide', stopRefreshing);

  if (capability === '' || capability.length > capabilityMaxLength) {
    show('refused');
  } else {
    void step('enter');
  }
})();
`
