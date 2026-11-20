package httpapi

import (
    "context"
    "database/sql"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "strings"
    "testing"

    _ "github.com/jackc/pgx/v5/stdlib"
    "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
    "github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func TestAcademySessionStartCreatesPersistedAuthorBeforeFirstDraftPostgres(t *testing.T) {
    dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
    if dsn == "" { t.Skip("isolated PostgreSQL required") }
    store, err := runtimepostgres.Open(context.Background(), dsn)
    if err != nil { t.Fatal(err) }
    defer store.Close()
    db, err := sql.Open("pgx", dsn)
    if err != nil { t.Fatal(err) }
    defer db.Close()
    handler := New(Options{ClientSessionKey: []byte(strings.Repeat("s", 40)), Academy: store})

    startSession := func() *http.Cookie {
        t.Helper()
        req := httptest.NewRequest("POST", "https://apgic.ru/v1/academy/session/start", strings.NewReader("{}"))
        req.Header.Set("Origin", "https://apgic.ru")
        w := httptest.NewRecorder()
        handler.ServeHTTP(w, req)
        if w.Code != 200 || len(w.Result().Cookies()) != 1 {
            t.Fatalf("signed author session not issued: status=%d body=%s", w.Code, w.Body.String())
        }
        return w.Result().Cookies()[0]
    }
    post := func(cookie *http.Cookie, body string) (academy.Course, int) {
        t.Helper()
        req := httptest.NewRequest("POST", "/v1/academy/drafts", strings.NewReader(body))
        req.Header.Set("Content-Type", "application/json")
        req.AddCookie(cookie)
        w := httptest.NewRecorder()
        handler.ServeHTTP(w, req)
        var course academy.Course
        if w.Code == http.StatusCreated {
            if err := json.Unmarshal(w.Body.Bytes(), &course); err != nil {t.Fatal(err)}
        } else {
            t.Logf("draft POST status=%d body=%s", w.Code, w.Body.String())
        }
        return course, w.Code
    }
    cookie := startSession()
    key := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
    input := `{"title":"гипноз","description":"","request_id":"`+key+`"}`
    created, status := post(cookie,input)
    if status != http.StatusCreated || created.ID == "" || created.ProductID == "" || created.VersionID == "" {
        t.Fatalf("first signed author draft was rejected: status=%d %+v",status,created)
    }
    var count int
    if err := db.QueryRow("SELECT COUNT(*) FROM identities WHERE id=$1::uuid",created.AuthorID).Scan(&count);err != nil||count!=1 {
        t.Fatalf("signed author is not persisted in canonical identities: count=%d err=%v",count,err)
    }
    again,status := post(cookie,input)
    if status != http.StatusCreated || again.ID != created.ID || again.ProductID != created.ProductID {
        t.Fatalf("same request did not replay original draft: %+v %+v status=%d",created,again,status)
    }
    foreignCookie := startSession()
    foreign,status := post(foreignCookie,input)
    if status != http.StatusCreated || foreign.ID==created.ID || foreign.AuthorID==created.AuthorID {
        t.Fatalf("other signed author reused private course: %+v %+v status=%d",created,foreign,status)
    }
}
