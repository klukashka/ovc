import type { ReactElement, ReactNode } from 'react';

export interface IconProps {
  readonly size?: number;
}

function Outline({ size = 16, children }: IconProps & { readonly children: ReactNode }): ReactElement {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.3}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

export function ChevronDownIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M4 6l4 4 4-4" />
    </Outline>
  );
}

export function MoreIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <circle cx="3.5" cy="8" r="0.6" />
      <circle cx="8" cy="8" r="0.6" />
      <circle cx="12.5" cy="8" r="0.6" />
    </Outline>
  );
}

export function SunIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <circle cx="8" cy="8" r="3" />
      <path d="M8 1.5v1.5M8 13v1.5M1.5 8H3M13 8h1.5M3.4 3.4l1 1M11.6 11.6l1 1M3.4 12.6l1-1M11.6 4.4l1-1" />
    </Outline>
  );
}

export function HomeIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M2.5 7 8 2.5 13.5 7v6a.5.5 0 0 1-.5.5H9.5V10h-3v3.5H3a.5.5 0 0 1-.5-.5Z" />
    </Outline>
  );
}

export function CalendarIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <rect x="2" y="3" width="12" height="11" rx="1.5" />
      <path d="M2 6.5h12M5.5 1.5v3M10.5 1.5v3" />
    </Outline>
  );
}

export function PeopleIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <circle cx="6" cy="5.5" r="2.5" />
      <path d="M1.5 13.5c.5-2.3 2.3-3.5 4.5-3.5s4 1.2 4.5 3.5M10.5 3.2a2.5 2.5 0 0 1 0 4.6M12 10.3c1.3.5 2.2 1.6 2.5 3.2" />
    </Outline>
  );
}

export function GroupIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <circle cx="8" cy="4.5" r="2" />
      <circle cx="3.5" cy="9.5" r="1.7" />
      <circle cx="12.5" cy="9.5" r="1.7" />
      <path d="M5 14c.4-1.8 1.5-2.8 3-2.8s2.6 1 3 2.8" />
    </Outline>
  );
}

export function TaskIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <rect x="2.5" y="2" width="11" height="12" rx="1.5" />
      <path d="M5 6l1.2 1.2L8.5 5M5 10.5h6" />
    </Outline>
  );
}

export function ChatIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M2.5 3.5h11v7.5H7l-3 2.5V11H2.5Z" />
    </Outline>
  );
}

export function TutorIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M2 3.5h4.5a1.5 1.5 0 0 1 1.5 1.5v8.5a1.5 1.5 0 0 0-1.5-1.5H2ZM14 3.5H9.5A1.5 1.5 0 0 0 8 5v8.5A1.5 1.5 0 0 1 9.5 12H14Z" />
    </Outline>
  );
}

export function BellIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M4 11V7a4 4 0 0 1 8 0v4l1.2 1.5H2.8ZM6.5 14a1.6 1.6 0 0 0 3 0" />
    </Outline>
  );
}

export function SettingsIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M2.5 4h6M11.5 4h2M2.5 8h2M7.5 8h6M2.5 12h7M12.5 12h1" />
      <circle cx="10" cy="4" r="1.5" />
      <circle cx="6" cy="8" r="1.5" />
      <circle cx="11" cy="12" r="1.5" />
    </Outline>
  );
}

export function SidebarIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <rect x="2" y="2.5" width="12" height="11" rx="1.5" />
      <path d="M6 2.5v11" />
    </Outline>
  );
}

export function ChevronLeftIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M10 4l-4 4 4 4" />
    </Outline>
  );
}

export function ChevronRightIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M6 4l4 4-4 4" />
    </Outline>
  );
}

export function DashIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M4 8h8" />
    </Outline>
  );
}

export function CheckIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M5 13l4 4L19 7" />
    </Outline>
  );
}

export function CrossIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M4.5 4.5l7 7M11.5 4.5l-7 7" />
    </Outline>
  );
}

export function DoorIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M4 14V2.5h8V14M2.5 14h11" />
      <path d="M9.5 8.5v-1" />
    </Outline>
  );
}

export function AttachIcon(props: IconProps): ReactElement {
  return (
    <Outline {...props}>
      <path d="M13.5 7.5l-5.6 5.6a2.5 2.5 0 01-3.5-3.5l6-6a1.75 1.75 0 012.5 2.5l-6 6a1 1 0 01-1.4-1.4l5.3-5.3" />
    </Outline>
  );
}
