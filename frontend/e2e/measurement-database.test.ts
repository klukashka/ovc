import { describe, expect, test } from 'vitest';

import { assertMeasurementDatabase } from './deployment.ts';

describe('measurement database confinement', () => {
  test.each([
    'postgres',
    'ovc_dev',
    'ovc_test',
    'ovc_e2e_test',
    'ovc_e2e_abc_test;DROP DATABASE postgres',
    'ovc_e2e_ABC_test',
  ])('refuses %s before altering any database', (database) => {
    const configure = (): void => {
      assertMeasurementDatabase(database);
    };

    expect(configure).toThrow('newly owned end-to-end database');
  });

  test('accepts the generated end-to-end name', () => {
    const database = 'ovc_e2e_a12b34c56d78_test';

    const configure = (): void => {
      assertMeasurementDatabase(database);
    };

    expect(configure).not.toThrow();
  });
});
