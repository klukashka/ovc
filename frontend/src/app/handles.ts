import type { ParseKeys } from 'i18next';
import { useMatches } from 'react-router';

export interface RouteHandle {
  readonly title: ParseKeys;
}

function isRouteHandle(value: unknown): value is RouteHandle {
  return typeof value === 'object' && value !== null && 'title' in value && typeof value.title === 'string';
}

export function useRouteTitle(): ParseKeys {
  const matches = useMatches();
  const handles = matches.map((match) => match.handle).filter(isRouteHandle);
  const deepest = handles.at(-1);
  return deepest === undefined ? 'page.notFound' : deepest.title;
}
