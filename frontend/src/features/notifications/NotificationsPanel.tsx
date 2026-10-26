import { type ReactElement, useCallback, useEffect, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed, focusHeadingIfLost, focusIsLost } from '../../components/ui/LoadState.tsx';
import { asApiError } from '../../lib/api/client.ts';
import { type PanelProps, useLatestNotifications } from './Bell.tsx';
import { MarkAllRead, NotificationList } from './NotificationRows.tsx';

function PanelBody({
  onChoose,
  focusPanelIfLost,
}: {
  readonly onChoose: () => void;
  readonly focusPanelIfLost: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const query = useLatestNotifications(true);
  const showAll = (
    <Link to="/notifications" onClick={onChoose} className={linkClass}>
      {t('notifications.showAll')}
    </Link>
  );

  if (query.isPending) {
    return <DelayedLoading />;
  }
  if (query.isError) {
    return (
      <LoadFailed
        error={asApiError(query.error)}
        onRetry={() => {
          void query.refetch();
        }}
        focusIfLost={focusPanelIfLost}
      />
    );
  }
  const items = query.data.items;
  const newest = items[0];
  if (newest === undefined) {
    return (
      <>
        <p className="text-text-muted">{t('notifications.empty')}</p>
        {showAll}
      </>
    );
  }
  return (
    <>
      <MarkAllRead newest={newest} />
      <NotificationList items={items} label={t('notifications.list')} onChoose={onChoose} />
      {showAll}
    </>
  );
}

export function NotificationsPanel({ id, className, button, onClose }: PanelProps): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const panel = useRef<HTMLElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const focusPanelHeadingIfLost = useCallback(() => {
    if (focusIsLost()) {
      heading.current?.focus();
    }
  }, []);

  useEffect(() => {
    heading.current?.focus();
    const element = panel.current;
    function returnFocusIfLost(): void {
      requestAnimationFrame(() => {
        if (focusIsLost()) {
          button.current?.focus();
        }
      });
    }
    function closeOnOutsidePointer(event: PointerEvent): void {
      const target = event.target;
      if (target instanceof Node && element?.contains(target) !== true && button.current?.contains(target) !== true) {
        onClose();
        returnFocusIfLost();
      }
    }
    function closeOnEscape(event: KeyboardEvent): void {
      const target = event.target;
      const fromPanel =
        focusIsLost() ||
        (target instanceof Node && (element?.contains(target) === true || button.current?.contains(target) === true));
      if (event.key === 'Escape' && fromPanel) {
        event.preventDefault();
        event.stopPropagation();
        onClose();
        button.current?.focus();
      }
    }
    function closeOnFocusLeaving(event: FocusEvent): void {
      const target = event.relatedTarget;
      if (target instanceof Node && element?.contains(target) !== true && target !== button.current) {
        onClose();
      }
    }
    document.addEventListener('pointerdown', closeOnOutsidePointer);
    document.addEventListener('keydown', closeOnEscape);
    element?.addEventListener('focusout', closeOnFocusLeaving);
    return (): void => {
      document.removeEventListener('pointerdown', closeOnOutsidePointer);
      document.removeEventListener('keydown', closeOnEscape);
      element?.removeEventListener('focusout', closeOnFocusLeaving);
    };
  }, [button, onClose]);

  return (
    <section ref={panel} id={id} aria-labelledby={headingId} className={className}>
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('notifications.heading')}
      </h2>
      <PanelBody
        onChoose={() => {
          onClose();
          requestAnimationFrame(focusHeadingIfLost);
        }}
        focusPanelIfLost={focusPanelHeadingIfLost}
      />
    </section>
  );
}
