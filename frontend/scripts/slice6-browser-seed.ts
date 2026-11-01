import type { TestAccount } from '../e2e/accounts.ts';
import type { Deployment } from '../e2e/deployment.ts';
import { accessToken, callApi } from '../e2e/tutoring.ts';
import { measurementCount, requiredMeasurementString } from './slice6-measurement-support.ts';

const picture = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
);

export async function seedMeasurementPictures(
  deployment: Deployment,
  accounts: readonly TestAccount[],
  channel: string,
): Promise<void> {
  if (accounts.length !== 4) {
    throw new Error('the picture workload requires four independent uploaders');
  }
  const tokens: string[] = [];
  for (const account of accounts) {
    tokens.push(await accessToken(deployment, account));
  }
  for (let ordinal = 0; ordinal < measurementCount(200, 8); ordinal += 1) {
    if (ordinal === 100) {
      const advance = await fetch(`${deployment.apiUrl}/__test__/clock`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ advance_seconds: 61 }),
        signal: AbortSignal.timeout(10_000),
      });
      if (!advance.ok) {
        throw new Error('picture seed clock advancement failed');
      }
      await advance.arrayBuffer();
    }
    const token = tokens[ordinal % 4];
    if (token === undefined) {
      throw new Error('picture seed uploader missing');
    }
    const body = new FormData();
    body.append('file', new File([picture], 'measurement.png', { type: 'image/png' }));
    const uploaded = await fetch(`${deployment.apiUrl}/api/v1/uploads?context=chat&scope=channel:${channel}`, {
      method: 'POST',
      headers: { Origin: deployment.origin, Authorization: `Bearer ${token}` },
      body,
      signal: AbortSignal.timeout(30_000),
    });
    const answer: unknown = await uploaded.json();
    if (uploaded.status !== 201) {
      throw new Error('picture seed upload refused');
    }
    const id = requiredMeasurementString(answer, 'id');
    const message = await callApi(deployment, token, 'POST', `/channels/${channel}/messages`, {
      body: '',
      attachment_ids: [id],
    });
    if (message.status !== 201) {
      throw new Error('picture seed binding refused');
    }
  }
}
