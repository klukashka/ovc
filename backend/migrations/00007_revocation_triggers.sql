-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION signal_session_revoked() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('ovc_realtime', json_build_object('v', 1, 'k', 'recheck', 'u', json_build_array(NEW.user_id))::text);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION signal_account_ended() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('ovc_realtime', json_build_object('v', 1, 'k', 'recheck', 'u', json_build_array(NEW.id))::text);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION signal_account_removed() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('ovc_realtime', json_build_object('v', 1, 'k', 'recheck', 'u', json_build_array(OLD.id))::text);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER sessions_revoked_signal
    AFTER UPDATE OF revoked_at ON sessions
    FOR EACH ROW
    WHEN (OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL)
    EXECUTE FUNCTION signal_session_revoked();

CREATE TRIGGER users_ended_signal
    AFTER UPDATE OF state ON users
    FOR EACH ROW
    WHEN (NEW.state IN ('blocked', 'deleted') AND OLD.state IS DISTINCT FROM NEW.state)
    EXECUTE FUNCTION signal_account_ended();

CREATE TRIGGER users_removed_signal
    AFTER DELETE ON users
    FOR EACH ROW
    EXECUTE FUNCTION signal_account_removed();

-- +goose Down
DROP TRIGGER users_removed_signal ON users;
DROP TRIGGER users_ended_signal ON users;
DROP TRIGGER sessions_revoked_signal ON sessions;
DROP FUNCTION signal_account_removed();
DROP FUNCTION signal_account_ended();
DROP FUNCTION signal_session_revoked();
