import { describe, expect, test } from 'vitest';

import { apiErrorFrom } from './client.ts';

function envelope(code: string): unknown {
  return { error: { code, message: 'Server message', request_id: 'request-1' } };
}

describe('API errors', () => {
  test('a known code keeps its code and request identifier', () => {
    const received = envelope('rate_limited');

    const error = apiErrorFrom(429, received);

    expect([error.status, error.code, error.requestId]).toEqual([429, 'rate_limited', 'request-1']);
  });

  test('an unknown code keeps the request identifier without claiming a code', () => {
    const received = envelope('code_from_a_newer_server');

    const error = apiErrorFrom(409, received);

    expect([error.status, error.code, error.requestId]).toEqual([409, undefined, 'request-1']);
  });

  test('validation problems keep their field without the body prefix and unknown codes become invalid', () => {
    const received = {
      error: {
        code: 'validation_failed',
        message: 'Server message',
        request_id: 'request-2',
        fields: [
          { path: 'body.new_password', code: 'too_short' },
          { path: 'body.email', code: 'code_from_a_newer_server' },
        ],
      },
    };

    const error = apiErrorFrom(422, received);

    expect([error.fieldCode('new_password'), error.fieldCode('email'), error.fieldCode('name')]).toEqual([
      'too_short',
      'invalid',
      undefined,
    ]);
  });

  test('a body that is not an error envelope keeps only the status', () => {
    const received = { error: { code: 'rate_limited' } };

    const error = apiErrorFrom(502, received);

    expect([error.status, error.code, error.requestId]).toEqual([502, undefined, undefined]);
  });

  test('a refusal naming relationships keeps their identifiers and drops entries that are not strings', () => {
    const received = {
      error: {
        code: 'relationship_not_active',
        message: 'Server message',
        request_id: 'request-3',
        details: { relationship_ids: ['relationship-1', 7, 'relationship-2'] },
      },
    };

    const error = apiErrorFrom(409, received);

    expect([error.code, error.relationshipIds]).toEqual([
      'relationship_not_active',
      ['relationship-1', 'relationship-2'],
    ]);
  });

  test('a refusal without relationship details names no relationships', () => {
    const received = {
      error: {
        code: 'student_exists',
        message: 'Server message',
        request_id: 'request-4',
        details: { student_id: 's' },
      },
    };

    const error = apiErrorFrom(409, received);

    expect([error.relationshipIds, error.existingStudentId]).toEqual([[], 's']);
  });
});
