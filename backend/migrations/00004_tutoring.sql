-- +goose Up
ALTER TABLE tutor_profiles ADD COLUMN code text;

WITH drawn AS (
    SELECT profile.user_id, position, uuid_send(gen_random_uuid()) AS bytes
    FROM tutor_profiles AS profile
    CROSS JOIN generate_series(1, 8) AS position
),
minted AS (
    SELECT
        user_id,
        string_agg(
            substr('23456789ABCDEFGHJKMNPQRSTUVWXYZ', (get_byte(bytes, 0) * 256 + get_byte(bytes, 1)) % 31 + 1, 1),
            '' ORDER BY position
        ) AS code
    FROM drawn
    GROUP BY user_id
)
UPDATE tutor_profiles AS profile
SET code = minted.code
FROM minted
WHERE minted.user_id = profile.user_id;

ALTER TABLE tutor_profiles ALTER COLUMN code SET NOT NULL;
ALTER TABLE tutor_profiles ADD CONSTRAINT tutor_profiles_code_key UNIQUE (code);
ALTER TABLE tutor_profiles ADD CONSTRAINT tutor_profiles_code_check CHECK (code ~ '^[2-9A-HJKMNP-Z]{8}$');

ALTER TABLE users ADD COLUMN reclaim_until timestamptz;

CREATE INDEX users_reclaim_until_idx ON users (reclaim_until) WHERE reclaim_until IS NOT NULL;

CREATE TABLE relationships (
    id uuid PRIMARY KEY,
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE CASCADE,
    student_id uuid REFERENCES users (id) ON DELETE RESTRICT,
    state text NOT NULL CHECK (state IN ('invited', 'provisional', 'reclaim_pending', 'active', 'ended', 'discarded')),
    discard_reason text CHECK (discard_reason IN ('withdrawn', 'merged', 'removed', 'tutor_deleted', 'student_purged')),
    ended_by text CHECK (ended_by IN ('tutor', 'student', 'deletion')),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 100),
    email citext CHECK (char_length(email) BETWEEN 3 AND 254),
    colour text CHECK (colour IN ('clay', 'slate', 'moss', 'plum', 'ochre', 'teal', 'rust', 'ink')),
    notes text NOT NULL DEFAULT '' CHECK (char_length(notes) <= 10000),
    create_key text CHECK (char_length(create_key) BETWEEN 1 AND 200),
    linked_at timestamptz,
    ended_at timestamptz,
    discarded_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT relationships_id_tutor_key UNIQUE (id, tutor_id),
    CONSTRAINT relationships_invited_unlinked CHECK (
        state <> 'invited' OR (student_id IS NULL AND colour IS NULL AND linked_at IS NULL)
    ),
    CONSTRAINT relationships_linked_states_linked CHECK (
        state NOT IN ('provisional', 'reclaim_pending', 'active', 'ended')
        OR (student_id IS NOT NULL AND colour IS NOT NULL AND linked_at IS NOT NULL)
    ),
    CONSTRAINT relationships_discard_reason_when_discarded CHECK ((state = 'discarded') = (discard_reason IS NOT NULL)),
    CONSTRAINT relationships_discarded_at_when_discarded CHECK ((state = 'discarded') = (discarded_at IS NOT NULL)),
    CONSTRAINT relationships_ending_fields_together CHECK ((ended_at IS NULL) = (ended_by IS NULL)),
    CONSTRAINT relationships_ending_fields_when_ended CHECK (
        CASE
            WHEN state = 'ended' THEN ended_at IS NOT NULL
            WHEN state = 'discarded' AND discard_reason = 'student_purged' THEN true
            ELSE ended_at IS NULL
        END
    )
);

CREATE UNIQUE INDEX relationships_tutor_student_idx ON relationships (tutor_id, student_id)
    WHERE student_id IS NOT NULL AND state <> 'discarded';
CREATE UNIQUE INDEX relationships_tutor_email_idx ON relationships (tutor_id, email)
    WHERE email IS NOT NULL AND state <> 'discarded';
CREATE UNIQUE INDEX relationships_one_provisional_idx ON relationships (student_id) WHERE state = 'provisional';
CREATE INDEX relationships_tutor_list_idx ON relationships (tutor_id, state, created_at DESC, id DESC);
CREATE INDEX relationships_student_id_idx ON relationships (student_id);
CREATE UNIQUE INDEX relationships_tutor_create_key_idx ON relationships (tutor_id, create_key) WHERE create_key IS NOT NULL;

