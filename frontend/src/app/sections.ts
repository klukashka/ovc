import type { ReactElement } from 'react';

import {
  type IconProps,
  BellIcon,
  CalendarIcon,
  ChatIcon,
  GroupIcon,
  HomeIcon,
  PeopleIcon,
  SettingsIcon,
  SunIcon,
  TaskIcon,
  TutorIcon,
} from '../components/ui/icons.tsx';

export type Role = 'tutor' | 'student';

export type SectionKey =
  | 'today'
  | 'home'
  | 'schedule'
  | 'students'
  | 'groups'
  | 'homework'
  | 'messages'
  | 'tutors'
  | 'notifications'
  | 'settings';

export interface Section {
  readonly key: SectionKey;
  readonly to: string;
  readonly matches: readonly string[];
  readonly Icon: (props: IconProps) => ReactElement;
}

const notifications: Section = {
  key: 'notifications',
  to: '/notifications',
  matches: ['/notifications'],
  Icon: BellIcon,
};
const settings: Section = { key: 'settings', to: '/settings/profile', matches: ['/settings'], Icon: SettingsIcon };

export const roleSections: Readonly<Record<Role, readonly Section[]>> = {
  tutor: [
    { key: 'today', to: '/today', matches: ['/today'], Icon: SunIcon },
    { key: 'schedule', to: '/schedule', matches: ['/schedule', '/lessons'], Icon: CalendarIcon },
    { key: 'students', to: '/students', matches: ['/students'], Icon: PeopleIcon },
    { key: 'groups', to: '/groups', matches: ['/groups'], Icon: GroupIcon },
    { key: 'homework', to: '/homework/library', matches: ['/homework'], Icon: TaskIcon },
    { key: 'messages', to: '/messages', matches: ['/messages'], Icon: ChatIcon },
  ],
  student: [
    { key: 'home', to: '/home', matches: ['/home'], Icon: HomeIcon },
    { key: 'schedule', to: '/schedule', matches: ['/schedule', '/lessons'], Icon: CalendarIcon },
    { key: 'homework', to: '/homework', matches: ['/homework'], Icon: TaskIcon },
    { key: 'messages', to: '/messages', matches: ['/messages'], Icon: ChatIcon },
    { key: 'tutors', to: '/tutors', matches: ['/tutors'], Icon: TutorIcon },
  ],
};

export const trailingSections: readonly Section[] = [notifications, settings];

export function isCurrent(section: Section, pathname: string): boolean {
  return section.matches.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

export type SectionCounts = Readonly<Partial<Record<SectionKey, number>>>;

export function hasNews(counts: SectionCounts): boolean {
  return Object.values(counts).some((count) => count > 0);
}
