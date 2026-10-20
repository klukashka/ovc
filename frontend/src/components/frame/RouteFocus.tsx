import { type ReactElement, useEffect, useRef } from 'react';
import { Outlet, useLocation } from 'react-router';

export function RouteFocus(): ReactElement {
  const { pathname } = useLocation();
  const previous = useRef(pathname);

  useEffect(() => {
    if (previous.current === pathname) {
      return;
    }
    previous.current = pathname;
    document.querySelector<HTMLElement>('main h1')?.focus();
  }, [pathname]);

  return <Outlet />;
}
