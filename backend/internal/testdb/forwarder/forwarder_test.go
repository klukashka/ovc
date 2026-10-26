package forwarder_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/testdb/forwarder"
)

const (
	silenceProbe = 500 * time.Millisecond
	endOfStream  = 5 * time.Second
)

const sessionLockKey int64 = 1

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

func hijack(t *testing.T, connection *pgx.Conn) net.Conn {
	t.Helper()
	hijacked, err := connection.PgConn().Hijack()
	if err != nil {
		t.Fatalf("hijack the forwarded connection: %s", core.ErrorKind(err))
	}
	t.Cleanup(func() {
		if err := hijacked.Conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Logf("close: %s", core.ErrorKind(err))
		}
	})
	return hijacked.Conn
}

func connectedBackend(t *testing.T, relay *forwarder.Forwarder) (*pgx.Conn, uint32) {
	t.Helper()
	connection, err := connect(t, relay.URL(), endOfStream)
	if err != nil {
		t.Fatalf("connect through the forwarder: %s", core.ErrorKind(err))
	}
	return connection, connection.PgConn().PID()
}

func readUntilEndOfStream(client net.Conn) error {
	if err := client.SetReadDeadline(clock.System{}.Now().Add(endOfStream)); err != nil {
		return err
	}
	_, err := io.ReadAll(client)
	return err
}

func TestTheForwarderPassesAServerCloseOnToTheClient(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	relay := forwarder.New(t, database)
	connection, backend := connectedBackend(t, relay)
	client := hijack(t, connection)

	var terminated bool
	terminateErr := database.Pool.QueryRow(context.Background(), "SELECT pg_terminate_backend($1)", backend).Scan(&terminated)
	readErr := readUntilEndOfStream(client)

	if terminateErr != nil || !terminated {
		t.Fatalf("terminate the forwarded backend: %s", core.ErrorKind(terminateErr))
	}
	if readErr != nil {
		t.Fatalf("the client did not see the server close: %s", core.ErrorKind(readErr))
	}
}

func TestTheForwarderPassesAClientCloseOnToTheServer(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	relay := forwarder.New(t, database)
	connection, _ := connectedBackend(t, relay)
	if _, err := connection.Exec(context.Background(), "SELECT pg_advisory_lock($1)", sessionLockKey); err != nil {
		t.Fatalf("take the advisory lock through the forwarder: %s", core.ErrorKind(err))
	}
	client := hijack(t, connection)

	closeErr := client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), endOfStream)
	defer cancel()
	_, lockErr := database.Pool.Exec(ctx, "SELECT pg_advisory_lock($1)", sessionLockKey)

	if closeErr != nil {
		t.Fatalf("close the client side: %s", core.ErrorKind(closeErr))
	}
	if lockErr != nil {
		t.Fatalf("the server kept the closed client's session: %s", core.ErrorKind(lockErr))
	}
}

func TestTheForwarderPassesNoEndOfStreamOnAfterItStops(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	relay := forwarder.New(t, database)
	connection, _ := connectedBackend(t, relay)
	if _, err := connection.Exec(context.Background(), "SELECT pg_advisory_lock($1)", sessionLockKey); err != nil {
		t.Fatalf("take the advisory lock through the forwarder: %s", core.ErrorKind(err))
	}
	client := hijack(t, connection)

	relay.Stop()
	closeErr := client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), silenceProbe)
	defer cancel()
	_, lockErr := database.Pool.Exec(ctx, "SELECT pg_advisory_lock($1)", sessionLockKey)

	if closeErr != nil {
		t.Fatalf("close the client side: %s", core.ErrorKind(closeErr))
	}
	if lockErr == nil {
		t.Fatal("the stopped forwarder passed the client close on to the server")
	}
}
