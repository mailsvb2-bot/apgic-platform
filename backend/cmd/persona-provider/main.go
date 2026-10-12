package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Independent private runtime: real inference, ephemeral context, no database,
// no transcript logging and no implicit model or voice fallback.
const maxActiveSessions = 100
const maxSessionTurns = 16
const sessionTTL = 12 * time.Minute

var validUUID = regexp.MustCompile(`^[a-fA-F0-9]{8}(-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12}$`)

type config struct {
	token, modelURL, modelKey, model, policy string
	client                                   *http.Client
}
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type session struct {
	mode, style, language, purpose string
	history                        []message
	turns                          int
	expires                        time.Time
	busy                           bool
	cancel                         context.CancelFunc
}
type service struct {
	cfg      config
	mu       sync.Mutex
	sessions map[string]*session
}

func newService(c config) *service {
	if c.client == nil {
		c.client = &http.Client{Timeout: 23 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &service{cfg: c, sessions: make(map[string]*session)}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "INVALID_REQUEST", 400)
		return false
	}
	return true
}
func replyJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func allowed(v string, options ...string) bool {
	for _, x := range options {
		if v == x {
			return true
		}
	}
	return false
}
func (s *service) reap() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.sessions {
		if time.Now().After(v.expires) {
			if v.cancel != nil {
				v.cancel()
			}
			v.history = nil
			delete(s.sessions, k)
		}
	}
}
func (s *service) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/persona-sessions", s.open)
	mux.HandleFunc("POST /v1/persona-sessions/turns", s.turn)
	mux.HandleFunc("POST /v1/persona-sessions/close", s.close)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		replyJSON(w, 200, map[string]string{"status": "ok", "mode": "MODEL_BACKED"})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path != "/healthz" {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.cfg.token)) != 1 {
				http.Error(w, "UNAUTHORIZED", 401)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

type openRequest struct {
	APGICSessionID string `json:"apgic_session_id"`
	PersonaID      string `json:"persona_id"`
	Purpose        string `json:"session_purpose"`
	PrivacyMode    string `json:"privacy_mode"`
	Mode           string `json:"mode"`
	Style          string `json:"style"`
	Language       string `json:"language"`
	VoiceProfile   string `json:"voice_profile"`
	VisualProfile  string `json:"visual_profile"`
	PolicyVersion  string `json:"policy_version"`
}

func (s *service) open(w http.ResponseWriter, r *http.Request) {
	var q openRequest
	if !decode(w, r, &q) {
		return
	}
	if !validUUID.MatchString(q.APGICSessionID) || !validUUID.MatchString(q.PersonaID) ||
		q.PrivacyMode != "PRIVATE_EPHEMERAL" || q.PolicyVersion != s.cfg.policy ||
		!allowed(q.Mode, "TEXT", "VOICE") ||
		!allowed(q.Style, "CALM", "NEUTRAL", "BUSINESS", "WARM") ||
		!allowed(q.Purpose, "LISTEN", "ORGANIZE_THOUGHTS", "PREPARE_HUMAN", "WAIT_FOR_HUMAN", "NAVIGATE") ||
		!allowed(q.Language, "ru", "en", "ru-RU", "en-US") ||
		len(q.VoiceProfile) > 120 || len(q.VisualProfile) > 120 {
		http.Error(w, "POLICY_OR_MODE_DENIED", 403)
		return
	}
	s.reap()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) >= maxActiveSessions {
		http.Error(w, "RESOURCE_LIMIT", 429)
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		http.Error(w, "ENTROPY_UNAVAILABLE", 503)
		return
	}
	ref := hex.EncodeToString(raw)
	expires := time.Now().Add(sessionTTL)
	s.sessions[ref] = &session{mode: q.Mode, style: q.Style, language: q.Language, purpose: q.Purpose, expires: expires}
	// Inline TEXT/VOICE needs no external browser URL or token. APGIC owns the session cookie.
	replyJSON(w, 201, map[string]any{"session_ref": ref, "launch_url": "", "mode": q.Mode, "expires_at": expires.UTC()})
}
func (s *service) close(w http.ResponseWriter, r *http.Request) {
	var q struct {
		SessionRef string `json:"session_ref"`
	}
	if !decode(w, r, &q) {
		return
	}
	if len(q.SessionRef) != 64 {
		http.Error(w, "INVALID_REFERENCE", 400)
		return
	}
	s.mu.Lock()
	if v := s.sessions[q.SessionRef]; v != nil {
		if v.cancel != nil {
			v.cancel()
		}
		v.history = nil
		delete(s.sessions, q.SessionRef)
	}
	s.mu.Unlock()
	replyJSON(w, 200, map[string]bool{"deleted": true})
}
func (s *service) turn(w http.ResponseWriter, r *http.Request) {
	var q struct {
		SessionRef string `json:"session_ref"`
		Message    string `json:"message"`
	}
	if !decode(w, r, &q) {
		return
	}
	q.Message = strings.TrimSpace(q.Message)
	if q.Message == "" || len(q.Message) > 2000 || !utf8.ValidString(q.Message) || strings.ContainsRune(q.Message, 0) {
		http.Error(w, "INVALID_MESSAGE", 400)
		return
	}
	s.mu.Lock()
	v := s.sessions[q.SessionRef]
	if v == nil || time.Now().After(v.expires) {
		s.mu.Unlock()
		http.Error(w, "SESSION_NOT_ACTIVE", 410)
		return
	}
	if v.busy || v.turns >= maxSessionTurns {
		s.mu.Unlock()
		http.Error(w, "SESSION_TURN_LIMIT_OR_BUSY", 429)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	v.cancel = cancel
	v.busy = true
	history := append([]message(nil), v.history...)
	s.mu.Unlock()
	answer, err := s.infer(ctx, v, history, q.Message)
	s.mu.Lock()
	current := s.sessions[q.SessionRef]
	if current != v || time.Now().After(v.expires) {
		s.mu.Unlock()
		cancel()
		http.Error(w, "SESSION_REVOKED", 410)
		return
	}
	v.busy = false
	v.cancel = nil
	if err == nil {
		v.turns++
		v.history = append(v.history, message{"user", q.Message}, message{"assistant", answer})
		if len(v.history) > 12 {
			v.history = append([]message(nil), v.history[len(v.history)-12:]...)
		}
	}
	s.mu.Unlock()
	cancel()
	if err != nil {
		http.Error(w, "PERSONA_MODEL_UNAVAILABLE", 503)
		return
	}
	replyJSON(w, 200, map[string]string{"reply": answer, "source": "MODEL_GENERATED"})
}
func (s *service) infer(ctx context.Context, v *session, history []message, input string) (string, error) {
	policy := "Ты явно обозначенный ИИ-собеседник, не живой специалист. Не ставь диагнозы, не назначай лечение и не изображай реального человека. Не обещай исключительную эмоциональную связь и не выдумывай квалификацию. При непосредственной опасности советуй обратиться к местным экстренным службам или человеку рядом. Не навязывай оплату. Говори тепло и кратко."
	if strings.HasPrefix(v.language, "en") {
		policy = "You are clearly disclosed as an AI, never a human or clinician. No diagnosis, treatment, invented credentials, dependency or sales pressure. In immediate danger, encourage local emergency services and human support. Be kind and brief."
	}
	policy += " Style: " + v.style + ". Purpose: " + v.purpose + "."
	messages := append([]message{{"system", policy}}, history...)
	messages = append(messages, message{"user", input})
	body, err := json.Marshal(map[string]any{"model": s.cfg.model, "messages": messages, "max_tokens": 320, "temperature": 0.5, "stream": false})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", s.cfg.modelURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.modelKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.cfg.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", errors.New("model unavailable")
	}
	var out struct {
		Choices []struct {
			Message      message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 16384)).Decode(&out) != nil || len(out.Choices) != 1 || out.Choices[0].FinishReason != "stop" {
		return "", errors.New("model response invalid")
	}
	answer := strings.TrimSpace(out.Choices[0].Message.Content)
	if out.Choices[0].Message.Role != "assistant" || answer == "" || len(answer) > 4500 || !utf8.ValidString(answer) {
		return "", errors.New("model reply rejected")
	}
	return answer, nil
}
func configFromEnv() (config, error) {
	c := config{token: os.Getenv("PERSONA_PROVIDER_SERVICE_TOKEN"), modelURL: os.Getenv("PERSONA_PROVIDER_LLM_URL"),
		modelKey: os.Getenv("PERSONA_PROVIDER_LLM_API_KEY"), model: os.Getenv("PERSONA_PROVIDER_LLM_MODEL"), policy: os.Getenv("PERSONA_PROVIDER_POLICY_VERSION")}
	u, err := url.Parse(c.modelURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" ||
		!strings.HasSuffix(u.Path, "/chat/completions") || len(c.token) < 24 || len(c.modelKey) < 16 || c.model == "" || c.policy == "" {
		return config{}, fmt.Errorf("PersonaProvider model, credentials or HTTPS endpoint not configured")
	}
	return c, nil
}
func main() {
	cfg, err := configFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	svc := newService(cfg)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			svc.reap()
		}
	}()
	log.Println("Private, model-backed PersonaProvider listening on loopback; no transcript persistence")
	server := &http.Server{Addr: "127.0.0.1:8788", Handler: svc.handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 27 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	log.Fatal(server.ListenAndServe())
}
