package forwarder

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"ovc/backend/internal/core"
	"ovc/backend/internal/testdb"
)

type Forwarder struct {
	url      core.Secret
	network  string
	address  string
	listener net.Listener
	relaying sync.RWMutex
	stopped  bool
	mu       sync.Mutex
	closed   bool
	open     []net.Conn
	done     sync.WaitGroup
	logger   *slog.Logger
}

type testLog struct {
	tb testing.TB
}

func (w testLog) Write(record []byte) (int, error) {
	w.tb.Log(strings.TrimSpace(string(record)))
	return len(record), nil
}

func (f *Forwarder) start(work func()) {
	f.done.Add(1)
	core.Go(context.Background(), f.logger, "testdb.forwarder", func(context.Context) {
		defer f.done.Done()
		work()
	})
}

const bufferBytes = 32 * 1024

func New(tb testing.TB, database *testdb.Database) *Forwarder {
	tb.Helper()
	ctx := context.Background()
	if err := core.VerifyLocalDatabase(ctx, database.URL); err != nil {
		tb.Fatalf("forwarder: the test database is not local: %s", core.ErrorKind(err))
	}
	config, err := pgx.ParseConfig(database.URL.Reveal())
	if err != nil {
		tb.Fatal("forwarder: the test database URL does not parse")
	}
	if len(config.Fallbacks) > 0 {
		tb.Fatal("forwarder: the test database URL names more than one host")
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("forwarder: listen on loopback: %s", core.ErrorKind(err))
	}
	forwarded, err := withAddress(database.URL, listener.Addr().String())
	if err != nil {
		tb.Fatal("forwarder: rewrite the test database URL")
	}
	network, address := pgconn.NetworkAddress(config.Host, config.Port)
	f := &Forwarder{url: forwarded, network: network, address: address, listener: listener}
	f.logger = slog.New(slog.NewTextHandler(testLog{tb: tb}, nil))
	f.start(func() { f.accept(tb) })
	tb.Cleanup(func() { f.shutdown(tb) })
	return f
}

func withAddress(databaseURL core.Secret, address string) (core.Secret, error) {
	parsed, err := url.Parse(databaseURL.Reveal())
	if err != nil {
		return core.Secret{}, err
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return core.Secret{}, err
	}
	parsed.Host = net.JoinHostPort(host, port)
	query := parsed.Query()
	query.Del("host")
	query.Del("port")
	parsed.RawQuery = query.Encode()
	return core.NewSecret(parsed.String()), nil
}

func (f *Forwarder) URL() core.Secret {
	return f.url
}

func (f *Forwarder) Stop() {
	f.relaying.Lock()
	defer f.relaying.Unlock()
	f.stopped = true
}

func (f *Forwarder) accept(tb testing.TB) {
	for {
		client, err := f.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				tb.Errorf("forwarder: accept: %s", core.ErrorKind(err))
			}
			return
		}
		if !f.track(tb, client) || f.isStopped() {
			continue
		}
		upstream, err := (&net.Dialer{}).DialContext(context.Background(), f.network, f.address)
		if err != nil {
			tb.Errorf("forwarder: dial the test database: %s", core.ErrorKind(err))
			continue
		}
		if !f.track(tb, upstream) {
			continue
		}
		f.start(func() { f.relay(tb, client, upstream) })
		f.start(func() { f.relay(tb, upstream, client) })
	}
}

func (f *Forwarder) isStopped() bool {
	f.relaying.RLock()
	defer f.relaying.RUnlock()
	return f.stopped
}

func (f *Forwarder) track(tb testing.TB, connection net.Conn) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		closeConnection(tb, connection)
		return false
	}
	f.open = append(f.open, connection)
	return true
}

func closeConnection(tb testing.TB, connection net.Conn) {
	if err := connection.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		tb.Errorf("forwarder: close a connection: %s", core.ErrorKind(err))
	}
}

func (f *Forwarder) relay(tb testing.TB, from net.Conn, to net.Conn) {
	buffer := make([]byte, bufferBytes)
	for {
		read, err := from.Read(buffer)
		if read > 0 && !f.write(tb, to, buffer[:read]) {
			return
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				tb.Logf("forwarder: read ended: %s", core.ErrorKind(err))
			}
			return
		}
	}
}

func (f *Forwarder) write(tb testing.TB, to net.Conn, data []byte) bool {
	f.relaying.RLock()
	defer f.relaying.RUnlock()
	if f.stopped {
		return false
	}
	if _, err := to.Write(data); err != nil {
		if !errors.Is(err, net.ErrClosed) {
			tb.Logf("forwarder: write ended: %s", core.ErrorKind(err))
		}
		return false
	}
	return true
}

func (f *Forwarder) shutdown(tb testing.TB) {
	f.mu.Lock()
	f.closed = true
	open := f.open
	f.open = nil
	f.mu.Unlock()
	if err := f.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		tb.Errorf("forwarder: close the loopback listener: %s", core.ErrorKind(err))
	}
	for _, connection := range open {
		closeConnection(tb, connection)
	}
	f.Stop()
	f.done.Wait()
}
