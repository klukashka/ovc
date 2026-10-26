const eventLoopTurnLimit = 100;

function nextEventLoopTurn(): Promise<void> {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, 0);
  });
}

export async function eventually(condition: () => void): Promise<void> {
  for (let turns = 0; ; turns += 1) {
    try {
      condition();
      return;
    } catch (error) {
      if (turns >= eventLoopTurnLimit) {
        throw error;
      }
    }
    await nextEventLoopTurn();
  }
}
