import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type ReactElement, type ReactNode, type RefObject, type SyntheticEvent, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { ActionMenu, type MenuItem } from '../../components/ui/ActionMenu.tsx';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormAlert,
  SubmitButton,
  TextField,
  dangerButtonClass,
  fieldMessage,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  stringField,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { PageColumns } from '../../components/ui/PageColumns.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { type ApiClient, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import type { components } from '../../lib/api/schema.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { LessonList } from '../schedule/LessonList.tsx';
import { ScheduleLessonAction } from '../schedule/ScheduleLessonAction.tsx';
import { GroupIssues, GroupMemberNote } from '../homework/GroupIssues.tsx';
import { homeworkTextLoaders } from '../homework/IssueAssignment.tsx';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { groupsTextLoaders } from './texts.ts';
import { studentsKey } from '../students/api.ts';
import {
  type Group,
  type GroupDetail,
  fetchGroup,
  groupKey,
  groupMemberLimit,
  groupsKey,
  isGroupColour,
  memberChoiceKey,
} from './api.ts';
import { ColourField, GroupDot } from './ColourField.tsx';
import { type Chosen, MemberPicker, toggled, trimmed } from './MemberPicker.tsx';
import { chosenAfterRefusal, membershipRefusal } from './NewGroupDialog.tsx';

const groupNewLessonOpener = 'group-new-lesson';

type GroupPatch = components['schemas']['GroupPatch'];

type MembershipNotice = { readonly kind: 'added' | 'removed'; readonly count: number } | null;

interface GroupHandlers {
  readonly onChanged: (group: Group) => void;
  readonly onChangedElsewhere: () => void;
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
  { readonly kind: 'saved' | 'archived' | 'unarchived' } | { readonly kind: 'refused'; readonly state: ActionState };

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
        <div className="flex flex-wrap gap-3">
          <SubmitButton busy={action.busy}>{t('groups.save')}</SubmitButton>
          <ActionButton
            busy={false}
            unavailable={action.busy}
            onClick={() => {
              action.reset();
              onClose();
            }}
            className={textButtonClass}
          >
            {t('groups.cancel')}
          </ActionButton>
        </div>
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
        <div className="flex flex-wrap gap-3">
          <ActionButton
            busy={action.busy}
            unavailable={chosen.size === 0 || slots === 0}
            onClick={add}
            className={primaryButtonClass}
          >
            {t('groups.addSubmit')}
          </ActionButton>
          <ActionButton busy={false} unavailable={action.busy} onClick={close} className={textButtonClass}>
            {t('groups.cancel')}
          </ActionButton>
        </div>
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
  const client = useApiClient();
  const headingId = useId();
  const opener = useRef<HTMLButtonElement>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [notice, setNotice] = useState<MembershipNotice>(null);
  const [removing, setRemoving] = useState<string | null>(null);
  const action = useAction();
  const archived = group.archived_at !== null;
  const listRef = useRef<HTMLUListElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  useFocusAfterRemovedRow(
    listRef,
    group.members.map((member) => member.relationship_id),
    'a',
    heading,
    action.busy || notice !== null,
  );

