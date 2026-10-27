package mail_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	netmail "net/mail"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/core"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/testmail"
)

func fixtureLogin() string {
	return "fixture-smtp-login"
}

var sentAt = time.Date(2026, time.October, 21, 9, 30, 0, 0, time.UTC)

type received struct {
	To          string
	Subject     string
	Text        string
	HTML        string
	Unsubscribe string
	OneClick    string
}

type backend struct {
	mailer  mail.Mailer
	inbox   func(t *testing.T, to string) []received
	cleanup func()
}

func message(to string, subject string) mail.Message {
	return mail.Message{
		To:        to,
		Subject:   subject,
		Text:      "Open https://ovc.example/verify-email?token=abc to confirm.",
		HTML:      `<p><a href="https://ovc.example/verify-email?token=abc">Confirm</a></p>`,
		MessageID: "0123abcd@ovc.example",
		Date:      sentAt,
	}
}

func TestMailContract(t *testing.T) {
	for name, start := range map[string]func(t *testing.T) backend{"smtp": startSMTP, "smtp-implicit": startImplicitSMTP, "sink": startSink, "testmail": startTestMail} {
		t.Run(name, func(t *testing.T) {
			t.Run("delivers the recipient, subject and both bodies", func(t *testing.T) {
				subject := backendFor(t, start)
				sent := message("tutor@example.org", "Подтвердите email")

				err := subject.mailer.Send(context.Background(), sent)

				inbox := subject.inbox(t, "tutor@example.org")
				if err != nil || len(inbox) != 1 {
					t.Fatalf("send %v, inbox %+v", err, inbox)
				}
				got := inbox[0]
				if got.Subject != sent.Subject || strings.TrimSpace(got.Text) != sent.Text || strings.TrimSpace(got.HTML) != sent.HTML {
					t.Fatalf("received %+v", got)
				}
			})

			t.Run("keeps a subject on one line", func(t *testing.T) {
				subject := backendFor(t, start)

				err := subject.mailer.Send(context.Background(), message("tutor@example.org", "Hello\r\nBcc: victim@example.org"))

				inbox := subject.inbox(t, "tutor@example.org")
				if err != nil || len(inbox) != 1 || strings.ContainsAny(inbox[0].Subject, "\r\n") || inbox[0].Subject != "Hello Bcc: victim@example.org" {
					t.Fatalf("send %v, inbox %+v", err, inbox)
				}
			})

			t.Run("carries the one-click unsubscribe headers of an optional email", func(t *testing.T) {
				subject := backendFor(t, start)
				optional := message("tutor@example.org", "Lesson in 1 hour")
				optional.Unsubscribe = "https://ovc.example/api/v1/unsubscribe?token=v1.abc.def"

				errs := []error{
					subject.mailer.Send(context.Background(), optional),
					subject.mailer.Send(context.Background(), message("student@example.org", "Confirm your email")),
				}

				inbox, required := subject.inbox(t, "tutor@example.org"), subject.inbox(t, "student@example.org")
				if errors.Join(errs...) != nil || len(inbox) != 1 || len(required) != 1 {
					t.Fatalf("send %v, inboxes %+v and %+v", errs, inbox, required)
				}
				if inbox[0].Unsubscribe != "<"+optional.Unsubscribe+">" || inbox[0].OneClick != "List-Unsubscribe=One-Click" {
					t.Fatalf("the optional email carries %q and %q", inbox[0].Unsubscribe, inbox[0].OneClick)
				}
				if required[0].Unsubscribe != "" || required[0].OneClick != "" {
					t.Fatalf("a required email carries %q and %q", required[0].Unsubscribe, required[0].OneClick)
				}
			})

			t.Run("refuses a malformed unsubscribe address", func(t *testing.T) {
				subject := backendFor(t, start)
				var errs []error
				for _, address := range []string{"https://ovc.example/u?token=a\r\nBcc: victim@example.org", "https://ovc.example/u?token=a>", "mailto:a@example.org"} {
					malformed := message("tutor@example.org", "Hello")
					malformed.Unsubscribe = address
					errs = append(errs, subject.mailer.Send(context.Background(), malformed))
				}

				for _, err := range errs {
					if !errors.Is(err, mail.ErrInvalidMessage) {
						t.Fatalf("expected a refusal, got %v", err)
					}
				}
				if inbox := subject.inbox(t, "tutor@example.org"); len(inbox) != 0 {
					t.Fatalf("a refused message was delivered: %+v", inbox)
				}
			})

			t.Run("refuses a recipient that is not one plain address", func(t *testing.T) {
				subject := backendFor(t, start)

				errs := []error{
					subject.mailer.Send(context.Background(), message("tutor@example.org\r\nBcc: victim@example.org", "Hello")),
					subject.mailer.Send(context.Background(), message("Tutor <tutor@example.org>", "Hello")),
				}

				for _, err := range errs {
					if !errors.Is(err, mail.ErrInvalidMessage) {
						t.Fatalf("expected a refusal, got %v", err)
					}
				}
				if inbox := subject.inbox(t, "tutor@example.org"); len(inbox) != 0 {
					t.Fatalf("a refused message was delivered: %+v", inbox)
				}
			})
		})
	}
}

