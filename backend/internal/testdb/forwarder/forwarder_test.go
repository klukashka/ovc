package forwarder_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/core"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/testdb/forwarder"
)

const silenceProbe = 500 * time.Millisecond

func connect(t *testing.T, url core.Secret, timeout time.Duration) (*pgx.Conn, error) {
	t.Helper()
	config, err := pgx.ParseConfig(url.Reveal())
	if err != nil {
		t.Fatal("the forwarded URL does not parse")
	}
	config.ConnectTimeout = timeout
	connection, err := pgx.ConnectConfig(context.Background(), config)
	if err == nil {
		t.Cleanup(func() {
			if err := connection.Close(context.Background()); err != nil {
				t.Logf("close: %s", core.ErrorKind(err))
			}
		})
	}
	return connection, err
}

func echo(connection *pgx.Conn, payload []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var echoed []byte
	err := connection.QueryRow(ctx, "SELECT $1::bytea", payload).Scan(&echoed)
	return echoed, err
}

func TestTheForwarderRelaysEveryByteUnchangedUntilItStops(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	relay := forwarder.New(t, database)
	connection, err := connect(t, relay.URL(), 5*time.Second)
	if err != nil {
		t.Fatalf("connect through the forwarder: %s", core.ErrorKind(err))
	}
	payload := make([]byte, 512*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}

	echoed, echoErr := echo(connection, payload, 10*time.Second)
	relay.Stop()
	_, silentErr := echo(connection, payload[:16], silenceProbe)
	_, refusedErr := connect(t, relay.URL(), silenceProbe)

	if echoErr != nil || !bytes.Equal(echoed, payload) {
		t.Fatalf("the round trip through the forwarder changed the bytes: %s", core.ErrorKind(echoErr))
	}
	if silentErr == nil {
		t.Fatal("a query was answered after the forwarder stopped")
	}
	if refusedErr == nil {
		t.Fatal("a new connection was relayed after the forwarder stopped")
	}
	if relay.URL().Reveal() == database.URL.Reveal() {
		t.Fatal("the forwarder URL points at the database itself")
	}
}
