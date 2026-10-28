import { type ReactElement, useEffect, useRef } from 'react';
import { Navigate, useLocation } from 'react-router';

import { PublicFrame } from '../components/frame/PublicFrame.tsx';
import { SessionPending, SessionUnavailable } from '../components/frame/SessionStatus.tsx';
import { SignedInFrame } from '../components/frame/SignedInFrame.tsx';
import { focusHeadingIfLost } from '../components/ui/LoadState.tsx';
import { useSession, useSessionState } from '../lib/api/session.ts';
import { type Access, type Audience, needsProvisionalLink, routeAccess } from './access.ts';
import { NotFoundPage } from './NotFoundPage.tsx';
import { type ProvisionalLink, provisionallyLinked, useProvisionalLink } from './provisionalLink.ts';

type GateAccess = Access | { readonly kind: 'linkUnavailable'; readonly retry: () => void };

function sessionSettled(access: GateAccess): boolean {
  return access.kind !== 'pending' && access.kind !== 'unavailable' && access.kind !== 'linkUnavailable';
}

function accessWithLink(access: Access, link: ProvisionalLink): GateAccess {
  if (access.kind !== 'redirect' && access.kind !== 'signedIn') {
    return access;
  }
  if (link.kind === 'pending') {
    return { kind: 'pending' };
  }
  if (link.kind === 'failed') {
    return { kind: 'linkUnavailable', retry: link.retry };
  }
  return access;
}

export function Gate({ audience }: { readonly audience: Audience }): ReactElement {
  const session = useSession();
  const state = useSessionState();
  const { pathname, search } = useLocation();
  const me = state.kind === 'signedIn' ? state.me : null;
  const link = useProvisionalLink(me);
  const routed = routeAccess(state, audience, { pathname, search }, window.location.origin, provisionallyLinked(link));
  const access = me !== null && needsProvisionalLink(me, pathname) ? accessWithLink(routed, link) : routed;
  const retried = useRef(false);
  const settled = sessionSettled(access);

  useEffect(() => {
    if (!settled || !retried.current) {
      return;
    }
    retried.current = false;
    focusHeadingIfLost();
  }, [settled]);

  switch (access.kind) {
    case 'pending':
      return <SessionPending />;
    case 'unavailable':
      return (
        <SessionUnavailable
          onRetry={() => {
            retried.current = true;
            void session.start();
          }}
        />
      );
    case 'linkUnavailable':
      return (
        <SessionUnavailable
          onRetry={() => {
            retried.current = true;
            access.retry();
          }}
        />
      );
    case 'redirect':
      return <Navigate to={access.to} replace />;
    case 'public':
      return <PublicFrame />;
    case 'signedIn':
      return <SignedInFrame me={access.me} />;
    case 'notFound':
      return access.me === null ? (
        <PublicFrame>
          <NotFoundPage />
        </PublicFrame>
      ) : (
        <SignedInFrame me={access.me} title="page.notFound">
          <NotFoundPage />
        </SignedInFrame>
      );
  }
}