func backendFor(t *testing.T, start func(t *testing.T) backend) backend {
	t.Helper()
	started := start(t)
	t.Cleanup(started.cleanup)
	return started
}

func startSink(t *testing.T) backend {
	t.Helper()
	sink := mail.NewSink(testdb.New(t).Pool, &countingRandom{})
	inbox := func(t *testing.T, to string) []received {
		t.Helper()
		messages, err := sink.Messages(context.Background(), to)
		if err != nil {
			t.Fatal(err)
		}
		var result []received
		for _, stored := range messages {
			if !stored.SentAt.Equal(sentAt) || len(stored.Links) != 1 || stored.Links[0] != "https://ovc.example/verify-email?token=abc" {
				t.Fatalf("stored %+v", stored)
			}
			result = append(result, received{
				To: to, Subject: stored.Subject, Text: stored.Text, HTML: stored.HTML,
				Unsubscribe: stored.Headers["List-Unsubscribe"], OneClick: stored.Headers["List-Unsubscribe-Post"],
			})
		}
		return result
	}
	return backend{mailer: sink, inbox: inbox, cleanup: func() {}}
}

func startTestMail(t *testing.T) backend {
	t.Helper()
	sink := startSink(t)
	return backend{mailer: testmail.New(sink.mailer), inbox: sink.inbox, cleanup: sink.cleanup}
}

type capture struct {
	mutex    sync.Mutex
	messages []received
	auths    int
}

func (c *capture) authenticated() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.auths++
}

func (c *capture) authentications() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.auths
}

func (c *capture) add(message received) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.messages = append(c.messages, message)
}

func (c *capture) to(recipient string) []received {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	var result []received
	for _, message := range c.messages {
		if message.To == recipient {
			result = append(result, message)
		}
	}
	return result
}

type session struct {
	capture *capture
	to      string
}

func (s *session) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

func (s *session) Auth(string) (sasl.Server, error) {
	s.capture.authenticated()
	return sasl.NewPlainServer(func(_, username, password string) error {
		if username != "ovc" || password != fixtureLogin() {
			return errors.New("invalid credentials")
		}
		return nil
	}), nil
}

func (s *session) Mail(string, *smtp.MailOptions) error { return nil }

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.to = to
	return nil
}

func (s *session) Data(reader io.Reader) error {
	parsed, err := parse(reader)
	if err != nil {
		return err
	}
	parsed.To = s.to
	s.capture.add(parsed)
	return nil
}

func (s *session) Reset() {}

func (s *session) Logout() error { return nil }

func parse(reader io.Reader) (received, error) {
	parsed, err := netmail.ReadMessage(reader)
	if err != nil {
		return received{}, err
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil {
		return received{}, err
	}
	_, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil {
		return received{}, err
	}
	result := received{Subject: subject, Unsubscribe: parsed.Header.Get("List-Unsubscribe"), OneClick: parsed.Header.Get("List-Unsubscribe-Post")}
	parts := multipart.NewReader(parsed.Body, params["boundary"])
	for {
		part, err := parts.NextPart()
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			return received{}, err
		}
		content, err := io.ReadAll(part)
		if err != nil {
			return received{}, err
		}
		if strings.HasPrefix(part.Header.Get("Content-Type"), "text/html") {
			result.HTML = string(content)
		} else {
			result.Text = string(content)
		}
	}
}

type smtpServer struct {
	port     int
	roots    *x509.CertPool
	captured *capture
}

func serveSMTP(t *testing.T, offerTLS bool, implicit bool) smtpServer {
	t.Helper()
	certificate, roots := selfSignedCertificate(t)
	captured := &capture{}
	server := smtp.NewServer(smtp.BackendFunc(func(*smtp.Conn) (smtp.Session, error) {
		return &session{capture: captured}, nil
	}))
	server.Domain = "127.0.0.1"
	server.ReadTimeout = 5 * time.Second
	server.WriteTimeout = 5 * time.Second
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	if offerTLS && !implicit {
		server.TLSConfig = tlsConfig
	}
	server.AllowInsecureAuth = !offerTLS
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if implicit {
		listener = tls.NewListener(listener, tlsConfig)
	}
	served := core.Run(context.Background(), slog.New(slog.DiscardHandler), "smtp-test-server", func() error { return server.Serve(listener) })
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Error(err)
		}
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
		if err := <-served; err != nil && !errors.Is(err, smtp.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
	})
	return smtpServer{port: portOf(t, listener.Addr()), roots: roots, captured: captured}
}

