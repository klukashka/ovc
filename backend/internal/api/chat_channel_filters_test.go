package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type channelFilterWorld struct {
	token    string
	direct   []string
	groups   []string
	unread   []string
	searched []string
}

func (h *authHarness) channelFilterWorld(t *testing.T) channelFilterWorld {
	t.Helper()
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	world := channelFilterWorld{token: token}
	for index := 0; index < 5; index++ {
		email := fmt.Sprintf("filter-student-%d@example.org", index)
		_, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, byte(index+2), email)
		card := h.linkedSignedIn(t, token, studentToken, fmt.Sprintf("Filter student %d", index), email)
		channel := h.directChannel(t, card.ID)
		h.setChannelActivity(t, channel.ID, authStart.Add(time.Duration(20-index)*time.Minute))
		world.direct = append(world.direct, channel.ID)
	}
	for index := 0; index < 4; index++ {
		name := fmt.Sprintf("Matching group %d", index)
		if index == 1 {
			name = "Other group"
		}
		group := h.newGroup(t, token, name, nil)
		channel := h.groupChannel(t, group.ID)
		h.setChannelActivity(t, channel.ID, authStart.Add(time.Duration(10-index)*time.Minute))
		world.groups = append(world.groups, channel.ID)
		if index != 1 {
			world.searched = append(world.searched, channel.ID)
		}
	}
	world.unread = []string{world.direct[1], world.groups[0], world.groups[3]}
	for _, id := range world.unread {
		h.setChannelSeq(t, id, 1)
	}
	return world
}

func (h *authHarness) walkChannelFilter(t *testing.T, token string, filter domain.ChannelFilter, search string) channelPage {
	t.Helper()
	query := url.Values{"filter": {string(filter)}, "limit": {"2"}, "q": {search}}
	combined := channelPage{}
	seen := map[string]bool{}
	for {
		result, page := h.channelList(t, token, "?"+query.Encode())
		h.mustAnswer(t, result, http.StatusOK)
		if len(page.Items) > 2 {
			t.Fatal("the page exceeds its limit")
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatalf("channel %s was returned twice", item.ID)
			}
			seen[item.ID] = true
			combined.Items = append(combined.Items, item)
		}
		combined.UnreadChannels = page.UnreadChannels
		if page.NextCursor == nil {
			return combined
		}
		if len(page.Items) == 0 {
			t.Fatal("an empty page has a cursor")
		}
		query.Set("cursor", *page.NextCursor)
	}
}

func TestChannelListFilterReturnsExactlyItsSetAcrossPages(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.channelFilterWorld(t)
	for _, testCase := range []struct {
		filter domain.ChannelFilter
		want   []string
	}{
		{domain.ChannelFilterAll, append(slices.Clone(world.direct), world.groups...)},
		{domain.ChannelFilterDirect, world.direct}, {domain.ChannelFilterGroup, world.groups}, {domain.ChannelFilterUnread, world.unread},
	} {
		t.Run(string(testCase.filter), func(t *testing.T) {

			page := h.walkChannelFilter(t, world.token, testCase.filter, "")

			if !slices.Equal(listedChannelIDs(page), testCase.want) || page.UnreadChannels != 3 {
				t.Fatalf("got %v and %d unread channels, want %v and 3", listedChannelIDs(page), page.UnreadChannels, testCase.want)
			}
		})
	}
}

func TestChannelListFilterComposesWithSearchAcrossPages(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.channelFilterWorld(t)

	page := h.walkChannelFilter(t, world.token, domain.ChannelFilterGroup, "MATCHING")

	if !slices.Equal(listedChannelIDs(page), world.searched) {
		t.Fatalf("got %v, want %v", listedChannelIDs(page), world.searched)
	}
}

func TestChannelListRejectsAnUnknownFilter(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)

	result, _ := h.channelList(t, token, "?filter=unknown")

	if result.status != http.StatusUnprocessableEntity || code(t, result) != "validation_failed" || !slices.Contains(fields(t, result), "query.filter:invalid") {
		t.Fatalf("unknown filter answered %d %s", result.status, result.body)
	}
}

func TestChannelListFiltersStayWithinTheCallersMembership(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	world := h.sharedChatWorld(t)
	order := []string{world.group.ID, world.direct.ID, world.squatChat.ID, world.secondChat.ID}
	for index, id := range order {
		h.setChannelActivity(t, id, authStart.Add(time.Duration(10-index)*time.Minute))
		h.setChannelSeq(t, id, 1)
	}
	for _, caller := range []struct {
		name, token         string
		direct, groups, all []string
	}{
		{"first tutor", world.parties.tutorToken, []string{world.direct.ID, world.squatChat.ID}, []string{world.group.ID}, order[:3]},
		{"second tutor", world.secondToken, []string{world.secondChat.ID}, []string{}, []string{world.secondChat.ID}},
		{"student", world.parties.studentToken, []string{world.direct.ID, world.secondChat.ID}, []string{world.group.ID}, []string{world.group.ID, world.direct.ID, world.secondChat.ID}},
	} {
		t.Run(caller.name, func(t *testing.T) {
			for _, filter := range []struct {
				value domain.ChannelFilter
				want  []string
			}{
				{domain.ChannelFilterAll, caller.all}, {domain.ChannelFilterDirect, caller.direct}, {domain.ChannelFilterGroup, caller.groups}, {domain.ChannelFilterUnread, caller.all},
			} {
				t.Run(string(filter.value), func(t *testing.T) {

					page := h.walkChannelFilter(t, caller.token, filter.value, "")

					if !slices.Equal(listedChannelIDs(page), filter.want) {
						t.Fatalf("got %v, want %v", listedChannelIDs(page), filter.want)
					}
				})
			}
		})
	}
}
