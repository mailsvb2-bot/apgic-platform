package main

import (
	"log"
	"net/http"
	"strings"
	"time"
)

func newReferenceService(c config) *referenceService {
	return &referenceService{config: c, clock: time.Now, httpClient: &http.Client{
		Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}
func (s *referenceService) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.ready() != nil {
			http.Error(w, "provider is not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /v1/room-grants", s.handleIssue)
	mux.HandleFunc("POST /v1/livekit-webhooks", s.handleLiveKitWebhook)
	mux.HandleFunc("POST /v1/rooms/finish", s.handleFinish)
	mux.HandleFunc("GET /v1/media-config", s.handleMediaConfig)
	mux.HandleFunc("GET /room/{bookingID}", func(w http.ResponseWriter, r *http.Request) {
		s.handleRoom(w, r)
	})
	mux.HandleFunc("GET /room.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		http.ServeFile(w, r, "static/room.js")
	})
	mux.HandleFunc("GET /room.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		http.ServeFile(w, r, "static/room.css")
	})
	mux.HandleFunc("GET /assets/livekit-client.umd.min.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public,max-age=86400")
		http.ServeFile(w, r, s.config.ClientBundle)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(self), microphone=(self), geolocation=()")
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

func main() {
	c, err := readConfig()
	if err != nil {
		log.Fatal("reference CommunicationProvider configuration invalid: ", err)
	}
	s := newReferenceService(c)
	if err = s.ready(); err != nil {
		log.Fatal("reference CommunicationProvider not ready: ", err)
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			s.flushAll()
		}
	}()
	log.Printf("reference CommunicationProvider listening on %s (media held by configured SFU)", c.Listen)
	if err = http.ListenAndServe(c.Listen, s.handler()); err != nil {
		log.Fatal(err)
	}
}
