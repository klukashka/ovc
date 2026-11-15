import { describe, expect, test } from 'vitest';

import type { ActionState } from '../../components/ui/action.ts';
import { apiErrorFrom } from '../../lib/api/client.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { problemOf } from './issueProblems.ts';
import { homeworkTexts } from './texts.en.ts';

function refused(status: number, code: string, fields: readonly { path: string; code: string }[] = []): ActionState {
  return {
    kind: 'refused',
    error: apiErrorFrom(status, { error: { code, message: '', request_id: 'request-1', fields } }),
    retryAfter: status === 429 ? 30 : null,
  };
}

async function homeworkI18n(): Promise<Awaited<ReturnType<typeof createI18n>>> {
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { homework: homeworkTexts }, true, true);
  return i18n;
}

describe('issue dialog refusals', () => {
  test.each([
    ['an ended relationship', refused(409, 'relationship_not_active'), homeworkTexts.refusals.relationshipNotActive],
    ['an archived audience', refused(409, 'audience_not_active'), homeworkTexts.refusals.audienceNotActive],
    ['a template or audience no longer found', refused(404, 'not_found'), homeworkTexts.refusals.notFound],
    [
      'a snapshot over the storage limit',
      refused(422, 'validation_failed', [{ path: 'body', code: 'too_many' }]),
      homeworkTexts.refusals.storageLimit,
    ],
  ])('%s becomes the focused dialog alert', async (_name, state, message) => {
    const i18n = await homeworkI18n();

    const problem = problemOf(i18n.t, state, null);

    expect(problem).toEqual({ kind: 'alert', message, builderLink: null });
  });

  test('a rate limit becomes the focused dialog alert with the wait', async () => {
    const i18n = await homeworkI18n();
    const state = refused(429, 'rate_limited');

    const problem = problemOf(i18n.t, state, null);

    expect(problem?.kind).toBe('alert');
    expect(problem?.kind === 'alert' ? problem.message : '').not.toBe('');
  });
});
