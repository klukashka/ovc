import { type ReactElement, type ReactNode, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { useFormResult, useOpenFormRoute, useReturnFocus } from '../../components/ui/formRoute.ts';
import { useFocusAfterReturn } from '../../components/ui/ReturnedNotice.tsx';
import { RouteTabs } from '../../components/ui/Tabs.tsx';
import { useSignedInAccount } from '../../lib/api/session.ts';
import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import { issueAddress, issueAssignmentAddress } from './api.ts';
import type { IssuePreset } from './IssueAssignmentPage.tsx';

export const homeworkTextLoaders: TextLoaders<'homework'> = {
  ru: async () => (await import('./texts.ru.ts')).homeworkTexts,
  en: async () => (await import('./texts.en.ts')).homeworkTexts,
};

export type { IssuePreset, PresetLesson } from './IssueAssignmentPage.tsx';

export function IssuedNotice({ issueId }: { readonly issueId: string | null }): ReactElement {
  const { t } = useTranslation();
  const element = useRef<HTMLParagraphElement>(null);
  useFocusAfterReturn(element);

  return (
    <div className="flex flex-wrap items-center gap-x-3">
      <p ref={element} role="status" tabIndex={-1} className="text-text outline-none">
        {t('homework.issued')}
      </p>
      {issueId === null ? null : (
        <Link to={issueAddress(issueId)} className={linkClass}>
          {t('homework.openIssued')}
        </Link>
      )}
    </div>
  );
}

export function useIssuedResult(): string | null {
  const result = useFormResult('assignmentIssued');
  return result?.issueId ?? null;
}

export function IssueAssignmentButton({
  opener,
  preset,
  templateId,
  unavailableReason = null,
  className = quietButtonClass,
  children,
}: {
  readonly opener: string;
  readonly preset?: IssuePreset;
  readonly templateId?: string;
  readonly unavailableReason?: string | null;
  readonly className?: string;
  readonly children?: ReactNode;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const open = useOpenFormRoute(me.id);
  const button = useRef<HTMLButtonElement>(null);
  useReturnFocus(opener, button);
  return (
    <button
      ref={button}
      type="button"
      aria-disabled={unavailableReason === null ? undefined : true}
      aria-describedby={unavailableReason ?? undefined}
      onClick={() => {
        if (unavailableReason === null) {
          open(issueAssignmentAddress(templateId), opener, preset);
        }
      }}
      className={className}
    >
      {children ?? t('homework.issue')}
    </button>
  );
}

export type HomeworkTab = 'library' | 'issued' | 'review';

const tabs: readonly { readonly key: HomeworkTab; readonly to: string }[] = [
  { key: 'library', to: '/homework/library' },
  { key: 'issued', to: '/homework/issued' },
  { key: 'review', to: '/homework/review' },
];

export function HomeworkTabs({ current }: { readonly current: HomeworkTab }): ReactElement {
  const { t } = useTranslation();
  return (
    <RouteTabs
      label={t('section.homework')}
      current={current}
      tabs={tabs.map((entry) => ({ ...entry, label: t(`homework.tabs.${entry.key}`) }))}
    />
  );
}

export function ConfirmEmailPrompt(): ReactElement {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-start gap-1">
      <p>{t('homework.confirmEmail')}</p>
      <Link to="/verify-email" className={linkClass}>
        {t('confirmationPrompt.action')}
      </Link>
    </div>
  );
}
