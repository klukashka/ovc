import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { NavLink } from 'react-router';

import { useSignedInAccount } from '../../lib/api/session.ts';

const sectionClass = 'inline-flex min-h-11 items-center rounded-sm px-3 text-small hover:bg-hover';

export function SettingsNav(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const sections = [
    { to: '/settings/profile', label: t('settings.profile') },
    { to: '/settings/account', label: t('settings.account') },
    { to: '/settings/notifications', label: t('settings.notifications') },
    ...(me.role === 'tutor' ? [{ to: '/settings/teaching', label: t('settings.teaching') }] : []),
    { to: '/settings/data', label: t('settings.myData') },
  ];
  return (
    <nav aria-label={t('settings.sections')} className="flex flex-wrap gap-1 border-b border-border pb-2">
      {sections.map((section) => (
        <NavLink
          key={section.to}
          to={section.to}
          className={({ isActive }) =>
            `${sectionClass} ${isActive ? 'bg-hover font-medium text-text' : 'text-text-muted'}`
          }
        >
          {section.label}
        </NavLink>
      ))}
    </nav>
  );
}
