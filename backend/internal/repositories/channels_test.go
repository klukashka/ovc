package repositories_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

const (
	seedChannelAccount = `INSERT INTO users (id, role, state, email, email_verified_at, password_hash, name, language, timezone, theme,
    consent_version, consent_at, unverified_since, created_at, updated_at)
VALUES ($1, $2, 'active', $3, $4, '$argon2id$v=19$m=19456,t=2,p=1$fixture', 'An account', 'en', 'Europe/Moscow', 'system',
    '2026-10', $4, $4, $4, $4)`
	seedChannelProfile = `INSERT INTO tutor_profiles (user_id, code, file_quota_bytes, recording_quota_bytes, created_at, updated_at)
VALUES ($1, 'ABCDEFGH', 1, 1, $2, $2)`
	seedChannelLink = `INSERT INTO relationships (id, tutor_id, student_id, state, display_name, colour, linked_at, created_at, updated_at)
VALUES ($1, $2, $3, 'active', 'A card', 'clay', $4, $4, $4)`
	seedChannelGroup  = `INSERT INTO groups (id, tutor_id, name, colour, created_at, updated_at) VALUES ($1, $2, 'A group', 'moss', $3, $3)`
	seedDirectChannel = `INSERT INTO channels (id, kind, tutor_id, relationship_id, last_seq, last_activity_at, discarded_at, erase_pending_since,
    created_at, updated_at)
VALUES ($1, 'direct', $2, $3, 1, $4, $5, $6, $4, $4)`
	seedGroupChannel = `INSERT INTO channels (id, kind, tutor_id, group_id, last_seq, last_activity_at, created_at, updated_at)
VALUES ($1, 'group', $2, $3, 1, $4, $4, $4)`
	seedChannelMember = `INSERT INTO channel_members (channel_id, user_id, suspended_since, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`
)

var mirrorAndPurgeQueries = []string{
	"AddDirectChannelMembers", "SuspendReclaimedMembers", "RestoreKeptMembers", "RemoveFormerGroupMembers", "AddGroupChannelMembers",
	"ListStudentDeletionTutors", "LockStudentDeletionGroups", "LockStudentDeletionChannels",
	"DeleteMembersOfChannels", "DeleteMembershipsOfUser", "LockChannelsOfMember", "DeleteTutorChannelMembersBatch",
	"ClaimUnbindableChatUploads", "MoveSenderReadMarker", "MoveReadMarker", "LockDigestMembers", "AdvanceDigestedMarker",
	"ListPeerReadSeqs",
	"GetPeerReadSeq", "GetDirectPeer", "ExportInvalidationAudience",
}

var namedQuery = regexp.MustCompile(`(?m)^-- name: (\w+) :\w+$`)

func queriesNaming(t *testing.T, table string) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../../queries/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no query files: %v", err)
	}
	naming := map[string]string{}
	for _, file := range files {
		text, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		bounds := namedQuery.FindAllStringSubmatchIndex(string(text), -1)
		for index, bound := range bounds {
			end := len(text)
			if index+1 < len(bounds) {
				end = bounds[index+1][0]
			}
			name, body := string(text[bound[2]:bound[3]]), string(text[bound[1]:end])
			if regexp.MustCompile(`\b` + table + `\b`).MatchString(body) {
				naming[name] = body
			}
		}
	}
	return naming
}

type predicateCase struct {
	name    string
	student domain.UserID
	channel domain.ChannelID
	link    domain.RelationshipID
	active  bool
}

