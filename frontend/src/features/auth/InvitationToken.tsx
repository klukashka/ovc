import { Fragment, type ReactElement, createContext, useContext, useEffect, useState } from 'react';
import { Outlet, useNavigate, useParams } from 'react-router';

export const invitationAddress = '/invite';

export interface HeldInvitation {
  readonly token: string | null;
}

const InvitationTokenContext = createContext<HeldInvitation>({ token: null });

export function InvitationTokenHolder(): ReactElement | null {
  const { token } = useParams();
  const navigate = useNavigate();
  const [held, setHeld] = useState(() => (token === undefined || token === '' ? null : token));
  if (token !== undefined && token !== '' && token !== held) {
    setHeld(token);
  }

  useEffect(() => {
    if (token !== undefined) {
      void navigate(invitationAddress, { replace: true });
    }
  }, [token, navigate]);

  if (token !== undefined) {
    return null;
  }
  return (
    <InvitationTokenContext value={{ token: held }}>
      <Fragment key={held ?? ''}>
        <Outlet />
      </Fragment>
    </InvitationTokenContext>
  );
}

export function useInvitationToken(): HeldInvitation {
  return useContext(InvitationTokenContext);
}
