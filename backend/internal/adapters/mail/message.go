package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

var ErrInvalidMessage = errors.New("the message cannot be sent")

type Message struct {
	To          string
	Subject     string
	Text        string
	HTML        string
	MessageID   string
	Date        time.Time
	Unsubscribe string
}

const (
	headerListUnsubscribe     = "List-Unsubscribe"
	headerListUnsubscribePost = "List-Unsubscribe-Post"
	oneClickUnsubscribe       = "List-Unsubscribe=One-Click"
)

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
	if malformedUnsubscribe(message.Unsubscribe) {
		return fmt.Errorf("%w: the unsubscribe address is malformed", ErrInvalidMessage)
	}
	return nil
}

func malformedUnsubscribe(address string) bool {
	if address == "" {
		return false
	}
	parsed, err := url.Parse(address)
	return err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || strings.ContainsAny(address, "\r\n<> ,")
}

const encodedWordBytes = 45

func encodedSubject(subject string) string {
	words := []string{}
	var chunk []byte
	for _, character := range subject {
		encoded := []byte(string(character))
		if len(chunk)+len(encoded) > encodedWordBytes {
			words = append(words, "=?utf-8?b?"+base64.StdEncoding.EncodeToString(chunk)+"?=")
			chunk = nil
		}
		chunk = append(chunk, encoded...)
	}
	words = append(words, "=?utf-8?b?"+base64.StdEncoding.EncodeToString(chunk)+"?=")
	return strings.Join(words, "\r\n ")
}

func messageHeaders(message Message) [][2]string {
	headers := [][2]string{
		{"Subject", encodedSubject(singleLine(message.Subject))},
		{"Date", message.Date.UTC().Format(time.RFC1123Z)},
		{"Message-ID", "<" + message.MessageID + ">"},
	}
	if message.Unsubscribe != "" {
		headers = append(headers, [2]string{headerListUnsubscribe, "<" + message.Unsubscribe + ">"},
			[2]string{headerListUnsubscribePost, oneClickUnsubscribe})
	}
	return headers
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
	headers := [][2]string{{"From", from}, {"To", message.To}}
	headers = append(headers, messageHeaders(message)...)
	headers = append(headers, [2]string{"MIME-Version", "1.0"}, [2]string{"Content-Type", "multipart/alternative; boundary=" + parts.Boundary()})
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
