package mail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

var ErrInvalidMessage = errors.New("the message cannot be sent")

type Message struct {
	To        string
	Subject   string
	Text      string
	HTML      string
	MessageID string
	Date      time.Time
}

type Mailer interface {
	Send(ctx context.Context, message Message) error
}

func singleLine(text string) string {
	return strings.Join(strings.FieldsFunc(text, func(r rune) bool { return r == '\r' || r == '\n' }), " ")
}

func validate(message Message) error {
	address, err := mail.ParseAddress(message.To)
	if err != nil || address.Address != message.To || strings.ContainsAny(message.To, "\r\n") {
		return fmt.Errorf("%w: the recipient is not one plain address", ErrInvalidMessage)
	}
	if message.MessageID == "" || strings.ContainsAny(message.MessageID, "\r\n<> ") || !strings.Contains(message.MessageID, "@") {
		return fmt.Errorf("%w: the message identifier is malformed", ErrInvalidMessage)
	}
	if message.Date.IsZero() {
		return fmt.Errorf("%w: the message has no date", ErrInvalidMessage)
	}
	return nil
}

func compose(from string, message Message) ([]byte, error) {
	if err := validate(message); err != nil {
		return nil, err
	}
	var body bytes.Buffer
	parts := multipart.NewWriter(&body)
	if err := writePart(parts, "text/plain; charset=utf-8", message.Text); err != nil {
		return nil, err
	}
	if err := writePart(parts, "text/html; charset=utf-8", message.HTML); err != nil {
		return nil, err
	}
	if err := parts.Close(); err != nil {
		return nil, fmt.Errorf("close the message body: %w", err)
	}

	var composed bytes.Buffer
	headers := [][2]string{
		{"From", from},
		{"To", message.To},
		{"Subject", mime.BEncoding.Encode("utf-8", singleLine(message.Subject))},
		{"Date", message.Date.UTC().Format(time.RFC1123Z)},
		{"Message-ID", "<" + message.MessageID + ">"},
		{"MIME-Version", "1.0"},
		{"Content-Type", "multipart/alternative; boundary=" + parts.Boundary()},
	}
	for _, header := range headers {
		composed.WriteString(header[0] + ": " + header[1] + "\r\n")
	}
	composed.WriteString("\r\n")
	composed.Write(body.Bytes())
	return composed.Bytes(), nil
}

func writePart(parts *multipart.Writer, contentType string, content string) error {
	part, err := parts.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {contentType},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return fmt.Errorf("create a message part: %w", err)
	}
	encoder := quotedprintable.NewWriter(part)
	if _, err := encoder.Write([]byte(content)); err != nil {
		return fmt.Errorf("encode a message part: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finish a message part: %w", err)
	}
	return nil
}
