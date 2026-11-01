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

func TestAcademyCreateCourseConcurrentRetry(t *testing.T) {
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
	author, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	product, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", author); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO products(id,owner_type,owner_id,commercial_owner_ref,revenue_beneficiary_ref,author_refs,name,status,published_at)
 VALUES($1::uuid,'IDENTITY',$2::uuid,$3,$3,ARRAY[$3]::text[],'Course retry fixture','PUBLISHED',now())`,
		product, author, "identity/"+author); err != nil {
		t.Fatal(err)
	}
	input := academy.Course{ProductID: product, Title: "Concurrent author course", Description: "Canonical retry"}
	const count = 12
	results := make([]academy.Course, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = store.AcademyCreateCourse(author, input)
		}(i)
	}
	wg.Wait()
	for i := 0; i < count; i++ {
		if errs[i] != nil || results[i].ID == "" || results[i].ID != results[0].ID {
			t.Fatalf("request %d: course=%+v err=%v", i, results[i], errs[i])
		}
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM academy_courses WHERE product_id=$1::uuid", product).Scan(&n); err != nil || n != 1 {
		t.Fatalf("course count=%d err=%v", n, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_ref=$1", "academy/course/"+results[0].ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("outbox count=%d err=%v", n, err)
	}
	again, err := store.AcademyCreateCourse(author, input)
	if err != nil || again.ID != results[0].ID {
		t.Fatalf("retry=%+v err=%v", again, err)
	}
	_, err = store.AcademyCreateCourse(author, academy.Course{ProductID: product, Title: "Different title"})
	if !errors.Is(err, academy.ErrConflict) {
		t.Fatalf("changed request must conflict: %v", err)
	}
	outsider, err := academyID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO identities(id) VALUES($1::uuid)", outsider); err != nil {
		t.Fatal(err)
	}
	_, err = store.AcademyCreateCourse(outsider, input)
	if !errors.Is(err, academy.ErrDenied) {
		t.Fatalf("foreign actor: %v", err)
	}
}
