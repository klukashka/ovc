import type { VisibilitySource } from './visibility.ts';

export interface VisibilitySubject {
  readonly source: VisibilitySource;
  readonly change: () => void;
}

export type VisibilityScenario = (subject: VisibilitySubject) => void;

class VisibilityContractViolation extends Error {
  override readonly name = 'VisibilityContractViolation';
}

function check(condition: boolean, message: string): void {
  if (!condition) {
    throw new VisibilityContractViolation(message);
  }
}

export const visibilityContract: Readonly<Record<string, VisibilityScenario>> = {
  'a shown page reports itself visible': (subject) => {
    const visible = subject.source.visible();

    check(visible, 'expected a shown page to be visible');
  },

  'a subscriber hears each change until it unsubscribes': (subject) => {
    let heard = 0;
    const unsubscribe = subject.source.subscribe(() => {
      heard += 1;
    });

    subject.change();
    unsubscribe();
    subject.change();

    check(heard === 1, `expected one change heard before unsubscribing, got ${String(heard)}`);
  },
};