func seedPredicateCases(t *testing.T, database *testdb.Database) (domain.UserID, domain.GroupID, []predicateCase) {
	t.Helper()
	ctx := context.Background()
	tutor, group := domain.UserID{1}, domain.GroupID{0x51}
	cases := []predicateCase{
		{name: "an active member", student: domain.UserID{2}, channel: domain.ChannelID{0x61}, link: domain.RelationshipID{0x41}, active: true},
		{name: "a suspended member", student: domain.UserID{3}, channel: domain.ChannelID{0x62}, link: domain.RelationshipID{0x42}},
		{name: "a discarded channel", student: domain.UserID{4}, channel: domain.ChannelID{0x63}, link: domain.RelationshipID{0x43}},
		{name: "a channel pending erasure", student: domain.UserID{5}, channel: domain.ChannelID{0x64}, link: domain.RelationshipID{0x44}},
	}
	exec := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	pool := database.Pool
	at := instant
	var none *time.Time
	shared := domain.ChannelID{0x65}
	_, err := pool.Exec(ctx, seedChannelAccount, tutor, "tutor", "tutor@example.org", at)
	exec(err)
	_, err = pool.Exec(ctx, seedChannelProfile, tutor, at)
	exec(err)
	_, err = pool.Exec(ctx, seedChannelGroup, group, tutor, at)
	exec(err)
	_, err = pool.Exec(ctx, seedGroupChannel, shared, tutor, group, at)
	exec(err)
	_, err = pool.Exec(ctx, seedChannelMember, shared, tutor, none, at)
	exec(err)
	for _, testCase := range cases {
		_, err = pool.Exec(ctx, seedChannelAccount, testCase.student, "student", testCase.student.String()+"@example.org", at)
		exec(err)
		_, err = pool.Exec(ctx, seedChannelLink, testCase.link, tutor, testCase.student, at)
		exec(err)
		discarded, erasing := none, none
		switch testCase.name {
		case "a discarded channel":
			discarded = &at
		case "a channel pending erasure":
			discarded, erasing = &at, &at
		}
		_, err = pool.Exec(ctx, seedDirectChannel, testCase.channel, tutor, testCase.link, at, discarded, erasing)
		exec(err)
		suspended := none
		if testCase.name == "a suspended member" {
			suspended = &at
		}
		_, err = pool.Exec(ctx, seedChannelMember, testCase.channel, tutor, none, at)
		exec(err)
		_, err = pool.Exec(ctx, seedChannelMember, testCase.channel, testCase.student, suspended, at)
		exec(err)
		groupSuspension := none
		if !testCase.active {
			groupSuspension = &at
		}
		_, err = pool.Exec(ctx, seedChannelMember, shared, testCase.student, groupSuspension, at)
		exec(err)
	}
	return tutor, group, cases
}

