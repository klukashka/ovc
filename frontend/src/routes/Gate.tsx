import type { ReactElement } from 'react';
import { Navigate, useLocation } from 'react-router';

import { PublicFrame } from '../components/frame/PublicFrame.tsx';
import { SessionPending, SessionUnavailable } from '../components/frame/SessionStatus.tsx';
import { SignedInFrame } from '../components/frame/SignedInFrame.tsx';
import { useSessionState } from '../lib/api/session.ts';
import { type Audience, routeAccess } from './access.ts';
import { NotFoundPage } from './NotFoundPage.tsx';

export function Gate({ audience }: { readonly audience: Audience }): ReactElement {
  const state = useSessionState();
  const { pathname, search } = useLocation();
  const access = routeAccess(state, audience, { pathname, search }, window.location.origin);
  switch (access.kind) {
    case 'pending':
      return <SessionPending />;
    case 'unavailable':
      return <SessionUnavailable />;
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
