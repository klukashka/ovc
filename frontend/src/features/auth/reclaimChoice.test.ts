import { describe, expect, test } from 'vitest';

import { type ReclaimList, linkField, reclaimChoice, requestField } from './reclaimChoice.ts';

const tutor = { name: 'Мария', avatar_id: null, subjects: [] };

const list: ReclaimList = {
  links: [{ relationship_id: 'link-1', tutor }],
  requests: [
    { request_id: 'request-1', tutor },
    { request_id: 'request-2', tutor },
  ],
  choose_by: '2026-11-01T12:00:00Z',
};

function answers(chosen: Readonly<Record<string, string>>): (field: string) => string | null {
  return (field) => chosen[field] ?? null;
}

describe('the reclaim choice', () => {
  test('names every waiting link and request exactly once, as kept or dropped', () => {
    const chosen = answers({
      [linkField('link-1')]: 'keep',
      [requestField('request-1')]: 'drop',
      [requestField('request-2')]: 'keep',
    });

    const choice = reclaimChoice(list, chosen);

    expect(choice).toEqual({
      keep_links: ['link-1'],
      drop_links: [],
      keep_requests: ['request-2'],
      drop_requests: ['request-1'],
    });
  });

  test('is not ready while any item has no decision', () => {
    const chosen = answers({ [linkField('link-1')]: 'keep', [requestField('request-1')]: 'drop' });

    const choice = reclaimChoice(list, chosen);

    expect(choice).toBeNull();
  });

  test('refuses a value that is neither keep nor drop', () => {
    const chosen = answers({
      [linkField('link-1')]: 'maybe',
      [requestField('request-1')]: 'drop',
      [requestField('request-2')]: 'drop',
    });

    const choice = reclaimChoice(list, chosen);

    expect(choice).toBeNull();
  });

  test('keeps a link and a request with the same identifier apart', () => {
    const shared: ReclaimList = {
      links: [{ relationship_id: 'same', tutor }],
      requests: [{ request_id: 'same', tutor }],
      choose_by: null,
    };

    const choice = reclaimChoice(shared, answers({ [linkField('same')]: 'drop', [requestField('same')]: 'keep' }));

    expect(choice).toEqual({ keep_links: [], drop_links: ['same'], keep_requests: ['same'], drop_requests: [] });
  });

  test('names an item listed twice only once', () => {
    const repeated: ReclaimList = { ...list, links: [...list.links, ...list.links], requests: [] };

    const choice = reclaimChoice(repeated, answers({ [linkField('link-1')]: 'keep' }));

    expect(choice).toEqual({ keep_links: ['link-1'], drop_links: [], keep_requests: [], drop_requests: [] });
  });
});
