import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import {
  type ReactElement,
  type ReactNode,
  type RefObject,
  type SyntheticEvent,
  useEffect,
  useId,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { ActionNotice } from '../../components/ui/ActionNotice.tsx';
import { ActionMenu, type MenuItem } from '../../components/ui/ActionMenu.tsx';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import { EmptyState } from '../../components/ui/EmptyState.tsx';
import {
  ActionButton,
  FormActions,
  FormAlert,
  SubmitButton,
  TextField,
  dangerFilledButtonClass,
  fieldMessage,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { PageColumns } from '../../components/ui/PageColumns.tsx';
import { Refusal, refusalText } from '../../components/ui/Refusal.tsx';
import { hasUnknownOutcome, settledRequest } from '../../components/ui/settled.ts';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { type ApiClient, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import type { components } from '../../lib/api/schema.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { LessonList } from '../schedule/LessonList.tsx';
import { NewLessonButton, ScheduleLessonAction, useFocusHeadingWhenFilled } from '../schedule/ScheduleLessonAction.tsx';
import { GroupIssues, GroupMemberNote } from '../homework/GroupIssues.tsx';
import { IssuedNotice, homeworkTextLoaders, useIssuedResult } from '../homework/IssueAssignment.tsx';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { groupsTextLoaders } from './texts.ts';
import { studentsKey } from '../students/api.ts';
import {
  type Group,
  type GroupDetail,
  type GroupMember,
  fetchGroup,
  groupKey,
  groupMemberLimit,
  groupsKey,
  isGroupColour,
  memberCapReached,
  memberChoiceKey,
} from './api.ts';
import { ColourField, GroupDot } from './ColourField.tsx';
import { type Chosen, MemberPicker, toggled, trimmed } from './MemberPicker.tsx';
import { chosenAfterRefusal, membershipRefusal } from './NewGroupDialog.tsx';

const groupNewLessonOpener = 'group-new-lesson';
const groupEmptyNewLessonOpener = 'group-empty-new-lesson';
const sectionHeadClass = 'flex w-full flex-wrap items-baseline justify-between gap-3';

type GroupPatch = components['schemas']['GroupPatch'];

type MembershipNotice =
  | { readonly kind: 'added'; readonly count: number }
  | { readonly kind: 'removed'; readonly relationshipId: string; readonly name: string; readonly count: number }
  | null;

type AddBackState =
  | { readonly kind: 'removed'; readonly unconfirmed: boolean; readonly problem: string | null }
  | { readonly kind: 'back' };

function addBackProblem(t: TFunction, state: ActionState): string | null {
  if (state.kind !== 'refused') {
    return null;
  }
  if (state.error.code === 'relationship_not_active' || state.error.code === 'not_found') {
    return t('groups.addBackInactive');
  }
  if (state.error.code === 'audience_not_active') {
    return t('groups.addBackArchived');
  }
  if (memberCapReached(state.error)) {
    return t('groups.addBackFull');
  }
  return refusalText(t, state.error, state.retryAfter, undefined);
}

function RemovedNotice({
  name,
  state,
  busy,
  onAddBack,
}: {
  readonly name: string;
  readonly state: Extract<AddBackState, { readonly kind: 'removed' }>;
  readonly busy: boolean;
  readonly onAddBack: () => void;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <ActionNotice
      action={{ label: t(state.unconfirmed ? 'groups.retry' : 'groups.addBack'), busy, onActivate: onAddBack }}
      problem={state.problem}
    >
      {state.unconfirmed ? (
        t('groups.unconfirmed')
      ) : (
        <>
          {t('groups.removedFrom')} <bdi>{name}</bdi>. {t('groups.unsentNotRestored')}
        </>
      )}
    </ActionNotice>
  );
}

export function AddBackNotice({
  groupId,
  relationshipId,
  name,
  unavailable,
  onBusy,
  onChanged,
}: {
  readonly groupId: string;
  readonly relationshipId: string;
  readonly name: string;
  readonly unavailable: boolean;
  readonly onBusy: (busy: boolean) => void;
  readonly onChanged: (group: Group) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [state, setState] = useState<AddBackState>({ kind: 'removed', unconfirmed: false, problem: null });
  const addBack = useAction(() => {
    onBusy(false);
  });

  async function freshMembers(): Promise<readonly string[] | null> {
    try {
      await queryClient.cancelQueries({ queryKey: groupKey(groupId) });
      const fresh = await queryClient.query({
        queryKey: groupKey(groupId),
        queryFn: () => fetchGroup(client, groupId),
        staleTime: 0,
        retry: false,
      });
      return fresh.members.map((member) => member.relationship_id);
    } catch {
      return null;
    }
  }

  function addBackRemoved(removed: Extract<AddBackState, { readonly kind: 'removed' }>): void {
    if (unavailable || addBack.busy) {
      return;
    }
    const isBack = (members: readonly string[] | null): boolean => members?.includes(relationshipId) === true;
    const confirmBack = (): ActionState => {
      setState({ kind: 'back' });
      return done;
    };
    onBusy(true);
    addBack.run(async (): Promise<ActionState> => {
      if (removed.unconfirmed && isBack(await freshMembers())) {
        return confirmBack();
      }
      const answer = await settledRequest(() => patchGroup(client, groupId, { add: [relationshipId] }));
      if (isGroup(answer)) {
        onChanged(answer);
        return confirmBack();
      }
      if (hasUnknownOutcome(answer)) {
        if (isBack(await freshMembers())) {
          return confirmBack();
        }
        setState({ kind: 'removed', unconfirmed: true, problem: null });
        return { kind: 'idle' };
      }
      setState({ kind: 'removed', unconfirmed: removed.unconfirmed, problem: addBackProblem(t, answer) });
      return { kind: 'idle' };
    });
  }

  if (state.kind === 'back') {
    return (
      <FocusedNotice
        message={
          <>
            {t('groups.backInGroup')} <bdi>{name}</bdi>. {t('groups.seesChat')}
          </>
        }
      />
    );
  }
  return (
    <RemovedNotice
      name={name}
      state={state}
      busy={addBack.busy}
      onAddBack={() => {
        addBackRemoved(state);
      }}
    />
  );
}

export function RemovalForm({
  groupId,
  groupName,
  member,
  headingId,
  consequenceId,
  cancelRef,
  unavailable,
  onBusy,
  onRemoved,
  onChangedElsewhere,
  onUnknownOutcome,
  onCancel,
}: {
  readonly groupId: string;
  readonly groupName: string;
  readonly member: GroupMember;
  readonly headingId: string;
  readonly consequenceId: string;
  readonly cancelRef: RefObject<HTMLButtonElement | null>;
  readonly unavailable: boolean;
  readonly onBusy: (busy: boolean) => void;
  readonly onRemoved: (group: Group) => void;
  readonly onChangedElsewhere: () => void;
  readonly onUnknownOutcome: () => void;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const action = useAction(() => {
    onBusy(false);
  });

  function remove(): void {
    if (unavailable || action.busy) {
      return;
    }
    onBusy(true);
    action.run(async (): Promise<ActionState> => {
      const answer = await settledRequest(() => patchGroup(client, groupId, { remove: [member.relationship_id] }));
      if (isGroup(answer)) {
        onRemoved(answer);
        return done;
      }
      if (showsStaleData(answer)) {
        onChangedElsewhere();
      } else if (hasUnknownOutcome(answer)) {
        onUnknownOutcome();
      }
      return answer;
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="text-heading">
        {t('groups.removeTitle', { name: member.display_name, group: groupName })}
      </h2>
      <p id={consequenceId}>{t('groups.removeConsequence')}</p>
      <Refusal state={action.state} />
      <FormActions>
        <ActionButton busy={action.busy} unavailable={unavailable} onClick={remove} className={dangerFilledButtonClass}>
          {t('groups.removeFromGroup')}
        </ActionButton>
        <ActionButton
          ref={cancelRef}
          busy={false}
          unavailable={action.busy}
          onClick={onCancel}
          className={quietButtonClass}
        >
          {t('groups.cancel')}
        </ActionButton>
      </FormActions>
    </div>
  );
}

function archiveConsequence(t: TFunction, cancelling: boolean, futureLessons: number): string {
  if (!cancelling) {
    return t('groups.archiveQuietConsequence');
  }
  return futureLessons === 0
    ? t('groups.archiveSeriesConsequence')
    : t('groups.archiveConsequence', { count: futureLessons });
}

export function ArchiveForm({
  group,
  cancelling,
  headingId,
  consequenceId,
  cancelRef,
  onBusy,
  onArchived,
  onChangedElsewhere,
  onUnknownOutcome,
  onCancel,
}: {
  readonly group: Pick<GroupDetail, 'id' | 'name' | 'future_lesson_count'>;
  readonly cancelling: boolean;
  readonly headingId: string;
  readonly consequenceId: string;
  readonly cancelRef: RefObject<HTMLButtonElement | null>;
  readonly onBusy: (busy: boolean) => void;
  readonly onArchived: (group: Group) => void;
  readonly onChangedElsewhere: () => void;
  readonly onUnknownOutcome: () => void;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const action = useAction(() => {
    onBusy(false);
  });

  function archive(): void {
    if (action.busy) {
      return;
    }
    onBusy(true);
    action.run(async (): Promise<ActionState> => {
      const answer = await settledRequest(async (): Promise<Group | ActionState> => {
        const { data, error, response } = await client.POST('/api/v1/groups/{id}/archive', {
          params: { path: { id: group.id } },
        });
        return data ?? refusedBy(response, error);
      });
      if (isGroup(answer)) {
        onArchived(answer);
        return done;
      }
      if (showsStaleData(answer)) {
        onChangedElsewhere();
      } else if (hasUnknownOutcome(answer)) {
        onUnknownOutcome();
      }
      return answer;
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="text-heading">
        {t('groups.archiveTitle', { name: group.name })}
      </h2>
      <p id={consequenceId}>{archiveConsequence(t, cancelling, group.future_lesson_count)}</p>
      <Refusal state={action.state} />
      <FormActions>
        <ActionButton busy={action.busy} onClick={archive} className={dangerFilledButtonClass}>
          {t('groups.archiveSubmit')}
        </ActionButton>
        <ActionButton
          ref={cancelRef}
          busy={false}
          unavailable={action.busy}
          onClick={onCancel}
          className={quietButtonClass}
        >
          {t('groups.cancel')}
        </ActionButton>
      </FormActions>
    </div>
  );
}

interface GroupHandlers {
  readonly onChanged: (group: Group) => void;
  readonly onChangedElsewhere: () => void;
  readonly onUnknownOutcome: () => void;
}

async function patchGroup(client: ApiClient, id: string, patch: GroupPatch): Promise<Group | ActionState> {
  const { data, error, response } = await client.PATCH('/api/v1/groups/{id}', {
    params: { path: { id } },
    body: patch,
  });
  return data ?? refusedBy(response, error);
}

function freePlaces(group: Group): number {
  return Math.max(0, groupMemberLimit - group.members.length);
}

function isGroup(value: Group | ActionState): value is Group {
  return 'id' in value;
}

type GroupOutcome =
  | { readonly kind: 'saved' | 'unarchived' | 'restored' }
  | { readonly kind: 'archived'; readonly undoable: boolean }
  | { readonly kind: 'refused'; readonly state: ActionState };

function DetailsDialog({
  group,
  reloads,
  handlers,
  open,
  returnFocus,
  onClose,
  onSaved,
}: {
  readonly group: Group;
  readonly reloads: number;
  readonly handlers: GroupHandlers;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onSaved: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const action = useAction();
  const error = action.state.kind === 'refused' ? action.state.error : null;

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const colour = stringField(data, 'colour');
    const patch: GroupPatch = {
      name: stringField(data, 'name'),
      ...(isGroupColour(colour) ? { colour } : {}),
    };
    action.run(async (): Promise<ActionState> => {
      const answer = await patchGroup(client, group.id, patch);
      if (!isGroup(answer)) {
        if (showsStaleData(answer)) {
          onClose();
          handlers.onChangedElsewhere();
          return { kind: 'idle' };
        }
        return answer;
      }
      handlers.onChanged(answer);
      onSaved();
      return { kind: 'idle' };
    });
  }

  return (
    <ModalDialog
      open={open}
      busy={action.busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      onClose={() => {
        action.reset();
        onClose();
      }}
    >
      <form aria-labelledby={headingId} onSubmit={submit} onChange={action.reset} className="flex flex-col gap-4">
        <h2 id={headingId} className="text-heading">
          {t('groups.details')}
        </h2>
        <Refusal state={action.state} />
        <TextField
          label={t('groups.name')}
          name="name"
          autoComplete="off"
          maxLength={60}
          defaultValue={group.name}
          reloads={reloads}
          error={fieldMessage(t, error, 'name')}
        />
        <ColourField defaultColour={group.colour} reloads={reloads} />
        <FormActions>
          <SubmitButton busy={action.busy}>{t('groups.save')}</SubmitButton>
          <ActionButton
            busy={false}
            unavailable={action.busy}
            onClick={() => {
              action.reset();
              onClose();
            }}
            className={quietButtonClass}
          >
            {t('groups.cancel')}
          </ActionButton>
        </FormActions>
      </form>
    </ModalDialog>
  );
}

function AddMembersDialog({
  group,
  note,
  open,
  returnFocus,
  onClose,
  onAdded,
  onChangedElsewhere,
}: {
  readonly group: Group;
  readonly note: ReactNode;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onAdded: (group: Group) => void;
  readonly onChangedElsewhere: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const headingId = useId();
  const added = useRef<Group | null>(null);
  const action = useAction((state) => {
    const group = added.current;
    added.current = null;
    if (state.kind === 'done' && group !== null) {
      onAdded(group);
    }
    if (state.kind === 'refused' && state.error.code === 'audience_not_active') {
      onChangedElsewhere();
    }
  });
  const [chosen, setChosen] = useState<Chosen>(new Map());
  const [refusals, setRefusals] = useState(0);
  const refusal = membershipRefusal(t, action.state);
  const slots = freePlaces(group);

  function close(): void {
    action.reset();
    setChosen(new Map());
    onClose();
  }

  function add(): void {
    const ids = [...chosen.keys()];
    action.run(async (): Promise<ActionState> => {
      const answer = await patchGroup(client, group.id, { add: ids });
      if (isGroup(answer)) {
        setChosen(new Map());
        added.current = answer;
        return done;
      }
      if (answer.kind === 'refused' && answer.error.code === 'audience_not_active') {
        setChosen(new Map());
        return answer;
      }
      setChosen((current) => chosenAfterRefusal(current, answer));
      setRefusals((count) => count + 1);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: groupsKey }),
        queryClient.invalidateQueries({ queryKey: memberChoiceKey }),
      ]);
      const reloaded = queryClient.getQueryData<Group>(groupKey(group.id));
      if (reloaded !== undefined) {
        setChosen((current) => trimmed(current, freePlaces(reloaded)));
      }
      return answer;
    });
  }

  return (
    <ModalDialog open={open} busy={action.busy} labelledBy={headingId} returnFocus={returnFocus} onClose={close}>
      <div className="flex flex-col gap-4">
        <h2 id={headingId} className="font-serif text-title">
          {t('groups.addTitle')}
        </h2>
        {refusal === null ? null : <FormAlert key={refusals}>{refusal}</FormAlert>}
        {note}
        <MemberPicker
          excluded={new Set(group.members.map((member) => member.relationship_id))}
          chosen={chosen}
          slots={slots}
          onToggle={(id, name) => {
            setChosen((current) => toggled(current, id, name));
          }}
        />
        <FormActions>
          <ActionButton
            busy={action.busy}
            unavailable={chosen.size === 0 || slots === 0}
            onClick={add}
            className={primaryButtonClass}
          >
            {t('groups.addSubmit')}
          </ActionButton>
          <ActionButton busy={false} unavailable={action.busy} onClick={close} className={quietButtonClass}>
            {t('groups.cancel')}
          </ActionButton>
        </FormActions>
      </div>
    </ModalDialog>
  );
}

function Members({
  group,
  memberNote,
  handlers,
}: {
  readonly group: Group;
  readonly memberNote: ReactNode;
  readonly handlers: GroupHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const opener = useRef<HTMLButtonElement>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [notice, setNotice] = useState<MembershipNotice>(null);
  const [confirming, setConfirming] = useState<GroupMember | null>(null);
  const [removing, setRemoving] = useState(false);
  const [addingBack, setAddingBack] = useState(false);
  const memberMenus = useId();
  const removeHeadingId = useId();
  const removeConsequenceId = useId();
  const memberMenuButton = useRef<HTMLElement | null>(null);
  const keepMember = useRef<HTMLButtonElement>(null);
  const archived = group.archived_at !== null;
  const listRef = useRef<HTMLUListElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  useFocusAfterRemovedRow(
    listRef,
    group.members.map((member) => member.relationship_id),
    'a',
    heading,
    removing || notice !== null,
  );

  const confirmingGone =
    confirming !== null && !group.members.some((member) => member.relationship_id === confirming.relationship_id);
  const [goneMeanwhile, setGoneMeanwhile] = useState(0);
  if (confirmingGone && !removing) {
    setConfirming(null);
    setGoneMeanwhile((count) => count + 1);
  }
  const reportedGone = useRef(0);

  useEffect(() => {
    if (goneMeanwhile !== reportedGone.current) {
      reportedGone.current = goneMeanwhile;
      handlers.onChangedElsewhere();
    }
  }, [goneMeanwhile, handlers]);

  function memberMenuId(member: GroupMember): string {
    return `${memberMenus}-${member.relationship_id}`;
  }

  function confirmRemoval(member: GroupMember): void {
    memberMenuButton.current = document.getElementById(memberMenuId(member));
    setConfirming(member);
  }

  function closeRemoval(): void {
    setConfirming(null);
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-3">
      <div className={sectionHeadClass}>
        <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
          {t('groups.members')}
        </h2>
        {archived ? null : (
          <button
            ref={opener}
            type="button"
            onClick={() => {
              setNotice(null);
              setDialogOpen(true);
            }}
            className={quietButtonClass}
          >
            {t('groups.add')}
          </button>
        )}
      </div>
      <p className="text-small text-text-muted">
        {t('groups.memberCountOf', { count: group.members.length, limit: groupMemberLimit })}
      </p>
      <ModalDialog
        open={confirming !== null}
        busy={removing}
        labelledBy={removeHeadingId}
        describedBy={removeConsequenceId}
        returnFocus={memberMenuButton}
        initialFocus={keepMember}
        onClose={closeRemoval}
      >
        {confirming === null ? null : (
          <RemovalForm
            groupId={group.id}
            groupName={group.name}
            member={confirming}
            headingId={removeHeadingId}
            consequenceId={removeConsequenceId}
            cancelRef={keepMember}
            unavailable={addingBack}
            onBusy={setRemoving}
            onRemoved={(answer) => {
              setConfirming(null);
              handlers.onChanged(answer);
              setNotice((previous) => ({
                kind: 'removed',
                relationshipId: confirming.relationship_id,
                name: confirming.display_name,
                count: (previous?.count ?? 0) + 1,
              }));
            }}
            onChangedElsewhere={() => {
              setConfirming(null);
              handlers.onChangedElsewhere();
            }}
            onUnknownOutcome={handlers.onUnknownOutcome}
            onCancel={closeRemoval}
          />
        )}
      </ModalDialog>
      {notice === null ? null : notice.kind === 'removed' ? (
        <AddBackNotice
          key={notice.count}
          groupId={group.id}
          relationshipId={notice.relationshipId}
          name={notice.name}
          unavailable={removing}
          onBusy={setAddingBack}
          onChanged={handlers.onChanged}
        />
      ) : (
        <FocusedNotice key={notice.count} message={t('groups.added')} />
      )}
      {group.members.length === 0 ? (
        <EmptyState
          level={3}
          title={t('groups.noMembers')}
          sentence={t(archived ? 'groups.archivedNoNewMembers' : 'groups.noMembersNext')}
        />
      ) : (
        <ul ref={listRef} aria-labelledby={headingId} className="w-full border-b border-border">
          {group.members.map((member) => (
            <li
              key={member.relationship_id}
              data-row={member.relationship_id}
              className="flex flex-wrap items-center gap-3 border-t border-border py-3"
            >
              <Avatar name={member.display_name} avatarId={member.avatar_id} />
              <span className="flex min-w-0 flex-1 flex-col">
                <Link
                  to={`/students/${encodeURIComponent(member.relationship_id)}`}
                  className="block truncate py-3 text-strong text-accent-text underline desktop:py-0"
                >
                  {member.display_name}
                </Link>
                <span className="truncate text-small text-text-muted">{member.name}</span>
              </span>
              <ActionMenu
                label={t('groups.memberActions', { name: member.display_name })}
                unavailable={removing || addingBack}
                buttonId={memberMenuId(member)}
                items={[
                  {
                    label: t('groups.removeFromGroup'),
                    danger: true,
                    onChoose: (): void => {
                      confirmRemoval(member);
                    },
                  },
                ]}
              >
                <MoreIcon />
              </ActionMenu>
            </li>
          ))}
        </ul>
      )}
      {archived && group.members.length > 0 ? (
        <p className="text-small text-text-muted">{t('groups.archivedNoNewMembers')}</p>
      ) : null}
      <AddMembersDialog
        group={group}
        note={memberNote}
        open={dialogOpen && !archived}
        returnFocus={opener}
        onClose={() => {
          setDialogOpen(false);
        }}
        onAdded={(changed) => {
          setDialogOpen(false);
          handlers.onChanged(changed);
          setNotice((previous) => ({ kind: 'added', count: (previous?.count ?? 0) + 1 }));
        }}
        onChangedElsewhere={() => {
          setDialogOpen(false);
          handlers.onChangedElsewhere();
        }}
      />
    </section>
  );
}

function cancelsLessons(group: GroupDetail): boolean {
  return group.future_lesson_count > 0 || group.has_open_series;
}

type GroupDialog = 'details' | { readonly kind: 'archive'; readonly cancelling: boolean } | null;

function GroupActions({
  group,
  reloads,
  handlers,
  onOutcome,
}: {
  readonly group: GroupDetail;
  readonly reloads: number;
  readonly handlers: GroupHandlers;
  readonly onOutcome: (outcome: GroupOutcome | null) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const dialogHeadingId = useId();
  const consequenceId = useId();
  const menuButton = useRef<HTMLButtonElement>(null);
  const dismissButton = useRef<HTMLButtonElement>(null);
  const [dialog, setDialog] = useState<GroupDialog>(null);
  const [archiving, setArchiving] = useState(false);
  const action = useAction();
  const archived = group.archived_at !== null;
  const archiveDialog = dialog !== null && dialog !== 'details' ? dialog : null;
  const [archiveChanged, setArchiveChanged] = useState(0);
  if (archiveDialog !== null && !archiving && (archived || cancelsLessons(group) !== archiveDialog.cancelling)) {
    setDialog(null);
    setArchiveChanged((count) => count + 1);
  }
  const confirming = archiveDialog !== null;
  const reportedArchiveChange = useRef(0);

  useEffect(() => {
    if (archiveChanged !== reportedArchiveChange.current) {
      reportedArchiveChange.current = archiveChanged;
      handlers.onChangedElsewhere();
    }
  }, [archiveChanged, handlers]);

  function close(): void {
    setDialog(null);
  }

  function unarchive(): void {
    onOutcome(null);
    action.run(async (): Promise<ActionState> => {
      const answer = await settledRequest(async (): Promise<Group | ActionState> => {
        const { data, error, response } = await client.POST('/api/v1/groups/{id}/unarchive', {
          params: { path: { id: group.id } },
        });
        return data ?? refusedBy(response, error);
      });
      if (!isGroup(answer)) {
        if (showsStaleData(answer)) {
          handlers.onChangedElsewhere();
          return answer;
        }
        if (hasUnknownOutcome(answer)) {
          handlers.onUnknownOutcome();
        }
        onOutcome({ kind: 'refused', state: answer });
        return answer;
      }
      handlers.onChanged(answer);
      onOutcome({ kind: 'unarchived' });
      return done;
    });
  }

  const items: MenuItem[] = [
    {
      label: t('groups.editDetails'),
      onChoose: (): void => {
        onOutcome(null);
        setDialog('details');
      },
    },
    {
      label: t(archived ? 'groups.unarchiveSubmit' : 'groups.archiveSubmit'),
      danger: !archived,
      onChoose: (): void => {
        if (archived) {
          unarchive();
          return;
        }
        onOutcome(null);
        setDialog({ kind: 'archive', cancelling: cancelsLessons(group) });
      },
    },
  ];

  return (
    <>
      <ActionMenu
        label={t('groups.actions')}
        busy={action.busy}
        unavailable={action.busy || archiving}
        items={items}
        buttonRef={menuButton}
      >
        <MoreIcon />
      </ActionMenu>
      <DetailsDialog
        group={group}
        reloads={reloads}
        handlers={handlers}
        open={dialog === 'details'}
        returnFocus={menuButton}
        onClose={close}
        onSaved={() => {
          close();
          onOutcome({ kind: 'saved' });
        }}
      />
      <ModalDialog
        open={confirming}
        busy={archiving}
        labelledBy={dialogHeadingId}
        describedBy={consequenceId}
        returnFocus={menuButton}
        initialFocus={dismissButton}
        onClose={close}
      >
        {archiveDialog === null ? null : (
          <ArchiveForm
            group={group}
            cancelling={archiveDialog.cancelling}
            headingId={dialogHeadingId}
            consequenceId={consequenceId}
            cancelRef={dismissButton}
            onBusy={setArchiving}
            onArchived={(answer) => {
              setDialog(null);
              handlers.onChanged(answer);
              onOutcome({ kind: 'archived', undoable: !archiveDialog.cancelling });
            }}
            onChangedElsewhere={() => {
              setDialog(null);
              handlers.onChangedElsewhere();
            }}
            onUnknownOutcome={handlers.onUnknownOutcome}
            onCancel={close}
          />
        )}
      </ModalDialog>
    </>
  );
}

function GroupLessons({ group }: { readonly group: GroupDetail }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const empty = group.next_lessons.length === 0;
  useFocusHeadingWhenFilled(empty, heading);
  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-3">
      <div className={sectionHeadClass}>
        <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
          {t('lessons.next')}
        </h2>
        <Link to="/schedule" className={linkClass}>
          {t('lessons.openSchedule')}
        </Link>
      </div>
      {empty ? (
        <div className="flex flex-col items-start gap-3">
          <p className="text-text-muted">{t('lessons.noNext')}</p>
          {group.archived_at === null && me.state === 'active' ? (
            <NewLessonButton
              audience={{ kind: 'group', id: group.id, name: group.name }}
              opener={groupEmptyNewLessonOpener}
            >
              {t('groups.newLessonForGroup')}
            </NewLessonButton>
          ) : null}
        </div>
      ) : (
        <div className="w-full">
          <LessonList items={group.next_lessons} label={t('lessons.next')} me={me} showAudience={false} />
        </div>
      )}
    </section>
  );
}

function GroupView({
  group,
  homework,
  homeworkPending,
  handlers,
}: {
  readonly group: GroupDetail;
  readonly homework: boolean;
  readonly homeworkPending: boolean;
  readonly handlers: GroupHandlers;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <>
      <p>
        <Link to="/groups" className={linkClass}>
          {t('groups.back')}
        </Link>
      </p>
      <p className="flex items-center gap-2 text-text-muted">
        <GroupDot colour={group.colour} />
        {group.archived_at === null ? null : <StatusWord tone="muted">{t('groups.archived')}</StatusWord>}
      </p>
      <PageColumns
        asideLabel={t('lessons.next')}
        busy={homeworkPending}
        main={
          <Members
            group={group}
            memberNote={homework ? <GroupMemberNote groupId={group.id} /> : null}
            handlers={handlers}
          />
        }
        aside={
          <>
            <GroupLessons group={group} />
            {homework ? <GroupIssues groupId={group.id} /> : null}
          </>
        }
      />
    </>
  );
}

function outcomeMessage(t: TFunction, outcome: GroupOutcome): string | null {
  if (outcome.kind === 'saved') {
    return t('groups.saved');
  }
  if (outcome.kind === 'restored') {
    return t('groups.restoredNotice');
  }
  return outcome.kind === 'unarchived' ? t('groups.unarchivedNotice') : null;
}

function ArchivedNotice({
  groupId,
  handlers,
  onRestored,
}: {
  readonly groupId: string;
  readonly handlers: GroupHandlers;
  readonly onRestored: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const action = useAction();
  const [unconfirmed, setUnconfirmed] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);

  async function isRestored(): Promise<boolean> {
    try {
      await queryClient.cancelQueries({ queryKey: groupKey(groupId) });
      const fresh = await queryClient.query({
        queryKey: groupKey(groupId),
        queryFn: () => fetchGroup(client, groupId),
        staleTime: 0,
        retry: false,
      });
      return fresh.archived_at === null;
    } catch {
      return false;
    }
  }

  function restored(): ActionState {
    onRestored();
    return done;
  }

  function undo(): void {
    action.run(async (): Promise<ActionState> => {
      if (unconfirmed && (await isRestored())) {
        return restored();
      }
      const answer = await settledRequest(async (): Promise<Group | ActionState> => {
        const { data, error, response } = await client.POST('/api/v1/groups/{id}/unarchive', {
          params: { path: { id: groupId } },
        });
        return data ?? refusedBy(response, error);
      });
      if (isGroup(answer)) {
        handlers.onChanged(answer);
        return restored();
      }
      if (hasUnknownOutcome(answer)) {
        if (await isRestored()) {
          return restored();
        }
        setUnconfirmed(true);
        setProblem(null);
        return { kind: 'idle' };
      }
      if (showsStaleData(answer)) {
        handlers.onChangedElsewhere();
        return answer;
      }
      setProblem(answer.kind === 'refused' ? refusalText(t, answer.error, answer.retryAfter, undefined) : null);
      return { kind: 'idle' };
    });
  }

  return (
    <ActionNotice
      action={{ label: t(unconfirmed ? 'groups.retry' : 'groups.undo'), busy: action.busy, onActivate: undo }}
      problem={problem}
    >
      {t(unconfirmed ? 'groups.unconfirmed' : 'groups.archivedNotice')}
    </ActionNotice>
  );
}

function OutcomeNotice({
  outcome,
  groupId,
  handlers,
  onOutcome,
}: {
  readonly outcome: GroupOutcome;
  readonly groupId: string;
  readonly handlers: GroupHandlers;
  readonly onOutcome: (outcome: GroupOutcome) => void;
}): ReactElement | null {
  const { t } = useTranslation();
  if (outcome.kind === 'refused') {
    return <Refusal state={outcome.state} />;
  }
  if (outcome.kind === 'archived') {
    return outcome.undoable ? (
      <ArchivedNotice
        groupId={groupId}
        handlers={handlers}
        onRestored={() => {
          onOutcome({ kind: 'restored' });
        }}
      />
    ) : (
      <FocusedNotice message={t('groups.archivedCancelledNotice')} />
    );
  }
  const message = outcomeMessage(t, outcome);
  return message === null ? null : <FocusedNotice message={message} />;
}

export function GroupPage(): ReactElement {
  const { groupId = '' } = useParams();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const key = groupKey(groupId);
  const [reloads, setReloads] = useState(0);
  const [changed, setChanged] = useState<{ readonly id: string; readonly count: number } | null>(null);
  if (changed !== null && changed.id !== groupId) {
    setChanged(null);
  }
  const query = useQuery({
    queryKey: key,
    queryFn: () => fetchGroup(client, groupId),
    retry: false,
  });
  const queryError = query.isError ? asApiError(query.error) : null;
  const refetchError =
    queryError !== null && queryError.code !== 'not_found' && query.data !== undefined ? queryError : null;
  const error = refetchError === null ? queryError : null;
  const notFound = error?.code === 'not_found';

  const me = useSignedInAccount();
  const homeworkTexts = useFeatureTexts('homework', homeworkTextLoaders);
  const groupTexts = useFeatureTexts('groups', groupsTextLoaders);
  const homework = me.email_verified && homeworkTexts.ready;
  const [outcome, setOutcome] = useState<{
    readonly id: string;
    readonly count: number;
    readonly outcome: GroupOutcome;
  } | null>(null);
  if (outcome !== null && outcome.id !== groupId) {
    setOutcome(null);
  }
  const issuedId = useIssuedResult();
  const [issued, setIssued] = useState<{ readonly id: string; readonly count: number } | null>(() =>
    issuedId === null ? null : { id: groupId, count: 1 },
  );
  if (issued !== null && issued.id !== groupId) {
    setIssued(null);
  }
  const shown = error === null && groupTexts.ready ? (query.data ?? null) : null;
  const noticesShown = groupTexts.ready && !notFound;
  const handlers: GroupHandlers = {
    onChanged: (group) => {
      queryClient.setQueryData<GroupDetail>(key, (previous) =>
        previous === undefined
          ? previous
          : {
              ...group,
              next_lessons: previous.next_lessons,
              future_lesson_count: previous.future_lesson_count,
              has_open_series: previous.has_open_series,
              channel_id: previous.channel_id,
            },
      );
      void queryClient.invalidateQueries({ queryKey: key });
      void queryClient.invalidateQueries({ queryKey: groupsKey, refetchType: 'none' });
      void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
    },
    onChangedElsewhere: () => {
      setChanged((previous) => ({ id: groupId, count: (previous?.count ?? 0) + 1 }));
      void queryClient.invalidateQueries({ queryKey: groupsKey }).then(() => {
        setReloads((count) => count + 1);
      });
    },
    onUnknownOutcome: () => {
      void queryClient.invalidateQueries({ queryKey: key });
    },
  };

  function showOutcome(next: GroupOutcome | null): void {
    setOutcome((previous) =>
      next === null ? null : { id: groupId, count: (previous?.count ?? 0) + 1, outcome: next },
    );
  }

  function body(): ReactElement {
    if (!groupTexts.ready) {
      return groupTexts.failed ? (
        <LoadFailed error={asApiError(undefined)} onRetry={groupTexts.retry} />
      ) : (
        <DelayedLoading />
      );
    }
    if (error !== null) {
      return notFound ? (
        <NotFoundContent />
      ) : (
        <LoadFailed
          error={error}
          onRetry={() => {
            void query.refetch();
          }}
        />
      );
    }
    if (shown === null) {
      return <DelayedLoading />;
    }
    return <GroupView group={shown} homework={homework} homeworkPending={homeworkTexts.pending} handlers={handlers} />;
  }

  return (
    <Page
      {...(notFound ? { title: 'page.notFound' as const } : {})}
      {...(shown === null ? {} : { headingText: shown.name })}
      focusTitle={notFound}
      actionsBelowTitleOnPhones
      action={
        shown === null ? null : (
          <>
            {shown.channel_id == null ? null : (
              <Link to={`/messages/${encodeURIComponent(shown.channel_id)}`} className={linkClass}>
                {t('section.messages')}
              </Link>
            )}
            {shown.archived_at === null && me.state === 'active' ? (
              <ScheduleLessonAction
                audience={{ kind: 'group', id: shown.id, name: shown.name }}
                opener={groupNewLessonOpener}
                onScheduled={() => {
                  void queryClient.invalidateQueries({ queryKey: key });
                  void queryClient.invalidateQueries({ queryKey: groupsKey, refetchType: 'none' });
                }}
              />
            ) : null}
          </>
        )
      }
      menu={
        shown === null ? null : (
          <GroupActions group={shown} reloads={reloads} handlers={handlers} onOutcome={showOutcome} />
        )
      }
    >
      {changed === null || !noticesShown ? null : <FocusedNotice key={changed.count} message={t('groups.changed')} />}
      {issued === null || !noticesShown || !homeworkTexts.ready ? null : (
        <IssuedNotice key={issued.count} issueId={null} />
      )}
      {outcome === null || !noticesShown ? null : (
        <OutcomeNotice
          key={outcome.count}
          outcome={outcome.outcome}
          groupId={groupId}
          handlers={handlers}
          onOutcome={showOutcome}
        />
      )}
      {refetchError === null ? null : (
        <RefetchFailed
          error={refetchError}
          onRetry={() => {
            void query.refetch();
          }}
        />
      )}
      {body()}
    </Page>
  );
}