func portOf(t *testing.T, address net.Addr) int {
	t.Helper()
	port, err := strconv.Atoi(strings.TrimPrefix(address.String(), "127.0.0.1:"))
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func smtpMailer(port int, mode core.SMTPTLSMode, roots *x509.CertPool, timeout time.Duration) *mail.SMTP {
	return mail.NewSMTP(mail.SMTPConfig{
		Host: "127.0.0.1", Port: port, TLS: mode, Username: core.NewSecret("ovc"), Password: core.NewSecret(fixtureLogin()),
		From: "noreply@ovc.example", PoolSize: 2, Timeout: timeout,
		TLSConfig: &tls.Config{RootCAs: roots, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12},
		Logger:    slog.New(slog.DiscardHandler),
	})
}

func smtpBackend(t *testing.T, mode core.SMTPTLSMode) backend {
	t.Helper()
	server := serveSMTP(t, true, mode == core.SMTPImplicit)
	mailer := smtpMailer(server.port, mode, server.roots, 5*time.Second)
	inbox := func(t *testing.T, to string) []received {
		t.Helper()
		return server.captured.to(to)
	}
	cleanup := func() {
		if err := mailer.Close(); err != nil {
			t.Error(err)
		}
	}
	return backend{mailer: mailer, inbox: inbox, cleanup: cleanup}
}

func startSMTP(t *testing.T) backend {
	t.Helper()
	return smtpBackend(t, core.SMTPStartTLS)
}

func startImplicitSMTP(t *testing.T) backend {
	t.Helper()
	return smtpBackend(t, core.SMTPImplicit)
}

func TestSMTPRefusesAServerWithoutSTARTTLS(t *testing.T) {
	server := serveSMTP(t, false, false)
	mailer := smtpMailer(server.port, core.SMTPStartTLS, server.roots, 5*time.Second)

	err := mailer.Send(context.Background(), message("tutor@example.org", "Hello"))

	if err == nil || server.captured.authentications() != 0 || len(server.captured.to("tutor@example.org")) != 0 {
		t.Fatalf("send %v, authentications %d", err, server.captured.authentications())
	}
}

func TestSMTPRefusesAnUntrustedCertificate(t *testing.T) {
	server := serveSMTP(t, true, false)
	mailer := smtpMailer(server.port, core.SMTPStartTLS, x509.NewCertPool(), 5*time.Second)

	err := mailer.Send(context.Background(), message("tutor@example.org", "Hello"))

	if err == nil || server.captured.authentications() != 0 || len(server.captured.to("tutor@example.org")) != 0 {
		t.Fatalf("send %v, authentications %d", err, server.captured.authentications())
	}
}

func TestSMTPSendIsBoundedByItsTimeout(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan net.Conn, 1)
	core.Go(context.Background(), slog.New(slog.DiscardHandler), "silent-smtp-peer", func(context.Context) {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	})
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
		select {
		case connection := <-accepted:
			if err := connection.Close(); err != nil {
				t.Error(err)
			}
		default:
		}
	})
	mailer := smtpMailer(portOf(t, listener.Addr()), core.SMTPStartTLS, x509.NewCertPool(), 200*time.Millisecond)

	result := core.Run(context.Background(), slog.New(slog.DiscardHandler), "send", func() error {
		return mailer.Send(context.Background(), message("tutor@example.org", "Hello"))
	})

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("a send to a silent server succeeded")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a send to a silent server was not bounded by its timeout")
	}
}

func selfSignedCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    clock.System{}.Now().Add(-time.Hour),
		NotAfter:     clock.System{}.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots
}

func TestSinkExtractsUnescapedLinksWithoutTrailingPunctuation(t *testing.T) {
	sink := mail.NewSink(testdb.New(t).Pool, &countingRandom{})
	sent := message("tutor@example.org", "Hello")
	sent.Text = "Open https://ovc.example/verify-email?token=abc&lang=en."
	sent.HTML = `<a href="https://ovc.example/verify-email?token=abc&amp;lang=en">Confirm</a>`

	err := sink.Send(context.Background(), sent)

	stored, readErr := sink.Messages(context.Background(), "tutor@example.org")
	if err != nil || readErr != nil || len(stored) != 1 {
		t.Fatalf("send %v, read %v, %d messages", err, readErr, len(stored))
	}
	if links := stored[0].Links; len(links) != 1 || links[0] != "https://ovc.example/verify-email?token=abc&lang=en" {
		t.Fatalf("links %q", links)
	}
}

type countingRandom struct {
	mutex sync.Mutex
	next  byte
}

func (r *countingRandom) Read(buffer []byte) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for index := range buffer {
		r.next++
		buffer[index] = r.next
	}
	return len(buffer), nil
}
