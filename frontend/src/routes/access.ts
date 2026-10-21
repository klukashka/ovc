import type { Me, SessionState } from '../lib/api/session.ts';
import { safeNext } from '../lib/navigation/safeNext.ts';

export type Audience = 'guest' | 'public' | 'tutor' | 'student' | 'shared' | 'unmatched';

export interface Address {
  readonly pathname: string;
  readonly search: string;
}

export type Access =
  | { readonly kind: 'pending' }
  | { readonly kind: 'unavailable' }
  | { readonly kind: 'public' }
  | { readonly kind: 'signedIn'; readonly me: Me }
  | { readonly kind: 'notFound'; readonly me: Me | null }
  | { readonly kind: 'redirect'; readonly to: string };

const confirmationAddress = '/verify-email';
const confirmationStateAddresses: ReadonlySet<string> = new Set(['/settings/account']);

export function homeOf(me: Me): string {
  if (me.role === 'tutor') {
    return '/today';
  }
  return me.state === 'unverified' ? confirmationAddress : '/home';
}

function confinedToConfirmation(me: Me): boolean {
  return me.role === 'student' && me.state === 'unverified';
}

function anonymousRedirect(address: Address): string {
  return `/?${new URLSearchParams({ next: `${address.pathname}${address.search}` }).toString()}`;
}

function afterSignIn(me: Me, address: Address, origin: string): string {
  const requested = address.pathname === '/' ? safeNext(new URLSearchParams(address.search).get('next'), origin) : null;
  return requested ?? homeOf(me);
}

function signedInAccess(me: Me, audience: Audience, address: Address, origin: string): Access {
  switch (audience) {
    case 'guest':
      return { kind: 'redirect', to: afterSignIn(me, address, origin) };
    case 'public':
      return { kind: 'public' };
    case 'unmatched':
      return confinedToConfirmation(me) ? { kind: 'redirect', to: confirmationAddress } : { kind: 'notFound', me };
    case 'tutor':
    case 'student':
    case 'shared':
      if (confinedToConfirmation(me)) {
        return confirmationStateAddresses.has(address.pathname)
          ? { kind: 'signedIn', me }
          : { kind: 'redirect', to: confirmationAddress };
      }
      if (audience !== 'shared' && audience !== me.role) {
        return { kind: 'notFound', me };
      }
      return { kind: 'signedIn', me };
  }
}

export function routeAccess(state: SessionState, audience: Audience, address: Address, origin: string): Access {
  switch (state.kind) {
    case 'unknown':
      return { kind: 'pending' };
    case 'unavailable':
      return { kind: 'unavailable' };
    case 'signedOut':
      if (audience === 'guest' || audience === 'public') {
        return { kind: 'public' };
      }
      if (audience === 'unmatched') {
        return { kind: 'notFound', me: null };
      }
      return { kind: 'redirect', to: state.reason === 'anonymous' ? anonymousRedirect(address) : '/' };
    case 'signedIn':
      return signedInAccess(state.me, audience, address, origin);
  }
}
