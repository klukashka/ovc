package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"ovc/backend/internal/core"
)

type SMTPConfig struct {
	Host      string
	Port      int
	TLS       core.SMTPTLSMode
	Username  core.Secret
	Password  core.Secret
	From      string
	PoolSize  int
	Timeout   time.Duration
	TLSConfig *tls.Config
	Logger    *slog.Logger
}

type SMTP struct {
	config SMTPConfig
	slots  chan struct{}
	idle   chan *pooled
}

type pooled struct {
	client *smtp.Client
	conn   *boundedConn
}

type boundedConn struct {
	net.Conn
	mutex sync.Mutex
	limit time.Time
}

func (c *boundedConn) bind(limit time.Time) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.limit = limit
	return c.Conn.SetDeadline(limit)
}

func (c *boundedConn) clamped(requested time.Time, set func(time.Time) error) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if requested.IsZero() || requested.After(c.limit) {
		requested = c.limit
	}
	return set(requested)
}

func (c *boundedConn) SetDeadline(requested time.Time) error {
	return c.clamped(requested, c.Conn.SetDeadline)
}

func (c *boundedConn) SetReadDeadline(requested time.Time) error {
	return c.clamped(requested, c.Conn.SetReadDeadline)
}

func (c *boundedConn) SetWriteDeadline(requested time.Time) error {
	return c.clamped(requested, c.Conn.SetWriteDeadline)
}

func NewSMTP(config SMTPConfig) *SMTP {
	if config.Logger == nil {
		config.Logger = slog.New(slog.DiscardHandler)
	}
	return &SMTP{config: config, slots: make(chan struct{}, config.PoolSize), idle: make(chan *pooled, config.PoolSize)}
}

func (s *SMTP) Send(ctx context.Context, message Message) error {
	payload, err := compose(s.config.From, message)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("wait for an SMTP connection: %w", ctx.Err())
	}
	defer func() { <-s.slots }()

	connection, err := s.connection(ctx)
	if err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { abort(connection.conn) })
	err = connection.client.SendMail(s.config.From, []string{message.To}, bytes.NewReader(payload))
	interrupted := !stop()
	if err != nil {
		return errors.Join(fmt.Errorf("send through SMTP: %w", errors.Join(err, ctx.Err())), closeClient(connection.client))
	}
	if interrupted {
		s.discard(ctx, connection)
		return nil
	}
	select {
	case s.idle <- connection:
		return nil
	default:
		return quitClient(connection.client)
	}
}

func (s *SMTP) Close() error {
	var problems []error
	for {
		select {
		case connection := <-s.idle:
			problems = append(problems, closeClient(connection.client))
		default:
			return errors.Join(problems...)
		}
	}
}

func (s *SMTP) connection(ctx context.Context) (*pooled, error) {
	limit, _ := ctx.Deadline()
	for {
		select {
		case connection := <-s.idle:
			if err := connection.conn.bind(limit); err == nil && connection.client.Reset() == nil {
				return connection, nil
			}
			s.discard(ctx, connection)
		default:
			return s.dial(ctx, limit)
		}
	}
}

func (s *SMTP) discard(ctx context.Context, connection *pooled) {
	if err := closeClient(connection.client); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, os.ErrDeadlineExceeded) {
		s.config.Logger.LogAttrs(ctx, slog.LevelWarn, "smtp.connection.discard_failed", slog.String("kind", core.ErrorKind(err)))
	}
}

func (s *SMTP) dial(ctx context.Context, limit time.Time) (*pooled, error) {
	dialer := net.Dialer{}
	address := net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port))
	raw, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect to SMTP: %w", err)
	}
	conn := &boundedConn{Conn: raw}
	if err := conn.bind(limit); err != nil {
		return nil, errors.Join(fmt.Errorf("bound the SMTP connection: %w", err), raw.Close())
	}
	stop := context.AfterFunc(ctx, func() { abort(conn) })
	defer stop()
	client, err := s.open(ctx, conn)
	if err != nil {
		return nil, errors.Join(err, closeRaw(raw))
	}
	client.CommandTimeout = s.config.Timeout
	client.SubmissionTimeout = s.config.Timeout
	if !s.config.Username.IsZero() {
		if err := client.Auth(sasl.NewPlainClient("", s.config.Username.Reveal(), s.config.Password.Reveal())); err != nil {
			return nil, errors.Join(fmt.Errorf("SMTP authentication: %w", err), closeClient(client))
		}
	}
	return &pooled{client: client, conn: conn}, nil
}

func (s *SMTP) open(ctx context.Context, conn *boundedConn) (*smtp.Client, error) {
	tlsConfig := s.tlsConfig()
	switch s.config.TLS {
	case core.SMTPImplicit:
		secured := tls.Client(conn, tlsConfig)
		if err := secured.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("SMTP TLS handshake: %w", err)
		}
		return smtp.NewClient(secured), nil
	case core.SMTPStartTLS:
		client, err := smtp.NewClientStartTLS(conn, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("SMTP STARTTLS: %w", err)
		}
		return client, nil
	}
	return nil, fmt.Errorf("unknown SMTP TLS mode %q", s.config.TLS)
}

func (s *SMTP) tlsConfig() *tls.Config {
	if s.config.TLSConfig != nil {
		return s.config.TLSConfig
	}
	return &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}
}

func closeRaw(raw net.Conn) error {
	if err := raw.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close the SMTP connection: %w", err)
	}
	return nil
}

func abort(conn *boundedConn) {
	if err := conn.bind(time.Unix(1, 0)); err != nil {
		return
	}
}

func closeClient(client *smtp.Client) error {
	if err := client.Close(); err != nil {
		return fmt.Errorf("close the SMTP connection: %w", err)
	}
	return nil
}

func quitClient(client *smtp.Client) error {
	if err := client.Quit(); err != nil {
		return errors.Join(fmt.Errorf("quit SMTP: %w", err), closeClient(client))
	}
	return nil
}
