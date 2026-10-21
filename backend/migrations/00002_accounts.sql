-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id uuid PRIMARY KEY,
    role text NOT NULL CHECK (role IN ('tutor', 'student')),
    state text NOT NULL CHECK (state IN ('unverified', 'active', 'blocked', 'deleted')),
    state_before_block text CHECK (state_before_block IN ('unverified', 'active')),
    email citext NOT NULL UNIQUE CHECK (char_length(email) BETWEEN 3 AND 254),
    email_verified_at timestamptz,
    password_hash text NOT NULL CHECK (password_hash LIKE '$argon2id$%'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    language text NOT NULL CHECK (language IN ('ru', 'en')),
    timezone text NOT NULL CHECK (char_length(timezone) BETWEEN 1 AND 64),
    theme text NOT NULL CHECK (theme IN ('light', 'dark', 'system')),
    consent_version text NOT NULL CHECK (char_length(consent_version) BETWEEN 1 AND 32),
    consent_at timestamptz NOT NULL,
    unverified_since timestamptz NOT NULL,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK ((state = 'blocked') = (state_before_block IS NOT NULL)),
    CHECK ((state = 'deleted') = (deleted_at IS NOT NULL)),
    CHECK (state <> 'active' OR email_verified_at IS NOT NULL)
);

CREATE INDEX users_deleted_at_idx ON users (deleted_at) WHERE state = 'deleted';
CREATE INDEX users_unverified_since_idx ON users (unverified_since) WHERE state = 'unverified';

CREATE TABLE tutor_profiles (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    bio text NOT NULL DEFAULT '' CHECK (char_length(bio) <= 1000),
    subjects text[] NOT NULL DEFAULT '{}' CHECK (cardinality(subjects) <= 10),
    recording_retention_days smallint NOT NULL DEFAULT 90 CHECK (recording_retention_days IN (7, 30, 90, 180, 365)),
    file_quota_bytes bigint NOT NULL CHECK (file_quota_bytes >= 0),
    recording_quota_bytes bigint NOT NULL CHECK (recording_quota_bytes >= 0),
    file_usage_bytes bigint NOT NULL DEFAULT 0 CHECK (file_usage_bytes >= 0),
    recording_usage_bytes bigint NOT NULL DEFAULT 0 CHECK (recording_usage_bytes >= 0),
    onboarding_dismissed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    generation bigint NOT NULL CHECK (generation >= 1),
    refresh_hash bytea NOT NULL CHECK (octet_length(refresh_hash) = 32),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    last_used_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

CREATE TABLE email_requests (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose text NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    created_at timestamptz NOT NULL,
    UNIQUE (id, user_id, purpose)
);

CREATE INDEX email_requests_user_purpose_idx ON email_requests (user_id, purpose, created_at DESC);

CREATE TABLE email_tokens (
    id uuid PRIMARY KEY,
    request_id uuid NOT NULL,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose text NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL,
    FOREIGN KEY (request_id, user_id, purpose) REFERENCES email_requests (id, user_id, purpose) ON DELETE CASCADE
);

CREATE INDEX email_tokens_user_purpose_idx ON email_tokens (user_id, purpose);
CREATE INDEX email_tokens_request_id_idx ON email_tokens (request_id);

CREATE TABLE rate_limit_counters (
    purpose text NOT NULL CHECK (char_length(purpose) BETWEEN 1 AND 40),
    subject bytea NOT NULL CHECK (octet_length(subject) = 32),
    window_start timestamptz NOT NULL,
    count integer NOT NULL CHECK (count >= 0),
    PRIMARY KEY (purpose, subject, window_start)
);

CREATE INDEX rate_limit_counters_window_start_idx ON rate_limit_counters (window_start);

CREATE TABLE attachments (
    id uuid PRIMARY KEY,
    uploader_id uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    context text NOT NULL CHECK (context IN ('avatar', 'prompt_image', 'library', 'chat', 'answer', 'corrected')),
    scope_kind text CHECK (scope_kind IN ('channel', 'submission')),
    scope_id uuid,
    scope_question_id uuid,
    quota_tutor_id uuid REFERENCES tutor_profiles (user_id) ON DELETE RESTRICT,
    storage_key text NOT NULL UNIQUE CHECK (char_length(storage_key) BETWEEN 1 AND 200),
    filename text NOT NULL CHECK (char_length(filename) BETWEEN 1 AND 255),
    media_type text NOT NULL CHECK (char_length(media_type) BETWEEN 1 AND 100),
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),
    state text NOT NULL CHECK (state IN ('pending', 'bound', 'deleted')),
    expires_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK ((state = 'pending') = (expires_at IS NOT NULL)),
    CHECK ((scope_kind IS NULL) = (scope_id IS NULL)),
    CHECK (scope_question_id IS NULL OR scope_kind = 'submission')
);

