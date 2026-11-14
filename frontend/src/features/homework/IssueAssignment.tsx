import { type ReactElement, type RefObject, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { ModuleLoadFailed } from '../../components/ui/LoadState.tsx';
import { type TextLoaders, useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { type IssueItem, issueAddress } from './api.ts';
import type { IssueDialog, IssuePreset } from './IssueDialog.tsx';

export const homeworkTextLoaders: TextLoaders<'homework'> = {
  ru: async () => (await import('./texts.ru.ts')).homeworkTexts,
  en: async () => (await import('./texts.en.ts')).homeworkTexts,
};

export type { IssuePreset, PresetLesson } from './IssueDialog.tsx';

type DialogComponent = typeof IssueDialog;

class DialogModule {
  private component: DialogComponent | null = null;
  private loading: Promise<DialogComponent> | null = null;

  loaded(): DialogComponent | null {
    return this.component;
  }

  start(): Promise<DialogComponent> {
    this.loading ??= import('./IssueDialog.tsx').then(
      (module) => {
        this.component = module.IssueDialog;
        return module.IssueDialog;
      },
      (error: unknown) => {
        this.loading = null;
        throw error;
      },
    );
    return this.loading;
  }
}

const dialogModule = new DialogModule();

export interface IssueDialogProps {
  readonly open: boolean;
  readonly preset: IssuePreset;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onIssued: (issue: IssueItem) => void;
}

export function IssueDialogHost(props: IssueDialogProps): ReactElement | null {
  const [Dialog, setDialog] = useState<DialogComponent | null>(() => dialogModule.loaded());
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!props.open || Dialog !== null) {
      return undefined;
    }
    let mounted = true;
    dialogModule.start().then(
      (component) => {
        if (mounted) {
          setFailed(false);
          setDialog(() => component);
        }
      },
      (error: unknown) => {
        console.error('dialog_load_failed', error instanceof Error ? error.name : typeof error);
        if (mounted) {
          setFailed(true);
        }
      },
    );
    return (): void => {
      mounted = false;
    };
  }, [props.open, Dialog]);

  if (Dialog === null) {
    return failed && props.open ? <ModuleLoadFailed /> : null;
  }
  return <Dialog {...props} />;
}

export function IssuedNotice({ issueId }: { readonly issueId: string | null }): ReactElement {
  const { t } = useTranslation();
  const element = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      element.current?.focus();
    });
    return (): void => {
      window.cancelAnimationFrame(frame);
    };
  }, []);

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

export function IssueAssignmentButton({
  preset,
  onIssued,
}: {
  readonly preset: IssuePreset;
  readonly onIssued: (issue: IssueItem) => void;
}): ReactElement {
  const { t } = useTranslation();
  const opener = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        ref={opener}
        type="button"
        onClick={() => {
          setOpen(true);
        }}
        className={quietButtonClass}
      >
        {t('homework.issue')}
      </button>
      <IssueDialogHost
        open={open}
        preset={preset}
        returnFocus={opener}
        onClose={() => {
          setOpen(false);
        }}
        onIssued={(issue) => {
          setOpen(false);
          onIssued(issue);
        }}
      />
    </>
  );
}

export function IssueAssignmentAction({
  preset,
  onIssued,
}: {
  readonly preset: IssuePreset;
  readonly onIssued: (issue: IssueItem) => void;
}): ReactElement | null {
  const texts = useFeatureTexts('homework', homeworkTextLoaders);
  return texts.ready ? <IssueAssignmentButton preset={preset} onIssued={onIssued} /> : null;
}

const tabClass = 'inline-flex min-h-11 items-center border-b-2 px-1 text-small';
const currentTabClass = `${tabClass} border-accent font-medium text-text`;
const otherTabClass = `${tabClass} border-transparent text-text-muted hover:text-text`;

export type HomeworkTab = 'library' | 'issued' | 'review';

const tabs: readonly { readonly key: HomeworkTab; readonly to: string }[] = [
  { key: 'library', to: '/homework/library' },
  { key: 'issued', to: '/homework/issued' },
  { key: 'review', to: '/homework/review' },
];

export function HomeworkTabs({ current }: { readonly current: HomeworkTab }): ReactElement {
  const { t } = useTranslation();
  return (
    <nav aria-label={t('section.homework')} className="border-b border-border">
      <ul className="flex flex-wrap gap-6">
        {tabs.map((entry) => (
          <li key={entry.key}>
            <Link
              to={entry.to}
              aria-current={entry.key === current ? 'page' : undefined}
              className={entry.key === current ? currentTabClass : otherTabClass}
            >
              {t(`homework.tabs.${entry.key}`)}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
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
