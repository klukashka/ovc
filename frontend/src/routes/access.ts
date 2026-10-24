import type { Me, SessionState } from '../lib/api/session.ts';
import { safeNext } from '../lib/navigation/safeNext.ts';

export type Audience = 'guest' | 'public' | 'reclaim' | 'tutor' | 'student' | 'shared' | 'unmatched';

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
export const reclaimAddress = '/reclaim';
const unconfirmedStudentSections: readonly string[] = ['/home', '/tutors', '/settings'];

export function homeOf(me: Me): string {
  return me.role === 'tutor' ? '/today' : '/home';
}

function confinedToConfirmation(me: Me): boolean {
  return me.role === 'student' && me.state === 'unverified';
}

function withinSection(pathname: string, section: string): boolean {
  return pathname === section || pathname.startsWith(`${section}/`);
}

export function reachable(me: Me, pathname: string): boolean {
  return !confinedToConfirmation(me) || unconfirmedStudentSections.some((section) => withinSection(pathname, section));
}

function anonymousRedirect(address: Address): string {
  return `/?${new URLSearchParams({ next: `${address.pathname}${address.search}` }).toString()}`;
}

function afterSignIn(me: Me, address: Address, origin: string): string {
  if (me.reclaim_pending) {
    return reclaimAddress;
  }
  const requested = address.pathname === '/' ? safeNext(new URLSearchParams(address.search).get('next'), origin) : null;
  return requested ?? homeOf(me);
}

function signedInAccess(me: Me, audience: Audience, address: Address, origin: string): Access {
  switch (audience) {
    case 'guest':
      return { kind: 'redirect', to: afterSignIn(me, address, origin) };
    case 'public':
      return { kind: 'public' };
    case 'reclaim':
      if (me.role !== 'student') {
        return { kind: 'notFound', me };
      }
      return reachable(me, address.pathname) ? { kind: 'signedIn', me } : { kind: 'redirect', to: confirmationAddress };
    case 'unmatched':
      return confinedToConfirmation(me) ? { kind: 'redirect', to: confirmationAddress } : { kind: 'notFound', me };
    case 'tutor':
    case 'student':
    case 'shared':
      if (!reachable(me, address.pathname)) {
        return { kind: 'redirect', to: confirmationAddress };
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
