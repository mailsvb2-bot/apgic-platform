package ucr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

func mockEvent(now time.Time) liveevents.Event {
	return liveevents.Event{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", OrganizationID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		Title: "Занятие в группе", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour),
		Capacity: 12, Format: liveevents.Group, State: liveevents.Draft}
}
func TestUCRRealCapabilityGateAndNoFakeMediaGrant(t *testing.T) {
	now := time.Date(2026, 10, 12, 11, 0, 0, 0, time.UTC)
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/capabilities" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"capabilities":{"max_participants":300,"browser_realtime_gateway":true,"production_webrtc":false,"turn":true,"video":true,"horizontal_sfu":true}}`))
			return
		}
		hits++
		w.WriteHeader(500)
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "opaque-machine-token", "integration-id", "provider-id", "https://join.example", server.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Prepare(context.Background(), mockEvent(now))
	if err != liveevents.ErrUnavailable || hits != 0 {
		t.Fatalf("false provider readiness allowed mutation: err=%v hits=%d", err, hits)
	}
}
func TestUCRServerOwnedConferenceJoinAndModeration(t *testing.T) {
	now := time.Date(2026, 10, 12, 11, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	requests := map[string]int{}
	var grantKeys []string
	var conferenceRefs []string
	var externalUsers []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path]++
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer machine-token" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/capabilities" {
			w.Write([]byte(`{"capabilities":{"max_participants":300,"browser_realtime_gateway":true,"production_webrtc":true,"turn":true,"video":true,"webinar":true,"horizontal_sfu":true}}`))
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(400)
			return
		}
		switch r.URL.Path {
		case "/v1/conferences":
			if request["external_conference_id_b64"] == "" {
				w.WriteHeader(400)
				return
			}
			w.Write([]byte(`{"conference":{"conference_id":"ucr-room-101","lifecycle":"scheduled"}}`))
		case "/v1/conferences/runtime":
			w.Write([]byte(`{"runtime":{"group_ready":true,"call_ready":true,"admitted_participant_count":0}}`))
		case "/v1/conferences/lifecycle":
			v, _ := request["target"].(string)
			json.NewEncoder(w).Encode(map[string]any{"conference": map[string]any{"lifecycle": v}})
		case "/v1/conferences/entry":
			w.Write([]byte(`{"conference":{"entry_open":true}}`))
		case "/v1/participants":
			if request["role"] != "attendee" && request["role"] != "host" {
				w.WriteHeader(403)
				return
			}
			w.Write([]byte(`{"participant":{"active":true}}`))
		case "/v1/participant-devices":
			w.Write([]byte(`{"device":{"active":true}}`))
		case "/v1/join-grants":
			mu.Lock()
			grantKeys = append(grantKeys, request["idempotency_key"].(string))
			conferenceRefs = append(conferenceRefs, request["conference_id"].(string))
			externalUsers = append(externalUsers, request["external_user_id_b64"].(string))
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"grant": map[string]any{
				"join_url":           "https://join.example/room#ucr_join=provider-signed-grant",
				"expires_at_unix_ms": now.Add(90 * time.Second).UnixMilli()}})
		case "/v1/participants/update":
			w.Write([]byte(`{"participant":{"audio_muted":true}}`))
		case "/v1/attendance":
			json.NewEncoder(w).Encode(map[string]any{"attendance": map[string]any{
				"join_count": 2, "media_ready_count": 1, "first_media_ready_at_unix_ms": now.Add(-30 * time.Second).UnixMilli(), "total_connected_seconds": 850, "first_join_at_unix_ms": now.Add(-time.Minute).UnixMilli()}})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "machine-token", "integration-id", "provider-id", "https://join.example", server.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	room, err := c.Prepare(context.Background(), mockEvent(now))
	if err != nil || room.ConferenceID != "ucr-room-101" {
		t.Fatalf("prepare: %+v %v", room, err)
	}
	if err = c.Change(context.Background(), room, liveevents.Published); err != nil {
		t.Fatal(err)
	}
	token, err := c.Join(context.Background(), room, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "attendee")
	if err != nil || !strings.Contains(token.JoinURL, "#ucr_join=") {
		t.Fatalf("join: %+v %v", token, err)
	}
	second, err := c.Join(context.Background(), room, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "attendee")
	if err != nil || second.JoinURL == "" {
		t.Fatalf("second attendee was denied a grant from the same UCR room: %v", err)
	}
	if err = c.Moderate(context.Background(), room, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "mute"); err != nil {
		t.Fatal(err)
	}
	attendance, err := c.Attendance(context.Background(), room, "cccccccc-cccc-4ccc-8ccc-cccccccccccc")
	if err != nil || attendance.JoinCount != 2 || !attendance.MediaReady || attendance.MediaReadyCount != 1 || attendance.ConnectedSeconds != 850 {
		t.Fatalf("attendance: %+v %v", attendance, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(grantKeys) != 2 || grantKeys[0] == grantKeys[1] ||
		len(conferenceRefs) != 2 || conferenceRefs[0] != room.ConferenceID ||
		conferenceRefs[1] != room.ConferenceID ||
		len(externalUsers) != 2 || externalUsers[0] == externalUsers[1] {
		t.Fatal("two-person UCR grant contract did not preserve one room and distinct participants")
	}
	for _, path := range []string{"/v1/conferences", "/v1/conferences/runtime", "/v1/conferences/lifecycle", "/v1/conferences/entry", "/v1/participants", "/v1/participant-devices", "/v1/join-grants", "/v1/participants/update", "/v1/attendance"} {
		if requests[path] == 0 {
			t.Errorf("UCR boundary missing request %s", path)
		}
	}
}
func TestUCRRejectsUntrustedGrantOrigin(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/capabilities":
			w.Write([]byte(`{"capabilities":{"max_participants":100,"browser_realtime_gateway":true,"production_webrtc":true,"turn":true,"video":true}}`))
		case "/v1/participants":
			w.Write([]byte(`{"participant":{"active":true}}`))
		case "/v1/participant-devices":
			w.Write([]byte(`{"device":{"active":true}}`))
		case "/v1/join-grants":
			json.NewEncoder(w).Encode(map[string]any{"grant": map[string]any{
				"join_url":           "https://attacker.example/room#ucr_join=fake",
				"expires_at_unix_ms": now.Add(time.Minute).UnixMilli()}})
		}
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "machine-token", "integration-id", "provider-id", "https://join.example", server.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	room := liveevents.MediaRoom{EventID: "event", OrganizationID: "org", IntegrationID: "integration-id", ConferenceID: "room", Format: liveevents.Group, Capacity: 10}
	grant, err := c.Join(context.Background(), room, "identity", "attendee")
	if err != liveevents.ErrUnavailable || grant.JoinURL != "" {
		t.Fatalf("untrusted origin accepted: %+v %v", grant, err)
	}
}

