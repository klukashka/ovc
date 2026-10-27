package mail

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

const (
	statementInsertSink = "INSERT INTO mail_sink (id, recipient, subject, text_body, html_body, links, message_id, sent_at, headers) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)"
	statementReadSink = "SELECT subject, text_body, html_body, links, sent_at, message_id, headers FROM mail_sink WHERE recipient = $1 ORDER BY sent_at, id"
)

var linkPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

type SinkMessage struct {
	Subject   string
	Text      string
	HTML      string
	Links     []string
	SentAt    time.Time
	MessageID string
	Headers   map[string]string
}

type Sink struct {
	pool   *pgxpool.Pool
	random core.RandomSource
}

func NewSink(pool *pgxpool.Pool, random core.RandomSource) *Sink {
	return &Sink{pool: pool, random: random}
}

func (s *Sink) Send(ctx context.Context, message Message) error {
	if err := validate(message); err != nil {
		return err
	}
	id, err := core.NewID[domain.EntityID](s.random)
	if err != nil {
		return err
	}
	headers := map[string]string{}
	for _, header := range messageHeaders(message) {
		headers[header[0]] = header[1]
	}
	_, err = s.pool.Exec(ctx, statementInsertSink, id, message.To, singleLine(message.Subject), message.Text, message.HTML,
		links(message), message.MessageID, message.Date.UTC(), headers)
	if err != nil {
		return fmt.Errorf("write the mail sink: %w", err)
	}
	return nil
}

func (s *Sink) Messages(ctx context.Context, recipient string) ([]SinkMessage, error) {
	rows, err := s.pool.Query(ctx, statementReadSink, recipient)
	if err != nil {
		return nil, fmt.Errorf("read the mail sink: %w", err)
	}
	defer rows.Close()
	var messages []SinkMessage
	for rows.Next() {
		var message SinkMessage
		if err := rows.Scan(&message.Subject, &message.Text, &message.HTML, &message.Links, &message.SentAt, &message.MessageID, &message.Headers); err != nil {
			return nil, fmt.Errorf("read a mail sink row: %w", err)
		}
		message.SentAt = message.SentAt.UTC()
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the mail sink: %w", err)
	}
	return messages, nil
}

func links(message Message) []string {
	found := []string{}
	candidates := append(linkPattern.FindAllString(message.Text, -1), linkPattern.FindAllString(html.UnescapeString(message.HTML), -1)...)
	for _, candidate := range candidates {
		link := strings.TrimRight(candidate, ".,;:!?)")
		if !slices.Contains(found, link) {
			found = append(found, link)
		}
	}
	return found
}
