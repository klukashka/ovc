import type { components } from '../src/lib/api/schema.ts';
import type { TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { accountId } from './realtime.ts';

type EmailGroup = components['schemas']['EmailPreference']['group'];

export async function mintUnsubscribeToken(
  deployment: Deployment,
  account: TestAccount,
  group: EmailGroup,
): Promise<string> {
  const response = await fetch(`${deployment.apiUrl}/__test__/unsubscribe-token`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user: await accountId(deployment, account), group }),
  });
  if (!response.ok) {
    throw new Error(`/__test__/unsubscribe-token answered ${String(response.status)}`);
  }
  const body: unknown = await response.json();
  if (typeof body !== 'object' || body === null || !('token' in body) || typeof body.token !== 'string') {
    throw new Error('the unsubscribe token route answered an unexpected shape');
  }
  return body.token;
}
