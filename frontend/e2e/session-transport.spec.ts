import { authTransportContract } from '../src/lib/api/transportContract.ts';
import { httpAuthTransport } from '../src/lib/api/transport.ts';
import { testNetAddress } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { expect, seedVerifiedUser, test } from './fixtures.ts';
import { seedActiveStudent } from './tutoring.ts';
import { unsubscribeTokenFromAReminder } from './unsubscribe.ts';

const refreshCookie = '__Host-ovc_refresh';

function cookieJarFetch(deployment: Deployment): typeof fetch {
  let cookie: string | null = null;
  const address = testNetAddress();
  return async (input, init) => {
    const request = new Request(input, init);
    const headers = new Headers(request.headers);
    headers.set('Origin', deployment.origin);
    headers.set('X-Forwarded-For', address);
    if (cookie !== null) {
      headers.set('Cookie', `${refreshCookie}=${cookie}`);
    }
    const response = await fetch(new Request(request, { headers, signal: request.signal }));
    for (const setCookie of response.headers.getSetCookie()) {
      const [pair = '', ...attributes] = setCookie.split(';').map((part) => part.trim());
      const separator = pair.indexOf('=');
      const name = separator < 0 ? pair : pair.slice(0, separator);
      const value = separator < 0 ? '' : pair.slice(separator + 1);
      if (name !== refreshCookie) {
        continue;
      }
      const cleared = value === '' || attributes.some((attribute) => /^max-age=0$/i.test(attribute));
      cookie = cleared ? null : value;
    }
    return response;
  };
}

test.describe('the HTTP authentication transport against the real backend', () => {
  for (const [name, scenario] of Object.entries(authTransportContract)) {
    test(`keeps the contract: ${name}`, async ({ deployment, control }) => {
      const account = await seedVerifiedUser(deployment, 'Contract tutor');
      const otherTutor = await seedVerifiedUser(deployment, 'Another contract tutor');
      const subject = {
        transport: httpAuthTransport(deployment.apiUrl, cookieJarFetch(deployment)),
        origin: deployment.apiUrl,
        email: account.email,
        password: account.password,
        otherTutorEmail: otherTutor.email,
        otherTutorPassword: otherTutor.password,
        student: async (): Promise<{ readonly email: string; readonly password: string }> => {
          const { student } = await seedActiveStudent(deployment, otherTutor, 'Contract student');
          return { email: student.email, password: student.password };
        },
        expireAccessTokens: (): Promise<void> => control.advance(16 * 60),
        blockAccount: (): Promise<void> => {
          deployment.administer(['users', 'block', account.email]);
          return Promise.resolve();
        },
        unsubscribeToken: (): Promise<string> => unsubscribeTokenFromAReminder(deployment, control, account),
      };

      const run = scenario(subject);

      await expect(run).resolves.toBeUndefined();
    });
  }
});