CREATE INDEX attachments_pending_expires_at_idx ON attachments (expires_at) WHERE state = 'pending';
CREATE INDEX attachments_uploader_id_idx ON attachments (uploader_id);
CREATE INDEX attachments_quota_tutor_id_idx ON attachments (quota_tutor_id);

CREATE TABLE attachment_references (
    attachment_id uuid NOT NULL REFERENCES attachments (id) ON DELETE CASCADE,
    ref_kind text NOT NULL CHECK (ref_kind IN ('message', 'answer', 'review_item', 'material', 'template', 'issue_snapshot', 'lesson', 'avatar')),
    ref_id uuid NOT NULL,
    question_id uuid,
    created_at timestamptz NOT NULL,
    CONSTRAINT attachment_references_key UNIQUE NULLS NOT DISTINCT (attachment_id, ref_kind, ref_id, question_id)
);

CREATE INDEX attachment_references_ref_idx ON attachment_references (ref_kind, ref_id);
CREATE UNIQUE INDEX attachment_references_one_avatar_idx ON attachment_references (ref_id) WHERE ref_kind = 'avatar';

CREATE TABLE outbox (
    id uuid PRIMARY KEY,
    lane text NOT NULL CHECK (lane IN ('email', 'provider_call', 'recording_copy', 'storage_delete')),
    priority smallint NOT NULL CHECK (priority IN (0, 1)),
    kind text NOT NULL CHECK (char_length(kind) BETWEEN 1 AND 64),
    payload jsonb NOT NULL,
    idempotency_key text NOT NULL UNIQUE CHECK (char_length(idempotency_key) BETWEEN 1 AND 200),
    status text NOT NULL CHECK (status IN ('pending', 'in_progress', 'done', 'dead')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL,
    lease_token uuid,
    lease_expires_at timestamptz,
    last_error_code text CHECK (char_length(last_error_code) BETWEEN 1 AND 64),
    done_reason text CHECK (done_reason IN ('sent', 'obsolete')),
    done_at timestamptz,
    created_at timestamptz NOT NULL,
    CHECK ((status = 'in_progress') = (lease_token IS NOT NULL)),
    CHECK ((lease_token IS NULL) = (lease_expires_at IS NULL)),
    CHECK ((status IN ('done', 'dead')) = (done_at IS NOT NULL)),
    CHECK ((status = 'done') = (done_reason IS NOT NULL))
);

CREATE INDEX outbox_pending_idx ON outbox (lane, priority, next_attempt_at) WHERE status = 'pending';
CREATE INDEX outbox_in_progress_idx ON outbox (lease_expires_at) WHERE status = 'in_progress';
CREATE INDEX outbox_finished_idx ON outbox (done_at) WHERE status IN ('done', 'dead');

CREATE TABLE job_runs (
    job text PRIMARY KEY CHECK (char_length(job) BETWEEN 1 AND 64),
    last_run_at timestamptz NOT NULL,
    last_result text NOT NULL CHECK (last_result IN ('ok', 'failed'))
);

CREATE TABLE mail_sink (
    id uuid PRIMARY KEY,
    recipient citext NOT NULL,
    subject text NOT NULL,
    text_body text NOT NULL,
    html_body text NOT NULL,
    links text[] NOT NULL,
    message_id text NOT NULL,
    sent_at timestamptz NOT NULL
);

CREATE INDEX mail_sink_recipient_sent_at_idx ON mail_sink (recipient, sent_at);

-- +goose Down
DROP TABLE mail_sink;
DROP TABLE job_runs;
DROP TABLE outbox;
DROP TABLE attachment_references;
DROP TABLE attachments;
DROP TABLE rate_limit_counters;
DROP TABLE email_tokens;
DROP TABLE email_requests;
DROP TABLE sessions;
DROP TABLE tutor_profiles;
DROP TABLE users;
