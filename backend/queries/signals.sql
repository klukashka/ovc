-- name: NotifySignal :exec
SELECT pg_notify(sqlc.arg(channel)::text, sqlc.arg(payload)::text);

-- name: NotifySignals :exec
SELECT pg_notify(sqlc.arg(channel)::text, signals.payload)
FROM unnest(sqlc.arg(payloads)::text[]) AS signals (payload);
