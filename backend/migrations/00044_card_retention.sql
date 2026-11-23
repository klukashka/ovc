-- +goose Up
ALTER TABLE relationships
    ALTER COLUMN display_name DROP NOT NULL,
    DROP CONSTRAINT relationships_display_name_check,
    ADD COLUMN email_set_at timestamptz,
    ADD COLUMN email_erased_at timestamptz;
UPDATE relationships SET email_set_at = updated_at WHERE email IS NOT NULL;
ALTER TABLE relationships
    ADD CONSTRAINT relationships_display_name_check CHECK (
        CASE WHEN display_name IS NULL THEN state = 'discarded' ELSE char_length(display_name) BETWEEN 1 AND 100 END
    ),
    ADD CONSTRAINT relationships_email_set_at_with_email CHECK ((email IS NULL) = (email_set_at IS NULL)),
    ADD CONSTRAINT relationships_email_erased_only_invited CHECK (
        email_erased_at IS NULL OR (state = 'invited' AND email IS NULL)
    );
CREATE INDEX relationships_email_erasure_due_idx ON relationships (email_set_at, id)
    WHERE state = 'invited' AND email IS NOT NULL;

ALTER TABLE invitations DROP CONSTRAINT invitations_revoke_reason_check;
ALTER TABLE invitations ADD CONSTRAINT invitations_revoke_reason_check CHECK (
    revoke_reason IN (
        'regenerated', 'withdrawn', 'email_changed', 'mismatch_limit', 'linked', 'removed', 'tutor_deleted', 'expired_on_send',
        'email_erased'
    )
);

-- +goose Down
ALTER TABLE invitations DROP CONSTRAINT invitations_revoke_reason_check;
UPDATE invitations SET revoke_reason = 'regenerated' WHERE revoke_reason = 'email_erased';
ALTER TABLE invitations ADD CONSTRAINT invitations_revoke_reason_check CHECK (
    revoke_reason IN (
        'regenerated', 'withdrawn', 'email_changed', 'mismatch_limit', 'linked', 'removed', 'tutor_deleted', 'expired_on_send'
    )
);

DROP INDEX relationships_email_erasure_due_idx;
ALTER TABLE relationships
    DROP CONSTRAINT relationships_email_erased_only_invited,
    DROP CONSTRAINT relationships_email_set_at_with_email,
    DROP CONSTRAINT relationships_display_name_check,
    DROP COLUMN email_erased_at,
    DROP COLUMN email_set_at;
UPDATE relationships SET display_name = 'removed' WHERE display_name IS NULL;
ALTER TABLE relationships
    ALTER COLUMN display_name SET NOT NULL,
    ADD CONSTRAINT relationships_display_name_check CHECK (char_length(display_name) BETWEEN 1 AND 100);
