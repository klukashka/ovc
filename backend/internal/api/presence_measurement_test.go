//go:build presence_measurement

package api_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
)

type presenceLoadAccount struct {
	user    domain.UserID
	token   string
	channel string
	host    *liveHarness
	socket  *socketClient
}

func TestPresenceNotificationRateCheck(t *testing.T) {
	for _, instances := range []int{1, 2} {
		t.Run(fmt.Sprintf("%d-instances", instances), func(t *testing.T) {
			h := newLiveHarness(t, liveTuning{})
			hosts := []*liveHarness{h}
			if instances == 2 {
				hosts = append(hosts, h.anotherInstance(t))
			}
			accounts := make([]presenceLoadAccount, 0, 200)
			for tutorIndex := range 20 {
				id := byte(tutorIndex*10 + 1)
				tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, id, fmt.Sprintf("tutor-%d@example.org", tutorIndex))
				tutorAccount := presenceLoadAccount{user: tutor, token: tutorToken, host: hosts[tutorIndex%instances]}
				for studentIndex := range 9 {
					student, token := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, id+byte(studentIndex+1), fmt.Sprintf("student-%d-%d@example.org", tutorIndex, studentIndex))
					card := h.linkedSignedIn(t, tutorToken, token, "A learner", fmt.Sprintf("student-%d-%d@example.org", tutorIndex, studentIndex))
					channel := h.directChannel(t, card.ID).ID
					if tutorAccount.channel == "" {
						tutorAccount.channel = channel
					}
					accounts = append(accounts, presenceLoadAccount{user: student, token: token, channel: channel, host: hosts[(tutorIndex+studentIndex)%instances]})
				}
				accounts = append(accounts, tutorAccount)
			}
			for index := range accounts {
				accounts[index].socket = accounts[index].host.connect(t, accounts[index].token)
			}
			before := h.countLogs(`"msg":"realtime.delivery"`)
			started := (clock.System{}).Now()
			deadline := started.Add(time.Minute)
			var latencies []time.Duration
			cycles := 0
			nextSend := started

			for (clock.System{}).Now().Before(deadline) {
				for index := range accounts {
					item := &accounts[index]
					if err := item.socket.conn.CloseNow(); err != nil {
						t.Fatal(err)
					}
					item.host.awaitRegistry(t, item.user, 0)
					item.socket = item.host.connect(t, item.token)
					item.socket.typing(t, domain.FrameTypingStart, item.channel, "load")
					item.socket.pongAfter(t, "load-complete")
				}
				cycles++
				if (clock.System{}).Now().After(nextSend) {
					sendStarted := (clock.System{}).Now()
					h.sent(t, accounts[9].token, accounts[9].channel, "Load measurement")
					latencies = append(latencies, time.Since(sendStarted))
					nextSend = (clock.System{}).Now().Add(3 * time.Second)
				}
			}
			elapsed := time.Since(started)
			deliveries := h.countLogs(`"msg":"realtime.delivery"`) - before
			slices.Sort(latencies)
			pending := 0
			for _, item := range accounts {
				if item.host.gateway.Presence(item.user).Pending {
					pending++
				}
			}

			if cycles == 0 || len(latencies) == 0 || pending != 0 {
				t.Fatalf("cycles %d, sends %d, pending accounts %d", cycles, len(latencies), pending)
			}
			t.Logf("accounts=%d instances=%d elapsed_s=%.3f reconnects=%d notifications_per_s=%.3f send_http_p50_ms=%.3f send_http_p95_ms=%.3f pending_accounts=%d", len(accounts), instances, elapsed.Seconds(), cycles*len(accounts), float64(deliveries)/float64(instances)/elapsed.Seconds(), float64(latencies[len(latencies)/2])/float64(time.Millisecond), float64(latencies[(len(latencies)-1)*95/100])/float64(time.Millisecond), pending)
		})
	}
}
