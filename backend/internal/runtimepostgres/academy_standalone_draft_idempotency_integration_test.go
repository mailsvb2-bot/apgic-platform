package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func TestAcademyStandaloneDraftIdempotentReplayAndIsolation(t *testing.T) {
	dsn := os.Getenv("APGIC_ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, _ := academyID()
	foreign, _ := academyID()
	for _, actor := range []string{owner, foreign} {
		if _, err := db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", actor); err != nil {
			t.Fatal(err)
		}
	}

	key, _ := academyID()
	first, err := store.AcademyCreateStandaloneDraft(owner, "№1", "Учебный курс", key)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.AcademyCreateStandaloneDraft(owner, "№1", "Учебный курс", key)
	if err != nil || replay.ID != first.ID || replay.VersionID != first.VersionID || replay.ProductID != first.ProductID {
		t.Fatalf("same owner/key created a second draft: first=%+v replay=%+v err=%v", first, replay, err)
	}
	if _, err := store.AcademyCreateStandaloneDraft(owner, "Другое название", "Учебный курс", key); !errors.Is(err, academy.ErrConflict) {
		t.Fatalf("same key with changed input must be rejected, got %v", err)
	}
	other, err := store.AcademyCreateStandaloneDraft(foreign, "№1", "Учебный курс", key)
	if err != nil || other.ID == first.ID {
		t.Fatalf("different owner must not see or reuse private draft: %+v %v", other, err)
	}

	concurrentKey, _ := academyID()
	const count = 8
	var wg sync.WaitGroup
	ids := make(chan string, count)
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			created, err := store.AcademyCreateStandaloneDraft(owner, "Конкурентный черновик", "", concurrentKey)
			if err != nil {
				errs <- err
				return
			}
			ids <- created.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var concurrentID string
	for id := range ids {
		if concurrentID == "" {
			concurrentID = id
		}
		if concurrentID != id {
			t.Fatalf("concurrent duplicate: %s versus %s", concurrentID, id)
		}
	}
	if concurrentID == "" {
		t.Fatal("concurrent attempts returned no draft")
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM academy_courses WHERE author_id=$1::uuid AND author_request_id=$2", owner, concurrentKey).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expected one canonical persisted draft, count=%d err=%v", n, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_ref=$1", "academy/course/"+concurrentID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expected one canonical creation event, count=%d err=%v", n, err)
	}
}

[executed on device: msk-1-vm-9vrn (ce05cfe4-fa8e-495a-bd76-12d14a659df2)]