  function remove(id: string): void {
    if (action.busy) {
      return;
    }
    setNotice(null);
    setRemoving(id);
    action.run(async (): Promise<ActionState> => {
      const answer = await patchGroup(client, group.id, { remove: [id] });
      if (!isGroup(answer)) {
        if (showsStaleData(answer)) {
          handlers.onChangedElsewhere();
        }
        return answer;
      }
      handlers.onChanged(answer);
      setNotice((previous) => ({ kind: 'removed', count: (previous?.count ?? 0) + 1 }));
      return done;
    });
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('groups.members')}
      </h2>
      <p className="text-small text-text-muted">
        {t('groups.memberCountOf', { count: group.members.length, limit: groupMemberLimit })}
      </p>
      <Refusal state={action.state} />
      {notice === null ? null : (
        <FocusedNotice key={notice.count} message={t(notice.kind === 'added' ? 'groups.added' : 'groups.removed')} />
      )}
      {group.members.length === 0 ? (
        <p className="text-text-muted">{t('groups.noMembers')}</p>
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
              <ActionButton
                busy={action.busy && removing === member.relationship_id}
                unavailable={action.busy}
                label={t('groups.removeNamed', { name: member.display_name })}
                onClick={() => {
                  remove(member.relationship_id);
                }}
                className={quietButtonClass}
              >
                {t('groups.remove')}
              </ActionButton>
            </li>
          ))}
        </ul>
      )}
      {archived ? (
        <p className="text-small text-text-muted">{t('groups.archivedNoNewMembers')}</p>
      ) : (
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

function needsArchiveConfirmation(group: GroupDetail): boolean {
  return group.archived_at === null && (group.future_lesson_count > 0 || group.has_open_series);
}

type GroupDialog = 'details' | 'archive' | null;

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
  const action = useAction();
  const archived = group.archived_at !== null;
  const confirming = dialog === 'archive';

  function close(): void {
    setDialog(null);
  }

  function toggle(viaDialog: boolean): void {
    onOutcome(null);
    action.run(async (): Promise<ActionState> => {
      const params = { params: { path: { id: group.id } } };
      const { data, error, response } = archived
        ? await client.POST('/api/v1/groups/{id}/unarchive', params)
        : await client.POST('/api/v1/groups/{id}/archive', params);
      if (data === undefined) {
        const refused = refusedBy(response, error);
        if (showsStaleData(refused)) {
          setDialog(null);
          handlers.onChangedElsewhere();
        } else if (!viaDialog) {
          onOutcome({ kind: 'refused', state: refused });
        }
        return refused;
      }
      setDialog(null);
      handlers.onChanged(data);
      onOutcome({ kind: archived ? 'unarchived' : 'archived' });
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
      onChoose: (): void => {
        if (needsArchiveConfirmation(group)) {
          onOutcome(null);
          action.reset();
          setDialog('archive');
          return;
        }
        toggle(false);
      },
    },
  ];

  return (
    <>
      <ActionMenu
        label={t('groups.actions')}
        busy={action.busy && !confirming}
        unavailable={action.busy}
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
        busy={action.busy}
        labelledBy={dialogHeadingId}
        describedBy={consequenceId}
        returnFocus={menuButton}
        initialFocus={dismissButton}
        onClose={close}
      >
        <div className="flex flex-col gap-4">
          <h2 id={dialogHeadingId} className="text-heading">
            {t('groups.archiveTitle')}
          </h2>
          <p id={consequenceId}>{t('groups.archiveConsequence', { count: group.future_lesson_count })}</p>
          <Refusal state={action.state} />
          <div className="flex flex-wrap gap-3">
            <ActionButton
              busy={action.busy}
              onClick={() => {
                toggle(true);
              }}
              className={dangerButtonClass}
            >
              {t('groups.archiveSubmit')}
            </ActionButton>
            <ActionButton
              ref={dismissButton}
              busy={false}
              unavailable={action.busy}
              onClick={close}
              className={textButtonClass}
            >
              {t('groups.cancel')}
            </ActionButton>
          </div>
        </div>
      </ModalDialog>
    </>
  );
}

function GroupLessons({ group }: { readonly group: GroupDetail }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-3">
      <h2 id={headingId} className="text-heading">
        {t('lessons.next')}
      </h2>
      {group.next_lessons.length === 0 ? (
        <p className="text-text-muted">{t('lessons.noNext')}</p>
      ) : (
        <div className="w-full">
          <LessonList items={group.next_lessons} label={t('lessons.next')} me={me} showAudience={false} />
        </div>
      )}
      <p>
        <Link to="/schedule" className={linkClass}>
          {t('lessons.openSchedule')}
        </Link>
      </p>
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
  if (outcome.kind === 'archived') {
    return t('groups.archivedNotice');
  }
  return outcome.kind === 'unarchived' ? t('groups.unarchivedNotice') : null;
}

function OutcomeNotice({ outcome }: { readonly outcome: GroupOutcome }): ReactElement | null {
  const { t } = useTranslation();
  if (outcome.kind === 'refused') {
    return <Refusal state={outcome.state} />;
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
  };

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
          <GroupActions
            group={shown}
            reloads={reloads}
            handlers={handlers}
            onOutcome={(next) => {
              setOutcome((previous) =>
                next === null ? null : { id: groupId, count: (previous?.count ?? 0) + 1, outcome: next },
              );
            }}
          />
        )
      }
    >
      {changed === null || !noticesShown ? null : <FocusedNotice key={changed.count} message={t('groups.changed')} />}
      {outcome === null || !noticesShown ? null : <OutcomeNotice key={outcome.count} outcome={outcome.outcome} />}
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
