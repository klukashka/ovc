package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"
)

const channelsRoute = "/api/v1/channels"

type channelTutor struct {
	Name   string  `json:"name"`
	Colour *string `json:"colour"`
}

type channelMemberLabel struct {
	UserID string `json:"user_id"`
	Label  string `json:"label"`
	Online *bool  `json:"online"`
}

type channelItem struct {
	ID             string               `json:"id"`
	Kind           string               `json:"kind"`
	Label          *string              `json:"label"`
	LabelKind      *string              `json:"label_kind"`
	Colour         *string              `json:"colour"`
	Tutor          *channelTutor        `json:"tutor"`
	UserID         *string              `json:"user_id"`
	LastMessage    json.RawMessage      `json:"last_message"`
	UnreadCount    int64                `json:"unread_count"`
	Writable       bool                 `json:"writable"`
	Online         *bool                `json:"online"`
	Unconfirmed    bool                 `json:"unconfirmed"`
	RelationshipID *string              `json:"relationship_id"`
	GroupID        *string              `json:"group_id"`
	Members        []channelMemberLabel `json:"members"`
}

type channelPage struct {
	Items          []channelItem `json:"items"`
	NextCursor     *string       `json:"next_cursor"`
	UnreadChannels int64         `json:"unread_channels"`
}

type storedChannel struct {
	ID                    string
	ReadOnly              bool
	Discarded             bool
	ErasePending          bool
	LastSeq               int64
	ProvisionalThroughSeq *int64
}

type storedMember struct {
	User       string
	Suspended  bool
	LastRead   int64
	Digested   int64
	ChannelSeq int64
}

func (h *authHarness) channelOf(t *testing.T, column string, audience string) (storedChannel, bool) {
	t.Helper()
	if h.countRows(t, "SELECT count(id) FROM channels WHERE "+column+" = $1::uuid", audience) == 0 {
		return storedChannel{}, false
	}
	var channel storedChannel
	err := h.database.Pool.QueryRow(context.Background(),
		"SELECT id::text, read_only_since IS NOT NULL, discarded_at IS NOT NULL, erase_pending_since IS NOT NULL, last_seq, provisional_through_seq "+
			"FROM channels WHERE "+column+" = $1::uuid", audience,
	).Scan(&channel.ID, &channel.ReadOnly, &channel.Discarded, &channel.ErasePending, &channel.LastSeq, &channel.ProvisionalThroughSeq)
	if err != nil {
		t.Fatal(err)
	}
	return channel, true
}

func (h *authHarness) directChannel(t *testing.T, relationship string) storedChannel {
	t.Helper()
	channel, found := h.channelOf(t, "relationship_id", relationship)
	if !found {
		t.Fatalf("the relationship %s has no channel", relationship)
	}
	return channel
}

func (h *authHarness) groupChannel(t *testing.T, group string) storedChannel {
	t.Helper()
	channel, found := h.channelOf(t, "group_id", group)
	if !found {
		t.Fatalf("the group %s has no channel", group)
	}
	return channel
}

func (h *authHarness) channelMembers(t *testing.T, channel string) map[string]storedMember {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(),
		"SELECT channel_members.user_id::text, channel_members.suspended_since IS NOT NULL, channel_members.last_read_seq, "+
			"channel_members.digested_through_seq, channels.last_seq FROM channel_members "+
			"JOIN channels ON channels.id = channel_members.channel_id WHERE channel_members.channel_id = $1::uuid", channel)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	members := map[string]storedMember{}
	for rows.Next() {
		var member storedMember
		if err := rows.Scan(&member.User, &member.Suspended, &member.LastRead, &member.Digested, &member.ChannelSeq); err != nil {
			t.Fatal(err)
		}
		members[member.User] = member
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return members
}

func activeMemberSet(members map[string]storedMember) []string {
	var active []string
	for user, member := range members {
		if !member.Suspended {
			active = append(active, user)
		}
	}
	slices.Sort(active)
	return active
}

func (h *authHarness) channelList(t *testing.T, token string, query string) (response, channelPage) {
	t.Helper()
	result := h.send(t, http.MethodGet, channelsRoute+query, "", jsonCall(token))
	var page channelPage
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &page); err != nil {
			t.Fatal(err)
		}
	}
	return result, page
}

func (h *authHarness) channelCall(t *testing.T, token string, id string) (response, channelItem) {
	t.Helper()
	result := h.send(t, http.MethodGet, channelsRoute+"/"+id, "", jsonCall(token))
	var item channelItem
	if result.status == http.StatusOK {
		if err := json.Unmarshal([]byte(result.body), &item); err != nil {
			t.Fatal(err)
		}
	}
	return result, item
}

func listedChannelIDs(page channelPage) []string {
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

func (h *authHarness) setChannelSeq(t *testing.T, channel string, seq int64) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE channels SET last_seq = $2 WHERE id = $1::uuid", channel, seq); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) setChannelActivity(t *testing.T, channel string, at time.Time) {
	t.Helper()
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE channels SET last_activity_at = $2 WHERE id = $1::uuid", channel, at); err != nil {
		t.Fatal(err)
	}
}

func unreadCount(t *testing.T, result response) int64 {
	t.Helper()
	var decoded struct {
		UnreadChannels *int64 `json:"unread_channels"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || decoded.UnreadChannels == nil {
		t.Fatalf("not an unread count: %d %s", result.status, result.body)
	}
	return *decoded.UnreadChannels
}
