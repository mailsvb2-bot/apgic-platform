package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"net/http"
)

type academyStore interface {
	AcademyCreateCourse(string, academy.Course) (academy.Course, error)
	AcademyNewVersion(string, string) (academy.Course, error)
	AcademyAddModule(string, string, string, academy.Module) (academy.Module, error)
	AcademyAddLesson(string, string, string, string, academy.Lesson) (academy.Lesson, error)
	AcademyPublish(string, string, string) (academy.Course, error)
	AcademyCatalog() ([]academy.Course, error)
	AcademyEnroll(string, string) (academy.Enrollment, error)
	AcademyLearning(string, string) ([]academy.Lesson, error)
	AcademyMarkProgress(string, string, string) error
	AcademyComplete(string, string) (academy.Certificate, error)
	AcademyVerify(string) (map[string]any, error)
	AcademySubmitAttempt(string, string, string, string) (academy.Attempt, error)
	AcademyEvaluateAttempt(string, string, string, int) (academy.Attempt, error)
	AcademyAuthorCourses(string) ([]academy.Course, error)
	AcademyOutline(string, string, string) (map[string]any, error)
	AcademyMyEnrollments(string) ([]academy.Enrollment, error)
	AcademyPendingReviews(string) ([]academy.ReviewAttempt, error)
}

func registerAcademy(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("GET /v1/academy/courses", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, 503, "ACADEMY_UNAVAILABLE", "Обучение недоступно.", true, nil)
			return
		}
		courses, err := store.AcademyCatalog()
		if err != nil {
			writeDemandError(w, r, 503, "ACADEMY_UNAVAILABLE", "Обучение недоступно.", true, nil)
			return
		}
		writeJSON(w, 200, map[string]any{"courses": courses})
	})
	mux.HandleFunc("POST /v1/academy/courses", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		var input academy.Course
		if !academyRead(w, r, &input) {
			return
		}
		course, err := store.AcademyCreateCourse(identity, input)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, course)
	})
	mux.HandleFunc("POST /v1/academy/courses/{courseID}/versions", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		course, err := store.AcademyNewVersion(identity, r.PathValue("courseID"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, course)
	})
	mux.HandleFunc("POST /v1/academy/courses/{courseID}/versions/{versionID}/modules", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		var input academy.Module
		if !academyRead(w, r, &input) {
			return
		}
		module, err := store.AcademyAddModule(identity, r.PathValue("courseID"), r.PathValue("versionID"), input)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, module)
	})
	mux.HandleFunc("POST /v1/academy/courses/{courseID}/versions/{versionID}/modules/{moduleID}/lessons", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		var input academy.Lesson
		if !academyRead(w, r, &input) {
			return
		}
		lesson, err := store.AcademyAddLesson(identity, r.PathValue("courseID"), r.PathValue("versionID"), r.PathValue("moduleID"), input)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, lesson)
	})
	mux.HandleFunc("POST /v1/academy/courses/{courseID}/versions/{versionID}/publish", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		course, err := store.AcademyPublish(identity, r.PathValue("courseID"), r.PathValue("versionID"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, course)
	})

	mux.HandleFunc("POST /v1/academy/courses/{courseID}/enroll", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		enrollment, err := store.AcademyEnroll(identity, r.PathValue("courseID"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusCreated, enrollment)
	})
	mux.HandleFunc("GET /v1/academy/enrollments/{enrollmentID}/learn", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		lessons, err := store.AcademyLearning(identity, r.PathValue("enrollmentID"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"lessons": lessons})
	})
	mux.HandleFunc("POST /v1/academy/enrollments/{enrollmentID}/lessons/{lessonID}/complete", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		err := store.AcademyMarkProgress(identity, r.PathValue("enrollmentID"), r.PathValue("lessonID"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"state": "COMPLETED"})
	})
	mux.HandleFunc("POST /v1/academy/enrollments/{enrollmentID}/complete", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		certificate, err := store.AcademyComplete(identity, r.PathValue("enrollmentID"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusCreated, certificate)
	})
	mux.HandleFunc("GET /v1/academy/certificates/verify/{token}", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		certificate, err := store.AcademyVerify(r.PathValue("token"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, certificate)
	})

	mux.HandleFunc("POST /v1/academy/enrollments/{enrollmentID}/assessments/{lessonID}/attempts", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		var input struct {
			Response string `json:"response"`
		}
		if !academyRead(w, r, &input) {
			return
		}
		attempt, err := store.AcademySubmitAttempt(identity, r.PathValue("enrollmentID"), r.PathValue("lessonID"), input.Response)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusCreated, attempt)
	})
	mux.HandleFunc("POST /v1/academy/attempts/{attemptID}/evaluate", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		var input struct {
			Score       int    `json:"score"`
			EvidenceRef string `json:"evidence_ref"`
		}
		if !academyRead(w, r, &input) {
			return
		}
		attempt, err := store.AcademyEvaluateAttempt(identity, r.PathValue("attemptID"), input.EvidenceRef, input.Score)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, attempt)
	})

	mux.HandleFunc("GET /v1/academy/my-courses", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		courses, err := store.AcademyAuthorCourses(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 200, map[string]any{"courses": courses})
	})
	mux.HandleFunc("GET /v1/academy/courses/{courseID}/versions/{versionID}/outline", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		outline, err := store.AcademyOutline(actor, r.PathValue("courseID"), r.PathValue("versionID"))
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 200, outline)
	})
	mux.HandleFunc("GET /v1/academy/my-enrollments", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		enrollments, err := store.AcademyMyEnrollments(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 200, map[string]any{"enrollments": enrollments})
	})

	mux.HandleFunc("GET /v1/academy/review/attempts", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		attempts, err := store.AcademyPendingReviews(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 200, map[string]any{"attempts": attempts})
	})

}

func academyRead(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeDemandError(w, r, 400, "ACADEMY_INVALID", "Некорректный формат запроса.", false, nil)
		return false
	}
	return true
}
func academyHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusServiceUnavailable
	code := "ACADEMY_UNAVAILABLE"
	message := "Образовательный контур временно недоступен."
	switch {
	case errors.Is(err, academy.ErrInvalid):
		status = 400
		code = "ACADEMY_INVALID"
		message = "Проверьте данные курса."
	case errors.Is(err, academy.ErrNotFound):
		status = 404
		code = "ACADEMY_NOT_FOUND"
		message = "Образовательный ресурс не найден."
	case errors.Is(err, academy.ErrDenied):
		status = 403
		code = "ACADEMY_ACCESS_DENIED"
		message = "Недостаточно полномочий."
	case errors.Is(err, academy.ErrEntitlement):
		status = 402
		code = "ACADEMY_ENTITLEMENT_REQUIRED"
		message = "Необходимо подтверждённое право доступа."
	case errors.Is(err, academy.ErrState):
		status = 409
		code = "ACADEMY_STATE_CONFLICT"
		message = "Недопустимое изменение состояния."
	case errors.Is(err, academy.ErrConflict):
		status = 409
		code = "ACADEMY_CONFLICT"
		message = "Конфликт конкурентного изменения."
	}
	writeDemandError(w, r, status, code, message, status >= 500, nil)
}
