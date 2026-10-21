import { useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router';

export function useAddressToken(): string | null {
  const location = useLocation();
  const navigate = useNavigate();
  const [token] = useState(() => new URLSearchParams(location.search).get('token'));

  useEffect(() => {
    if (new URLSearchParams(location.search).has('token')) {
      void navigate({ search: '' }, { replace: true });
    }
  }, [location.search, navigate]);

  return token === '' ? null : token;
}
