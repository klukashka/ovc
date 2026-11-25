import { type ReactElement, type RefObject, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type Action, type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormActions,
  FormAlert,
  primaryButtonClass,
  quietButtonClass,
} from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { useApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { formatStorage } from '../settings/storageAmount.ts';
import type { LessonDetail } from './api.ts';
import { useMaterialTexts } from './materialTexts.ts';
import { MaterialPicker } from './MaterialPicker.tsx';
import {
  type LessonMaterial,
  type PickedMaterial,
  idsOf,
  materialAddress,
  opensAsPicture,
  pickedOf,
  sameMaterials,
} from './materials.ts';

export interface MaterialHandlers {
  readonly onSaved: (lesson: LessonDetail) => void;
  readonly onStale: () => void;
}

const staleCodes: ReadonlySet<string> = new Set(['conflict', 'not_found']);

function isStale(state: ActionState): boolean {
  return state.kind === 'refused' && state.error.code !== undefined && staleCodes.has(state.error.code);
}

function useMaterialsUpdate(
  lesson: LessonDetail,
  handlers: MaterialHandlers,
  onRefused: () => void,
): (action: Action, chosen: readonly PickedMaterial[]) => void {
  const client = useApiClient();
  return (action, chosen): void => {
    action.run(async (): Promise<ActionState> => {
      const {
        data: updated,
        error: problem,
        response,
      } = await client.POST('/api/v1/lessons/{id}/materials', {
        params: { path: { id: lesson.id } },
        body: { material_ids: idsOf(chosen), version: lesson.version },
      });
      if (updated === undefined) {
        const refused = refusedBy(response, problem);
        if (isStale(refused)) {
          handlers.onStale();
          return refused;
        }
        onRefused();
        return refused;
      }
      handlers.onSaved(updated);
      return done;
    });
  };
}

function rowKey(material: LessonMaterial): string {
  return material.material_id ?? material.attachment_id;
}

function MaterialRow({
  material,
  me,
  detach,
}: {
  readonly material: LessonMaterial;
  readonly me: Me;
  readonly detach: { readonly busy: boolean; readonly unavailable: boolean; readonly onDetach: () => void } | null;
}): ReactElement {
  const { t } = useTranslation();
  const texts = useMaterialTexts();
  const titleId = useId();
  const detachButton = useRef<HTMLButtonElement>(null);
  const address = materialAddress(material, window.location.origin);
  const picture = opensAsPicture(material);
  return (
    <li
      data-row={rowKey(material)}
      className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-b border-border py-2"
    >
      <span className="flex min-w-0 flex-col">
        {address === null ? (
          <span id={titleId} dir="auto" className="break-words text-strong">
            {material.title}
          </span>
        ) : (
          <a
            id={titleId}
            href={address}
            referrerPolicy="no-referrer"
            {...(picture ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
            className="inline-flex min-h-11 items-center break-words text-accent-text underline"
          >
            <span dir="auto">{material.title}</span>
            {picture ? <span className="sr-only"> {t('signUp.opensInNewTab')}</span> : null}
          </a>
        )}
        <span className="text-small text-text-muted">
          <span dir="auto">{material.filename}</span>
          {' · '}
          {formatStorage(material.size_bytes, me.language)}
        </span>
      </span>
      {detach === null ? null : (
        <ActionButton
          ref={detachButton}
          busy={detach.busy}
          unavailable={detach.unavailable}
          onClick={() => {
            detachButton.current?.focus();
            detach.onDetach();
          }}
          className={quietButtonClass}
        >
          {texts.detach}
          <span className="sr-only">
            {' '}
            <span dir="auto">{material.title}</span>
          </span>
        </ActionButton>
      )}
    </li>
  );
}

function AttachDialog({
  lesson,
  open,
  returnFocus,
  handlers,
  onClose,
}: {
  readonly lesson: LessonDetail;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly handlers: MaterialHandlers;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const texts = useMaterialTexts();
  const headingId = useId();
  const searchField = useRef<HTMLInputElement>(null);
  const [opened] = useState(lesson);
  const [refusals, setRefusals] = useState(0);
  const action = useAction((settled) => {
    if (settled.kind === 'done') {
      onClose();
    }
  });
  const save = useMaterialsUpdate(
    opened,
    {
      onSaved: handlers.onSaved,
      onStale: () => {
        onClose();
        handlers.onStale();
      },
    },
    () => {
      setRefusals((count) => count + 1);
    },
  );
  const [chosen, setChosen] = useState<readonly PickedMaterial[]>(() => pickedOf(opened.materials));
  const refusal = action.state.kind === 'refused' && !isStale(action.state) ? action.state : null;

  return (
    <ModalDialog
      open={open}
      busy={action.busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={searchField}
      onClose={onClose}
    >
      <div className="flex flex-col gap-4">
        <h2 id={headingId} className="font-serif text-title">
          {texts.attachMaterial}
        </h2>
        <MaterialPicker chosen={chosen} searchField={searchField} onChange={setChosen} />
        {refusal === null ? null : (
          <FormAlert key={refusals}>{refusalText(t, refusal.error, refusal.retryAfter, undefined)}</FormAlert>
        )}
        <FormActions>
          <ActionButton
            busy={action.busy}
            onClick={() => {
              if (sameMaterials(chosen, pickedOf(opened.materials))) {
                onClose();
                return;
              }
              save(action, chosen);
            }}
            className={primaryButtonClass}
          >
            {t('lessonEdit.save')}
          </ActionButton>
          <ActionButton busy={false} unavailable={action.busy} onClick={onClose} className={quietButtonClass}>
            {t('newLesson.cancel')}
          </ActionButton>
        </FormActions>
      </div>
    </ModalDialog>
  );
}

export function LessonMaterials({
  lesson,
  me,
  handlers,
}: {
  readonly lesson: LessonDetail;
  readonly me: Me;
  readonly handlers: MaterialHandlers;
}): ReactElement | null {
  const { t } = useTranslation();
  const texts = useMaterialTexts();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const attachButton = useRef<HTMLButtonElement>(null);
  const [attaching, setAttaching] = useState(0);
  const [attachOpen, setAttachOpen] = useState(false);
  const [detaching, setDetaching] = useState<string | null>(null);
  const [refusals, setRefusals] = useState(0);
  const detachAction = useAction();
  const editing = me.role === 'tutor' && me.state === 'active';
  useFocusAfterRemovedRow(list, lesson.materials.map(rowKey), editing ? 'button' : 'a', heading);
  const detach = useMaterialsUpdate(lesson, handlers, () => {
    setRefusals((count) => count + 1);
  });
  if (!editing && lesson.materials.length === 0) {
    return null;
  }
  const detachRefusal =
    detachAction.state.kind === 'refused' && !isStale(detachAction.state) ? detachAction.state : null;

  return (
    <section aria-labelledby={headingId} data-part="materials" className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
          {texts.materials}
        </h2>
        {editing ? (
          <ActionButton
            ref={attachButton}
            busy={false}
            unavailable={detachAction.busy}
            onClick={() => {
              setAttaching((count) => count + 1);
              setAttachOpen(true);
            }}
            className={quietButtonClass}
          >
            {texts.attachMaterial}
          </ActionButton>
        ) : null}
      </div>
      {lesson.materials.length === 0 ? (
        <p className="text-small text-text-muted">{texts.noMaterials}</p>
      ) : (
        <ul ref={list} aria-labelledby={headingId} className="flex flex-col border-t border-border">
          {lesson.materials.map((material): ReactElement => {
            const materialId = material.material_id;
            return (
              <MaterialRow
                key={rowKey(material)}
                material={material}
                me={me}
                detach={
                  editing && materialId !== undefined
                    ? {
                        busy: detachAction.busy && detaching === materialId,
                        unavailable: detachAction.busy,
                        onDetach: (): void => {
                          setDetaching(materialId);
                          detach(
                            detachAction,
                            pickedOf(lesson.materials).filter((picked) => picked.id !== materialId),
                          );
                        },
                      }
                    : null
                }
              />
            );
          })}
        </ul>
      )}
      {detachRefusal === null ? null : (
        <FormAlert key={refusals}>{refusalText(t, detachRefusal.error, detachRefusal.retryAfter, undefined)}</FormAlert>
      )}
      {editing ? (
        <AttachDialog
          key={attaching}
          lesson={lesson}
          open={attachOpen}
          returnFocus={attachButton}
          handlers={handlers}
          onClose={() => {
            setAttachOpen(false);
          }}
        />
      ) : null}
    </section>
  );
}
