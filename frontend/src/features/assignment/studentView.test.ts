import { describe, expect, test } from 'vitest';

import type { components } from '../../lib/api/schema.ts';
import { parseStudentView } from './studentView.ts';

type StudentTemplateResponse = components['schemas']['StudentTemplate'];

const keyFields = {
  correct: true,
  accepted_answers: ['went'],
  value: '3.5',
  tolerance: '0.1',
  explanation: 'Because it is irregular.',
  points: 2,
  scoring: 'partial',
  max_points: 10,
  ready: true,
  problems: [],
  material_id: '6c1e0f8b-7a3d-4c55-9b21-5d2e8f4a6b70',
  attachment_id: '7a3d0f8b-6c1e-4c55-9b21-5d2e8f4a6b70',
};

function responseWithKeys(): StudentTemplateResponse {
  const picture = {
    filename: 'chart.png',
    media_type: 'image/png',
    size_bytes: 120,
    width: 40,
    height: 30,
    url: '/api/v1/files/7a3d0f8b-6c1e-4c55-9b21-5d2e8f4a6b70?t=signed',
    expires_at: '2026-11-10T10:10:00Z',
    ...keyFields,
  };
  const response = {
    title: 'Past simple',
    ...keyFields,
    blocks: [
      {
        id: 'b1',
        type: 'single_choice' as const,
        prompt: 'Pick one',
        images: [picture],
        options: [
          { id: 'o1', text: 'goed', ...keyFields },
          { id: 'o2', text: 'went', ...keyFields },
        ],
        ...keyFields,
      },
      { id: 'b2', type: 'material' as const, material: { ...picture, title: 'Verbs' }, ...keyFields },
    ],
  };
  return response;
}

function keysOf(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.flatMap(keysOf);
  }
  if (typeof value !== 'object' || value === null) {
    return [];
  }
  return Object.entries(value).flatMap(([key, nested]) => [key, ...keysOf(nested)]);
}

describe('the student view boundary', () => {
  test('only the student fields are copied, whatever else the response carries', () => {
    const response = responseWithKeys();

    const assignment = parseStudentView(response);

    const keys = new Set(keysOf(assignment));
    expect([...keys].filter((key) => key in keyFields)).toEqual([]);
    expect(JSON.stringify(assignment)).not.toContain('irregular');
    expect(assignment.blocks.map((block) => block.type)).toEqual(['single_choice', 'material']);
  });
});
