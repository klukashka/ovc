import { Fragment, type ReactElement, createContext, useContext, useEffect, useState } from 'react';
import { Outlet, useNavigate, useParams } from 'react-router';

export const invitationAddress = '/invite';

export interface HeldInvitation {
  readonly token: string | null;
  readonly addressCleared: boolean;
}

const InvitationTokenContext = createContext<HeldInvitation>({ token: null, addressCleared: true });

export function InvitationTokenHolder(): ReactElement {
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

  return (
    <InvitationTokenContext value={{ token: held, addressCleared: token === undefined }}>
      <Fragment key={held ?? ''}>
        <Outlet />
      </Fragment>
    </InvitationTokenContext>
  );
}

export function useInvitationToken(): HeldInvitation {
  return useContext(InvitationTokenContext);
}
