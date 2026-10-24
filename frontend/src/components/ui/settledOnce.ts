import { useState } from 'react';

export function useSettledOnce(settled: boolean): boolean {
  const [settledBefore, setSettledBefore] = useState(settled);
  if (settled && !settledBefore) {
    setSettledBefore(true);
  }
  return settled || settledBefore;
}
