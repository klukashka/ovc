import { Fragment, type ReactElement, createContext, useContext, useEffect, useState } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router';

interface HeldToken {
  readonly pathname: string;
  readonly token: string | null;
}

const AddressTokenContext = createContext<string | null>(null);

function tokenIn(search: string): string | null {
  return new URLSearchParams(search).get('token');
}

export function AddressTokenHolder(): ReactElement | null {
  const { pathname, search } = useLocation();
  const navigate = useNavigate();
  const carried = tokenIn(search);
  const [held, setHeld] = useState<HeldToken>({ pathname, token: carried });
  if (held.pathname !== pathname || (carried !== null && carried !== held.token)) {
    setHeld({ pathname, token: carried });
  }

  useEffect(() => {
    if (carried !== null) {
      void navigate({ search: '' }, { replace: true });
    }
  }, [carried, navigate]);

  if (carried !== null || held.pathname !== pathname) {
    return null;
  }
  const token = held.token === '' ? null : held.token;
  return (
    <AddressTokenContext value={token}>
      <Fragment key={token ?? ''}>
        <Outlet />
      </Fragment>
    </AddressTokenContext>
  );
}

export function useAddressToken(): string | null {
  return useContext(AddressTokenContext);
}
