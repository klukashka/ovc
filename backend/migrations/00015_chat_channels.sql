-- +goose Up
CREATE TABLE channels (
    id uuid PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('direct', 'group')),
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE RESTRICT,
    relationship_id uuid UNIQUE,
    group_id uuid UNIQUE,
    last_seq bigint NOT NULL DEFAULT 0 CHECK (last_seq >= 0),
    last_activity_at timestamptz NOT NULL,
    read_only_since timestamptz,
    provisional_through_seq bigint CHECK (provisional_through_seq >= 0),
    discarded_at timestamptz,
    erase_pending_since timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT channels_audience_xor CHECK (num_nonnulls(relationship_id, group_id) = 1),
    CONSTRAINT channels_kind_matches_audience CHECK ((kind = 'direct') = (relationship_id IS NOT NULL)),
    CONSTRAINT channels_only_direct_discarded CHECK (discarded_at IS NULL OR kind = 'direct'),
    CONSTRAINT channels_erase_only_when_discarded CHECK (erase_pending_since IS NULL OR discarded_at IS NOT NULL),
    CONSTRAINT channels_relationship_fkey FOREIGN KEY (relationship_id, tutor_id)
        REFERENCES relationships (id, tutor_id) ON DELETE RESTRICT,
    CONSTRAINT channels_group_fkey FOREIGN KEY (group_id, tutor_id) REFERENCES groups (id, tutor_id) ON DELETE RESTRICT
);

CREATE INDEX channels_tutor_id_idx ON channels (tutor_id, id);
CREATE INDEX channels_erase_pending_idx ON channels (erase_pending_since, id) WHERE erase_pending_since IS NOT NULL;

CREATE TABLE channel_members (
    channel_id uuid NOT NULL REFERENCES channels (id) ON DELETE RESTRICT,
    user_id uuid NOT NULL,
    last_read_seq bigint NOT NULL DEFAULT 0 CHECK (last_read_seq >= 0),
    last_read_at timestamptz,
    digested_through_seq bigint NOT NULL DEFAULT 0 CHECK (digested_through_seq >= 0),
    suspended_since timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX channel_members_user_id_idx ON channel_members (user_id, channel_id);

CREATE VIEW active_channel_members AS
SELECT channel_members.channel_id, channel_members.user_id, channel_members.last_read_seq, channel_members.digested_through_seq,
    channels.kind, channels.tutor_id, channels.relationship_id, channels.group_id, channels.last_seq, channels.last_activity_at,
    channels.read_only_since, channels.provisional_through_seq, channels.created_at
FROM channel_members
JOIN channels ON channels.id = channel_members.channel_id
WHERE channel_members.suspended_since IS NULL AND channels.discarded_at IS NULL AND channels.erase_pending_since IS NULL;

CREATE INDEX attachments_pending_scope_idx ON attachments (scope_kind, scope_id) WHERE state = 'pending' AND scope_kind IS NOT NULL;

INSERT INTO channels (id, kind, tutor_id, relationship_id, last_activity_at, read_only_since, provisional_through_seq, created_at, updated_at)
SELECT gen_random_uuid(), 'direct', relationships.tutor_id, relationships.id, COALESCE(relationships.linked_at, relationships.created_at),
    CASE WHEN relationships.state = 'ended' THEN relationships.ended_at END,
    CASE WHEN relationships.state = 'reclaim_pending' THEN 0 END,
    COALESCE(relationships.linked_at, relationships.created_at), COALESCE(relationships.linked_at, relationships.created_at)
FROM relationships
WHERE relationships.state IN ('provisional', 'active', 'ended', 'reclaim_pending');

INSERT INTO channels (id, kind, tutor_id, group_id, last_activity_at, read_only_since, created_at, updated_at)
SELECT gen_random_uuid(), 'group', groups.tutor_id, groups.id, groups.created_at, groups.archived_at, groups.created_at, groups.created_at
FROM groups;

INSERT INTO channel_members (channel_id, user_id, suspended_since, created_at, updated_at)
SELECT channels.id, relationships.tutor_id, NULL, channels.created_at, channels.created_at
FROM channels
JOIN relationships ON relationships.id = channels.relationship_id
UNION ALL
SELECT channels.id, relationships.student_id,
    CASE WHEN relationships.state = 'reclaim_pending' THEN relationships.updated_at END, channels.created_at, channels.created_at
FROM channels
JOIN relationships ON relationships.id = channels.relationship_id
WHERE relationships.student_id IS NOT NULL;

INSERT INTO channel_members (channel_id, user_id, created_at, updated_at)
SELECT channels.id, channels.tutor_id, channels.created_at, channels.created_at
FROM channels
WHERE channels.kind = 'group'
UNION ALL
SELECT channels.id, relationships.student_id, group_members.added_at, group_members.added_at
FROM channels
JOIN group_members ON group_members.group_id = channels.group_id AND group_members.tutor_id = channels.tutor_id
JOIN relationships ON relationships.id = group_members.relationship_id AND relationships.tutor_id = group_members.tutor_id
WHERE channels.kind = 'group' AND relationships.student_id IS NOT NULL;

-- +goose Down
DROP INDEX attachments_pending_scope_idx;
DROP VIEW active_channel_members;
DROP TABLE channel_members;
DROP TABLE channels;
