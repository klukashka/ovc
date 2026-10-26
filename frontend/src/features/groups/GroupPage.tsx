import { useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type RefObject, type SyntheticEvent, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormAlert,
  FormNotice,
  SubmitButton,
  TextField,
  fieldMessage,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  stringField,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { type ApiClient, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { studentsKey } from '../students/api.ts';
import {
  type Group,
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

function DetailsForm({
  group,
  reloads,
  handlers,
}: {
  readonly group: Group;
  readonly reloads: number;
  readonly handlers: GroupHandlers;
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
          handlers.onChangedElsewhere();
        }
        return answer;
      }
      handlers.onChanged(answer);
      return done;
    });
  }

  return (
    <form
      aria-labelledby={headingId}
      onSubmit={submit}
      onChange={action.reset}
      className="flex max-w-sm flex-col gap-4"
    >
      <h2 id={headingId} className="text-heading">
        {t('groups.details')}
      </h2>
      <Refusal state={action.state} />
      {action.state.kind === 'done' ? <FormNotice takeFocus>{t('groups.saved')}</FormNotice> : null}
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
      <p>
        <SubmitButton busy={action.busy} quiet>
          {t('groups.save')}
        </SubmitButton>
      </p>
    </form>
  );
}

function AddMembersDialog({
  group,
  open,
  returnFocus,
  onClose,
  onAdded,
  onChangedElsewhere,
}: {
  readonly group: Group;
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

function Members({ group, handlers }: { readonly group: Group; readonly handlers: GroupHandlers }): ReactElement {
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
                  className="truncate text-strong text-accent-text underline"
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

function ArchiveSection({
  group,
  handlers,
}: {
  readonly group: Group;
  readonly handlers: GroupHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const action = useAction();
  const archived = group.archived_at !== null;

  function toggle(): void {
    action.run(async (): Promise<ActionState> => {
      const params = { params: { path: { id: group.id } } };
      const { data, error, response } = archived
        ? await client.POST('/api/v1/groups/{id}/unarchive', params)
        : await client.POST('/api/v1/groups/{id}/archive', params);
      if (data === undefined) {
        const refused = refusedBy(response, error);
        if (showsStaleData(refused)) {
          handlers.onChangedElsewhere();
        }
        return refused;
      }
      handlers.onChanged(data);
      return done;
    });
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-3">
      <h2 id={headingId} className="text-heading">
        {t('groups.archive')}
      </h2>
      <p className="text-small text-text-muted">{t(archived ? 'groups.unarchiveHint' : 'groups.archiveHint')}</p>
      <Refusal state={action.state} />
      {action.state.kind === 'done' ? (
        <FormNotice takeFocus>{t(archived ? 'groups.archivedNotice' : 'groups.unarchivedNotice')}</FormNotice>
      ) : null}
      <ActionButton busy={action.busy} onClick={toggle} className={quietButtonClass}>
        {t(archived ? 'groups.unarchiveSubmit' : 'groups.archiveSubmit')}
      </ActionButton>
    </section>
  );
}

export function GroupView({
  group,
  reloads = 0,
  handlers,
}: {
  readonly group: Group;
  readonly reloads?: number;
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
        {t(`colours.${group.colour}`)}
        {group.archived_at === null ? null : <StatusWord tone="muted">{t('groups.archived')}</StatusWord>}
      </p>
      <Members group={group} handlers={handlers} />
      <DetailsForm group={group} reloads={reloads} handlers={handlers} />
      <ArchiveSection group={group} handlers={handlers} />
    </>
  );
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
  const error = query.isError ? asApiError(query.error) : null;
  const notFound = error?.code === 'not_found';

  function body(): ReactElement {
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
    if (query.data === undefined) {
      return <DelayedLoading />;
    }
    return (
      <GroupView
        group={query.data}
        reloads={reloads}
        handlers={{
          onChanged: (group) => {
            queryClient.setQueryData(key, group);
            void queryClient.invalidateQueries({ queryKey: groupsKey, refetchType: 'none' });
            void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
          },
          onChangedElsewhere: () => {
            setChanged((previous) => ({ id: groupId, count: (previous?.count ?? 0) + 1 }));
            void queryClient.invalidateQueries({ queryKey: groupsKey }).then(() => {
              setReloads((count) => count + 1);
            });
          },
        }}
      />
    );
  }

  return (
    <Page
      {...(notFound ? { title: 'page.notFound' as const } : {})}
      {...(error === null && query.data !== undefined ? { headingText: query.data.name } : {})}
      focusTitle={notFound}
    >
      {changed === null || notFound ? null : <FocusedNotice key={changed.count} message={t('groups.changed')} />}
      {body()}
    </Page>
  );
}
