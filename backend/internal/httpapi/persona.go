package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persona"
)

type PersonaStore interface {
	ListPersonas(context.Context, string, string) ([]persona.CatalogItem, error)
	BeginAvatar(context.Context, string, string, string, string, persona.Start) (persona.Session, error)
	ActivateAvatar(context.Context, string, string, string, string) error
	FailAvatar(context.Context, string, string) error
	EndAvatar(context.Context, string, string) error
	HandoffAvatar(context.Context, string, string) error
	EscalateAvatar(context.Context, string, string) error
	ReadAvatar(context.Context, string, string) (persona.Session, error)
	AuthorizeAvatarTurn(context.Context, string, string) error
}
type personaAPIConfig struct {
	Enabled            bool
	ProviderInstanceID string
	PolicyVersion      string
}

func registerPersona(mux *http.ServeMux, store PersonaStore, provider connector.PersonaRuntime, cfg personaAPIConfig,
	sessions *clientSessionManager, sessionConfigErr error) {
	enabled := func() bool {
		return cfg.Enabled && store != nil && provider != nil &&
			strings.TrimSpace(cfg.ProviderInstanceID) != "" && strings.TrimSpace(cfg.PolicyVersion) != ""
	}
	mux.HandleFunc("GET /v1/personas", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !enabled() {
			writeJSON(w, http.StatusOK, map[string]any{"available": false, "reason_code": "PERSONA_NOT_CONFIGURED", "personas": []persona.CatalogItem{}})
			return
		}
		if probe, ok := provider.(interface{ Ready(context.Context) bool }); ok && !probe.Ready(r.Context()) {
			writeJSON(w, http.StatusOK, map[string]any{"available": false, "reason_code": persona.ReasonProviderUnavailable, "personas": []persona.CatalogItem{}})
			return
		}
		items, err := store.ListPersonas(r.Context(), cfg.ProviderInstanceID, cfg.PolicyVersion)
		if err != nil {
			writePersonaError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": len(items) > 0, "disclosure": "AI_AVATAR_NOT_HUMAN_SPECIALIST",
			"privacy_mode": persona.PrivateEphemeral, "personas": items})
	})
	mux.HandleFunc("POST /v1/avatar-sessions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !enabled() {
			writePersonaError(w, r, persona.ErrUnavailable)
			return
		}
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, 503, "CLIENT_SESSION_UNAVAILABLE", "Клиентская сессия не настроена.", false, nil)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var body persona.Start
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.Validate() != nil {
			writePersonaError(w, r, persona.ErrInvalidRequest)
			return
		}
		idem := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(idem) < 8 || len(idem) > 128 {
			writePersonaError(w, r, persona.ErrInvalidRequest)
			return
		}
		identity, cookie, err := sessions.identityForCreate(r)
		if err != nil {
			writeDemandError(w, r, 401, "CLIENT_SESSION_INVALID", "Сессия клиента недействительна.", false, nil)
			return
		}
		session, err := store.BeginAvatar(r.Context(), identity, cfg.ProviderInstanceID, idem, cfg.PolicyVersion, body)
		if err != nil {
			writePersonaError(w, r, err)
			return
		}
		if cookie != nil {
			http.SetCookie(w, cookie)
		}
		result, err := provider.Open(r.Context(), connector.PersonaOpen{
			APGICSessionID: session.ID, PersonaID: session.PersonaID, Purpose: session.Purpose,
			PrivacyMode: session.PrivacyMode, Mode: session.Mode, Style: session.Style, Language: session.Language,
			VoiceProfile: session.VoiceProfile, VisualProfile: session.VisualProfile, PolicyVersion: cfg.PolicyVersion,
		})
		if err != nil {
			_ = store.FailAvatar(r.Context(), session.ID, identity)
			writeDemandError(w, r, 503, persona.ReasonProviderUnavailable,
				"Сейчас аватар недоступен. Можно перейти к живому специалисту или попробовать позже.", true, nil)
			return
		}
		if err := store.ActivateAvatar(r.Context(), session.ID, identity, result.SessionRef, result.Mode); err != nil {
			_ = provider.Close(r.Context(), result.SessionRef)
			writePersonaError(w, r, err)
			return
		}
		session, err = store.ReadAvatar(r.Context(), session.ID, identity)
		if err != nil {
			writePersonaError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"session": session,
			"transport":  map[string]any{"launch_url": result.LaunchURL, "expires_at": result.ExpiresAt, "mode": result.Mode},
			"disclosure": "Это AI-аватар, а не живой специалист. Он не диагностирует и не заменяет профессиональную помощь."})
	})
	// Session-scoped actual model exchange, not a link or synthetic text.
	// Transcripts are deliberately neither logged nor persisted by APGIC.
	mux.HandleFunc("POST /v1/avatar-sessions/{id}/turns", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !enabled() {
			writePersonaError(w, r, persona.ErrUnavailable)
			return
		}
		owner, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !persona.ValidID(id) {
			writePersonaError(w, r, persona.ErrInvalidRequest)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2300)
		var input struct {
			Message string `json:"message"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Message) == "" || len(input.Message) > 2000 {
			writePersonaError(w, r, persona.ErrInvalidRequest)
			return
		}
		session, err := store.ReadAvatar(r.Context(), id, owner)
		if err != nil {
			writePersonaError(w, r, err)
			return
		}
		if session.State != persona.StateActive || session.SafetyState != "NORMAL" || session.HandoffRequestedAt != nil || session.ProviderSessionRef == "" {
			writePersonaError(w, r, persona.ErrConflict)
			return
		}
		// SQL authorization checks current session, publication, model policy and kill switch.
		if err := store.AuthorizeAvatarTurn(r.Context(), id, owner); err != nil {
			writePersonaError(w, r, err)
			return
		}
		reply, err := provider.Turn(r.Context(), session.ProviderSessionRef, input.Message)
		if err != nil || strings.TrimSpace(reply) == "" {
			writeDemandError(w, r, 503, persona.ReasonProviderUnavailable, "ИИ-собеседник не ответил. Сообщение не было подменено готовым текстом.", true, nil)
			return
		}
		// A concurrent END/HANDOFF must invalidate any answer still in flight.
		if err := store.AuthorizeAvatarTurn(r.Context(), id, owner); err != nil {
			writePersonaError(w, r, persona.ErrConflict)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"reply": reply, "source": "MODEL_GENERATED"})
	})
	mux.HandleFunc("GET /v1/avatar-sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if store == nil {
			writePersonaError(w, r, persona.ErrUnavailable)
			return
		}
		owner, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		if !persona.ValidID(r.PathValue("id")) {
			writePersonaError(w, r, persona.ErrInvalidRequest)
			return
		}
		session, err := store.ReadAvatar(r.Context(), r.PathValue("id"), owner)
		if err != nil {
			writePersonaError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, session)
	})
	mux.HandleFunc("POST /v1/avatar-sessions/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if store == nil {
			writePersonaError(w, r, persona.ErrUnavailable)
			return
		}
		owner, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var request struct {
			Action string `json:"action"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writePersonaError(w, r, persona.ErrInvalidRequest)
			return
		}
		id := r.PathValue("id")
		if !persona.ValidID(id) {
			writePersonaError(w, r, persona.ErrInvalidRequest)
			return
		}
		current, err := store.ReadAvatar(r.Context(), id, owner)
		if err != nil {
			writePersonaError(w, r, err)
			return
		}
		switch request.Action {
		case "HUMAN_HANDOFF":
			err = store.HandoffAvatar(r.Context(), id, owner)
		case "SAFETY_ESCALATE":
			err = store.EscalateAvatar(r.Context(), id, owner)
		case "END":
			if current.State == persona.StateActive {
				err = store.EndAvatar(r.Context(), id, owner)
			} else if current.State != persona.StateEnded {
				err = persona.ErrConflict
			}
		default:
			writePersonaError(w, r, persona.ErrInvalidRequest)
			return
		}
		if err != nil {
			writePersonaError(w, r, err)
			return
		}
		// Human handoff and safety escalation revoke conversation authority;
		// they cannot silently keep paid/private provider media active.
		if request.Action != "END" && current.State == persona.StateActive {
			if err := store.EndAvatar(r.Context(), id, owner); err != nil {
				writePersonaError(w, r, err)
				return
			}
		}
		updated, err := store.ReadAvatar(r.Context(), id, owner)
		if err != nil {
			writePersonaError(w, r, err)
			return
		}
		response := map[string]any{"session": updated}
		if request.Action == "HUMAN_HANDOFF" {
			response["handoff"] = persona.SafeHandoffProjection(updated)
			response["next_url"] = "/#start"
			response["notice"] = "Ничего из разговора не передано специалисту. Сформулируйте запрос самостоятельно."
		}
		if request.Action == "SAFETY_ESCALATE" {
			response["notice"] = "При непосредственной опасности обратитесь в местные экстренные службы или к человеку рядом. Доступ к помощи не требует оплаты."
		}
		if provider != nil && current.ProviderSessionRef != "" {
			// APGIC ends business access regardless of provider outage; never
			// silently claim the remote media runtime has stopped if delivery failed.
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			if e := provider.Close(cleanupCtx, current.ProviderSessionRef); e != nil {
				response["provider_close"] = "DELIVERY_UNCERTAIN"
			} else {
				response["provider_close"] = "CONFIRMED"
			}
		}
		writeJSON(w, http.StatusOK, response)
	})
}
func writePersonaError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, persona.ErrInvalidRequest):
		writeDemandError(w, r, 400, "PERSONA_REQUEST_INVALID", "Некорректные параметры аватара.", false, nil)
	case errors.Is(err, persona.ErrNotFound):
		writeDemandError(w, r, 404, "PERSONA_NOT_FOUND", "Аватар или сессия не найдены.", false, nil)
	case errors.Is(err, persona.ErrIdempotency):
		writeDemandError(w, r, 409, "PERSONA_IDEMPOTENCY_CONFLICT", "Повторный запрос уже обработан; создайте новый ключ для новой сессии.", false, nil)
	case errors.Is(err, persona.ErrBudgetExceeded):
		writeDemandError(w, r, 429, "PERSONA_BUDGET_EXHAUSTED", "Лимит запусков ИИ-персонажа исчерпан. Вы можете обратиться к живому специалисту.", false, nil)
	case errors.Is(err, persona.ErrConcurrencyExceeded):
		writeDemandError(w, r, 409, "PERSONA_CONCURRENT_LIMIT", "Завершите предыдущий разговор перед запуском нового.", false, nil)
	case errors.Is(err, persona.ErrConflict):
		writeDemandError(w, r, 409, "PERSONA_STATE_CONFLICT", "Действие недоступно в текущем состоянии.", false, nil)
	default:
		writeDemandError(w, r, 503, "PERSONA_NOT_CONFIGURED", "Функция аватара пока недоступна. Доступен обычный подбор специалиста.", true, nil)
	}
}
