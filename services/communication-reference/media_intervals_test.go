package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReconnectCountsOnlyVerifiedTwoWayMediaAfterRestart(t *testing.T) {
	service, started := referenceFixture(t)
	booking := "88888888-8888-4888-8888-888888888888"
	client := "99999999-9999-4999-8999-999999999999"
	specialist := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	clientToken, _ := issueFixture(t, service, booking, client, "CLIENT", "recovery-client-key-001", started.Add(5*time.Minute))
	specialistToken, _ := issueFixture(t, service, booking, specialist, "SPECIALIST", "recovery-specialist-key-001", started.Add(5*time.Minute))
	clientID := identityFor("CLIENT", client)
	specialistID := identityFor("SPECIALIST", specialist)
	roomName := roomName(booking)
	apply := func(id, kind, identity string, seconds int, audio bool) {
		t.Helper()
		event := providerEvent(id, kind, identity, roomName, started.Add(time.Duration(seconds)*time.Second))
		if audio {
			event.Track.Type = "AUDIO"
		}
		service.mu.Lock()
		room, err := service.readRoom(booking)
		if err == nil {
			err = service.applyLiveKitEvent(room, event)
		}
		if err == nil {
			err = service.writeRoom(room)
		}
		service.mu.Unlock()
		if err != nil {
			t.Fatalf("provider evidence %s: %v", id, err)
		}
	}
	apply("joined-client", "participant_joined", clientID, 0, false)
	apply("joined-specialist", "participant_joined", specialistID, 1, false)
	apply("audio-client", "track_published", clientID, 2, true)
	apply("audio-specialist", "track_published", specialistID, 3, true)
	apply("client-offline", "participant_left", clientID, 20, false)
	room, err := service.readRoom(booking)
	if err != nil {
		t.Fatal(err)
	}
	if !room.Recovering || room.MediaAccumulatedNS != int64(17*time.Second) {
		t.Fatalf("offline interval counted as media: %+v", room)
	}
	// A separately opened adapter process must preserve the amount of
	// authenticated two-way media already completed before disconnection.
	restarted := newReferenceService(service.config)
	restarted.clock = service.clock
	service = restarted
	apply("client-rejoined", "participant_joined", clientID, 30, false)
	apply("client-audio-republished", "track_published", clientID, 31, true)
	for _, token := range []string{clientToken, specialistToken} {
		req := httptest.NewRequest(http.MethodPost, "/v1/rooms/finish", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		service.handleFinish(response, req)
		if response.Code != http.StatusNoContent {
			t.Fatalf("mutual completion intent not accepted: %d %s", response.Code, response.Body.String())
		}
	}
	apply("final-client-left", "participant_left", clientID, 100, false)
	apply("final-specialist-left", "participant_left", specialistID, 101, false)
	apply("room-closed", "room_finished", "", 104, false)
	room, err = service.readRoom(booking)
	if err != nil {
		t.Fatal(err)
	}
	if !room.Terminal || room.MediaAccumulatedNS != int64(86*time.Second) {
		t.Fatalf("recovery destroyed verified conversation time: %+v", room)
	}
	counts := make(map[string]int)
	for _, record := range room.Outbox {
		var fact struct {
			FactType string `json:"fact_type"`
		}
		if err := json.Unmarshal(record.Payload, &fact); err != nil {
			t.Fatal(err)
		}
		counts[fact.FactType]++
	}
	if counts["STARTED"] != 1 || counts["RECOVERY_STARTED"] != 2 ||
		counts["RECOVERY_SUCCEEDED"] != 1 || counts["ENDED"] != 1 ||
		counts["TECHNICAL_FAILURE"] != 0 {
		t.Fatalf("incorrect media recovery lifecycle facts: %+v", counts)
	}
}

func TestAudioUnpublishPausesBusinessMediaTimeAndStopsFalseCompletion(t *testing.T) {
	service, start := referenceFixture(t)
	booking := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	client := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	specialist := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	clientToken, _ := issueFixture(t, service, booking, client, "CLIENT", "client-unpublish-key-01", start.Add(5*time.Minute))
	specialistToken, _ := issueFixture(t, service, booking, specialist, "SPECIALIST", "specialist-unpublish-key-01", start.Add(5*time.Minute))
	clientID := identityFor("CLIENT", client)
	specialistID := identityFor("SPECIALIST", specialist)
	apply := func(id, kind, identity string, seconds int, audio bool) error {
		t.Helper()
		event := providerEvent(id, kind, identity, roomName(booking), start.Add(time.Duration(seconds)*time.Second))
		if audio {
			event.Track.Type = "AUDIO"
		}
		service.mu.Lock()
		defer service.mu.Unlock()
		room, err := service.readRoom(booking)
		if err == nil {
			err = service.applyLiveKitEvent(room, event)
		}
		if err == nil {
			err = service.writeRoom(room)
		}
		return err
	}
	for _, event := range []struct {
		id, kind, participant string
		sec                   int
		audio                 bool
	}{
		{"join-c", "participant_joined", clientID, 0, false},
		{"join-s", "participant_joined", specialistID, 1, false},
		{"publish-c", "track_published", clientID, 2, true},
		{"publish-s", "track_published", specialistID, 3, true},
		{"unpublish-c", "track_unpublished", clientID, 15, true},
	} {
		if err := apply(event.id, event.kind, event.participant, event.sec, event.audio); err != nil {
			t.Fatal(err)
		}
	}
	if err := apply("unauthorized-audio-removal", "track_unpublished", "FOREIGN:untrusted", 19, true); err == nil {
		t.Fatal("foreign participant modified audio evidence")
	}
	if err := apply("republish-c", "track_published", clientID, 35, true); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{clientToken, specialistToken} {
		req := httptest.NewRequest(http.MethodPost, "/v1/rooms/finish", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		service.handleFinish(response, req)
		if response.Code != http.StatusNoContent {
			t.Fatalf("participant finish intent denied: %d", response.Code)
		}
	}
	for _, event := range []struct {
		id, kind, participant string
		sec                   int
	}{
		{"exit-c", "participant_left", clientID, 64},
		{"exit-s", "participant_left", specialistID, 65},
		{"closed", "room_finished", "", 70},
	} {
		if err := apply(event.id, event.kind, event.participant, event.sec, false); err != nil {
			t.Fatal(err)
		}
	}
	room, err := service.readRoom(booking)
	if err != nil {
		t.Fatal(err)
	}
	// 12 seconds before audio unpublish, then 29 seconds after republish:
	// never count 20 seconds offline/without an active audio track.
	if room.MediaAccumulatedNS != int64(41*time.Second) {
		t.Fatalf("muted/rejected media was counted: %s", time.Duration(room.MediaAccumulatedNS))
	}
	var final struct {
		FactType string `json:"fact_type"`
	}
	if err := json.Unmarshal(room.Outbox[len(room.Outbox)-1].Payload, &final); err != nil {
		t.Fatal(err)
	}
	if final.FactType != "TECHNICAL_FAILURE" {
		t.Fatalf("insufficient real media was incorrectly certified completed: %s", final.FactType)
	}
}