func TestEveryPathReadsMembersThroughTheActivePredicate(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	store := repositories.NewStore(database.Pool)
	tutor, group, cases := seedPredicateCases(t, database)
	repos := store.Repositories()
	ctx := context.Background()

	direct := queriesNaming(t, "channel_members")
	viewers := queriesNaming(t, "active_channel_members")
	links, linksErr := repos.Channels.OfRelationshipsFor(ctx, tutor, []domain.RelationshipID{cases[0].link, cases[1].link, cases[2].link, cases[3].link})
	labels, labelsErr := repos.Channels.MemberLabels(ctx, domain.ChannelID{0x65}, instant)
	members, membersErr := repos.Channels.ActiveMembers(ctx, []domain.ChannelID{cases[0].channel, cases[1].channel, cases[2].channel, cases[3].channel})
	tutorRows, tutorErr := repos.Channels.List(ctx, repositories.ChannelQuery{Member: tutor, Limit: 100})

	for name := range direct {
		if !slices.Contains(mirrorAndPurgeQueries, name) {
			t.Errorf("%s reads channel_members without the active-member predicate", name)
		}
	}
	for _, name := range mirrorAndPurgeQueries {
		if _, found := direct[name]; !found {
			t.Errorf("the mirror or purge query %s is listed but no longer names channel_members", name)
		}
	}
	if len(viewers) < 6 {
		t.Fatalf("only %d queries read the active-member predicate", len(viewers))
	}
	if linksErr != nil || labelsErr != nil || membersErr != nil || tutorErr != nil {
		t.Fatal(linksErr, labelsErr, membersErr, tutorErr)
	}
	for _, testCase := range cases {
		rows, err := repos.Channels.List(ctx, repositories.ChannelQuery{Member: testCase.student, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		unread, err := repos.Channels.CountUnread(ctx, testCase.student, 100)
		if err != nil {
			t.Fatal(err)
		}
		own, err := repos.Channels.OfRelationshipsFor(ctx, testCase.student, []domain.RelationshipID{testCase.link})
		if err != nil {
			t.Fatal(err)
		}
		inGroup, err := repos.Channels.OfGroupFor(ctx, testCase.student, group)
		if err != nil {
			t.Fatal(err)
		}
		_, tutorReaches := links[testCase.link]
		_, studentReaches := own[testCase.link]
		listed := slices.ContainsFunc(rows, func(row repositories.ChannelRow) bool { return row.ChannelID == testCase.channel })
		tutorLists := slices.ContainsFunc(tutorRows, func(row repositories.ChannelRow) bool { return row.ChannelID == testCase.channel })
		labelled := slices.ContainsFunc(labels, func(label repositories.MemberLabel) bool { return label.UserID == testCase.student })
		recipient := slices.ContainsFunc(members, func(member repositories.ActiveMember) bool { return member.UserID == testCase.student })
		channelLive := testCase.name != "a discarded channel" && testCase.name != "a channel pending erasure"
		studentCounted := map[bool]int64{true: 2, false: 0}[testCase.active]
		if listed != testCase.active || studentReaches != testCase.active || recipient != testCase.active || labelled != testCase.active ||
			(inGroup != nil) != testCase.active || unread != studentCounted {
			t.Errorf("%s: listed %t, reaches %t, recipient %t, labelled %t, in the group %t, unread %d",
				testCase.name, listed, studentReaches, recipient, labelled, inGroup != nil, unread)
		}
		if tutorLists != channelLive || tutorReaches != channelLive {
			t.Errorf("%s: the tutor lists it %t and reaches it %t", testCase.name, tutorLists, tutorReaches)
		}
	}
}

func TestDigestMemberWritesUseTheActivePredicate(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	store := repositories.NewStore(database.Pool)
	_, _, cases := seedPredicateCases(t, database)
	ctx := context.Background()
	reached := make(map[domain.UserID]bool)
	markers := make(map[domain.UserID]int64)

	for _, testCase := range cases {
		err := store.WithTx(ctx, func(repos repositories.Repositories) error {
			channels, err := repos.Digests.Channels(ctx, testCase.student)
			if err != nil {
				return err
			}
			reached[testCase.student] = slices.Contains(channels, testCase.channel)
			if err := repos.Digests.LockMembers(ctx, testCase.student, []domain.ChannelID{testCase.channel}); err != nil {
				return err
			}
			return repos.Digests.Advance(ctx, testCase.student, repositories.DigestBounds{ChannelID: testCase.channel, ThroughSeq: 1})
		})
		if err != nil {
			t.Fatal(err)
		}
		var marker int64
		if err := database.Pool.QueryRow(ctx, "SELECT digested_through_seq FROM channel_members WHERE user_id = $1 AND channel_id = $2", testCase.student, testCase.channel).Scan(&marker); err != nil {
			t.Fatal(err)
		}
		markers[testCase.student] = marker
	}

	for _, testCase := range cases {
		expected := int64(0)
		if testCase.active {
			expected = 1
		}
		if reached[testCase.student] != testCase.active || markers[testCase.student] != expected {
			t.Errorf("%s: reachable %t, marker %d", testCase.name, reached[testCase.student], markers[testCase.student])
		}
	}
}

func TestListPeerReadSeqsEqualsGetPeerReadSeqForEveryPredicateCase(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	tutor, _, cases := seedPredicateCases(t, database)
	ctx := context.Background()
	repos := repositories.NewStore(database.Pool).Repositories()
	purged := predicateCase{name: "a purged member", student: domain.UserID{6}, channel: domain.ChannelID{0x66}, link: domain.RelationshipID{0x46}}
	var none *time.Time
	if _, err := database.Pool.Exec(ctx, seedChannelAccount, purged.student, domain.RoleStudent, "purged@example.org", instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, seedChannelLink, purged.link, tutor, purged.student, instant); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, seedDirectChannel, purged.channel, tutor, purged.link, instant, none, none); err != nil {
		t.Fatal(err)
	}
	for _, member := range []domain.UserID{tutor, purged.student} {
		if _, err := database.Pool.Exec(ctx, seedChannelMember, purged.channel, member, none, instant); err != nil {
			t.Fatal(err)
		}
	}
	cases = append(cases, purged)
	channels := []domain.ChannelID{{0x65}}
	for _, testCase := range cases {
		channels = append(channels, testCase.channel)
	}
	if _, err := database.Pool.Exec(ctx, "UPDATE channels SET last_seq = 1000"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `WITH markers AS (
 SELECT channel_id, user_id, row_number() OVER (ORDER BY channel_id, user_id) AS position FROM channel_members
 ) UPDATE channel_members SET last_read_seq = markers.position * 3, shared_read_seq = markers.position * 3 - 1
 FROM markers WHERE channel_members.channel_id = markers.channel_id AND channel_members.user_id = markers.user_id`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, "DELETE FROM channel_members WHERE channel_id = $1 AND user_id = $2", purged.channel, purged.student); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range cases {
		for _, viewer := range []struct {
			name string
			id   domain.UserID
		}{{"tutor", tutor}, {"student", testCase.student}} {
			t.Run(testCase.name+"/"+viewer.name, func(t *testing.T) {
				single, singleErr := repos.Messages.PeerReadSeq(ctx, testCase.channel, viewer.id)
				groupMarker, groupErr := repos.Messages.PeerReadSeq(ctx, domain.ChannelID{0x65}, viewer.id)

				batch, batchErr := repos.Messages.PeerReadSeqs(ctx, channels, viewer.id)

				if singleErr != nil || batchErr != nil || groupErr != nil {
					t.Fatal(singleErr, batchErr, groupErr)
				}
				marker, exists := batch[testCase.channel]
				if (single != nil) != exists || (single != nil && marker != *single) {
					t.Fatalf("single %v, batch %v", single, batch)
				}
				if _, groupExists := batch[domain.ChannelID{0x65}]; groupExists || groupMarker != nil {
					t.Fatal("a group read marker was exposed")
				}
				if single != nil && (*single == 0 || *single%3 != 2) {
					t.Fatalf("shared marker was replaced by another marker: %d", *single)
				}
			})
		}
	}
}