func TestUCRRoomCapacityIncludesHostAndNoConferenceIsCreatedOnShortfall(t *testing.T) {
	now := time.Date(2026, 10, 12, 11, 0, 0, 0, time.UTC)
	mutations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/capabilities" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"capabilities":{"max_participants":12,"browser_realtime_gateway":true,"production_webrtc":true,"turn":true,"video":true,"horizontal_sfu":true}}`))
			return
		}
		mutations++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "machine-test-credential", "integration-id", "provider-id", "https://join.example", server.Client(), func() time.Time { return now })
	if err != nil { t.Fatal(err) }
	_, err = c.Prepare(context.Background(), mockEvent(now))
	if err != liveevents.ErrUnavailable || mutations != 0 {
		t.Fatalf("host overflow accepted: error=%v UCR_mutations=%d", err, mutations)
	}
}
func TestUCRWaitingRoomAdmissionIsNotRealMediaAttendance(t *testing.T) {
	now := time.Date(2026, 10, 12, 11, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/attendance" { w.WriteHeader(http.StatusNotFound); return }
		_, _ = w.Write([]byte(`{"attendance":{"join_count":1,"media_ready_count":0,"total_connected_seconds":10,"first_join_at_unix_ms":1791793140000}}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "machine-test-credential", "integration-id", "provider-id", "https://join.example", server.Client(), func() time.Time { return now })
	if err != nil { t.Fatal(err) }
	room := liveevents.MediaRoom{OrganizationID:"tenant",ConferenceID:"ucr-room-101",IntegrationID:"integration-id"}
	fact, err := c.Attendance(context.Background(), room, "guest")
	if err != nil { t.Fatal(err) }
	if !fact.Joined || fact.MediaReady || fact.MediaReadyCount != 0 {
		t.Fatalf("waiting-room user counted as realtime participant: %+v", fact)
	}
}
func TestUCRRejectsUnpreparedRuntimeEvenWhenAPIConferenceExists(t *testing.T) {
	now := time.Date(2026, 10, 12, 11, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/capabilities":
			_, _ = w.Write([]byte(`{"capabilities":{"max_participants":300,"browser_realtime_gateway":true,"production_webrtc":true,"turn":true,"video":true}}`))
		case "/v1/conferences":
			_, _ = w.Write([]byte(`{"conference":{"conference_id":"ucr-room-101","lifecycle":"scheduled"}}`))
		case "/v1/conferences/runtime":
			_, _ = w.Write([]byte(`{"runtime":{"group_ready":true,"call_ready":false}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "machine-test-credential", "integration-id", "provider-id", "https://join.example", server.Client(), func() time.Time { return now })
	if err != nil { t.Fatal(err) }
	_, err = c.Prepare(context.Background(), mockEvent(now))
	if err != liveevents.ErrUnavailable { t.Fatalf("UCR not-media-ready room was certified: %v",err) }
}
