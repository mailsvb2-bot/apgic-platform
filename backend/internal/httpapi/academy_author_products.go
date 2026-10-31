package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyAuthorProductStore interface {
	AcademyAuthorProducts(string) ([]academy.AuthorProduct, error)
}

func registerAcademyAuthorProducts(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("GET /v1/academy/author/eligible-products", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		owner, ok := store.(academyAuthorProductStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		products, err := owner.AcademyAuthorProducts(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"products": products})
	})
}