CREATE TABLE invitations (
    id uuid PRIMARY KEY,
    relationship_id uuid NOT NULL REFERENCES relationships (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    token_encrypted bytea CHECK (octet_length(token_encrypted) BETWEEN 1 AND 512),
    bound_student_id uuid REFERENCES users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    used_by uuid REFERENCES users (id) ON DELETE SET NULL,
    revoked_at timestamptz,
    revoke_reason text CHECK (
        revoke_reason IN (
            'regenerated', 'withdrawn', 'email_changed', 'mismatch_limit', 'linked', 'removed', 'tutor_deleted', 'expired_on_send'
        )
    ),
    mismatch_count smallint NOT NULL DEFAULT 0 CHECK (mismatch_count BETWEEN 0 AND 5),
    send_count smallint NOT NULL DEFAULT 0 CHECK (send_count >= 0),
    created_at timestamptz NOT NULL,
    CONSTRAINT invitations_ciphertext_only_while_open CHECK (token_encrypted IS NULL OR (used_at IS NULL AND revoked_at IS NULL)),
    CONSTRAINT invitations_revoke_reason_when_revoked CHECK ((revoked_at IS NULL) = (revoke_reason IS NULL)),
    CONSTRAINT invitations_used_or_revoked CHECK (used_at IS NULL OR revoked_at IS NULL),
    CONSTRAINT invitations_used_by_only_when_used CHECK (used_by IS NULL OR used_at IS NOT NULL),
    CONSTRAINT invitations_mismatch_limit_revokes CHECK ((mismatch_count = 5) = (revoke_reason IS NOT DISTINCT FROM 'mismatch_limit'))
);

CREATE UNIQUE INDEX invitations_one_open_idx ON invitations (relationship_id) WHERE used_at IS NULL AND revoked_at IS NULL;
CREATE INDEX invitations_relationship_id_idx ON invitations (relationship_id);
CREATE INDEX invitations_bound_student_id_idx ON invitations (bound_student_id);
CREATE INDEX invitations_used_by_idx ON invitations (used_by);
CREATE INDEX invitations_readable_expires_at_idx ON invitations (expires_at) WHERE token_encrypted IS NOT NULL;

CREATE TABLE join_requests (
    id uuid PRIMARY KEY,
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE CASCADE,
    student_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    state text NOT NULL CHECK (state IN ('held', 'pending', 'accepted', 'declined', 'expired')),
    suppressed text NOT NULL DEFAULT 'none' CHECK (suppressed IN ('none', 'blocked', 'limit')),
    owner_choice_pending boolean NOT NULL DEFAULT false,
    expires_at timestamptz NOT NULL,
    decided_at timestamptz,
    created_at timestamptz NOT NULL,
    CONSTRAINT join_requests_owner_choice_only_while_held CHECK (NOT owner_choice_pending OR state = 'held'),
    CONSTRAINT join_requests_suppressed_only_while_pending CHECK (suppressed = 'none' OR state = 'pending'),
    CONSTRAINT join_requests_decided_at_when_closed CHECK ((state IN ('held', 'pending')) = (decided_at IS NULL))
);

CREATE UNIQUE INDEX join_requests_open_idx ON join_requests (tutor_id, student_id) WHERE state IN ('held', 'pending');
CREATE INDEX join_requests_open_expires_at_idx ON join_requests (expires_at) WHERE state IN ('held', 'pending');
CREATE INDEX join_requests_visible_idx ON join_requests (tutor_id, created_at, id) WHERE state = 'pending' AND suppressed = 'none';
CREATE INDEX join_requests_over_limit_idx ON join_requests (tutor_id, created_at, id) WHERE state = 'pending' AND suppressed = 'limit';
CREATE INDEX join_requests_tutor_id_idx ON join_requests (tutor_id);
CREATE INDEX join_requests_student_idx ON join_requests (student_id, created_at DESC, id DESC);

CREATE TABLE blocked_requesters (
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE CASCADE,
    student_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tutor_id, student_id)
);

CREATE INDEX blocked_requesters_tutor_list_idx ON blocked_requesters (tutor_id, created_at DESC, student_id);
CREATE INDEX blocked_requesters_student_id_idx ON blocked_requesters (student_id);

CREATE TABLE groups (
    id uuid PRIMARY KEY,
    tutor_id uuid NOT NULL REFERENCES tutor_profiles (user_id) ON DELETE CASCADE,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    colour text NOT NULL CHECK (colour IN ('clay', 'slate', 'moss', 'plum', 'ochre', 'teal', 'rust', 'ink')),
    archived_at timestamptz,
    create_key text CHECK (char_length(create_key) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT groups_id_tutor_key UNIQUE (id, tutor_id)
);

CREATE INDEX groups_tutor_id_idx ON groups (tutor_id);
CREATE UNIQUE INDEX groups_tutor_create_key_idx ON groups (tutor_id, create_key) WHERE create_key IS NOT NULL;

CREATE TABLE group_members (
    group_id uuid NOT NULL,
    relationship_id uuid NOT NULL,
    tutor_id uuid NOT NULL,
    added_at timestamptz NOT NULL,
    PRIMARY KEY (group_id, relationship_id),
    CONSTRAINT group_members_group_fkey FOREIGN KEY (group_id, tutor_id) REFERENCES groups (id, tutor_id) ON DELETE CASCADE,
    CONSTRAINT group_members_relationship_fkey FOREIGN KEY (relationship_id, tutor_id)
        REFERENCES relationships (id, tutor_id) ON DELETE CASCADE
);

CREATE INDEX group_members_relationship_id_idx ON group_members (relationship_id);

-- +goose Down
DROP TABLE group_members;
DROP TABLE groups;
DROP TABLE blocked_requesters;
DROP TABLE join_requests;
DROP TABLE invitations;
DROP TABLE relationships;
DROP INDEX users_reclaim_until_idx;
ALTER TABLE users DROP COLUMN reclaim_until;
ALTER TABLE tutor_profiles DROP COLUMN code;
