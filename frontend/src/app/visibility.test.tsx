import { describe, expect, test } from 'vitest';

import { controlledVisibility, documentVisibility } from './visibility.ts';
import { visibilityContract } from './visibilityContract.ts';

describe('the document visibility source', () => {
  test.each(Object.entries(visibilityContract))('keeps the contract: %s', (_name, scenario) => {
    const subject = {
      source: documentVisibility,
      change: (): void => {
        document.dispatchEvent(new Event('visibilitychange'));
      },
    };

    const run = (): void => {
      scenario(subject);
    };

    expect(run).not.toThrow();
  });
});

describe('the controlled visibility fake', () => {
  test.each(Object.entries(visibilityContract))('keeps the contract: %s', (_name, scenario) => {
    const fake = controlledVisibility();
    let hidden = false;
    const subject = {
      source: fake,
      change: (): void => {
        hidden = !hidden;
        if (hidden) fake.hide();
        else fake.show();
      },
    };

    const run = (): void => {
      scenario(subject);
    };

    expect(run).not.toThrow();
  });

  test('hiding and showing it is reported', () => {
    const fake = controlledVisibility();

    fake.hide();
    const whileHidden = fake.visible();
    fake.show();

    expect(whileHidden).toBe(false);
    expect(fake.visible()).toBe(true);
  });
});
