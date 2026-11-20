package runtimepostgres

import (
	"database/sql"
	"fmt"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"strings"
)

// AcademyEnsureIdentity persists the existing signed first-party principal
// in the one canonical identities table before the session cookie is issued.
// A failed write must never result in a seemingly valid author session.
func (c *Checker) AcademyEnsureIdentity(actor string) error {
	if c == nil || c.db == nil {
		return academy.ErrUnavailable
	}
	if actor == "" {
		return academy.ErrInvalid
	}
	ctx, cancel := academyContext()
	defer cancel()
	_, err := c.db.ExecContext(ctx,
		`INSERT INTO identities(id) VALUES($1::uuid) ON CONFLICT(id) DO NOTHING`, actor)
	if err != nil {
		return fmt.Errorf("ensure signed academy identity: %w", err)
	}
	return nil
}

// AcademyCreateStandaloneDraft writes a private unsellable Commerce DRAFT and
// an Academy DRAFT in one transaction. It does not grant learning access.
func (c *Checker) AcademyCreateStandaloneDraft(actor, title, description string, requestIDs ...string) (academy.Course, error) {
	title = strings.TrimSpace(title)
	if actor == "" || len(title) < 1 || len(title) > 200 || len(requestIDs) > 1 {
		return academy.Course{}, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.Course{}, err
	}
	defer tx.Rollback()
	// A newly issued signed Academy session may not yet have an Identity row.
	// Register its canonical principal in this same transaction BEFORE the
	// owner-bound product trigger and Academy foreign key are evaluated.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO identities(id) VALUES($1::uuid) ON CONFLICT(id) DO NOTHING`,
		actor); err != nil {
		return academy.Course{}, fmt.Errorf("ensure signed academy author identity: %w", err)
	}
	var requestID string
	if len(requestIDs) == 1 {
		requestID = strings.TrimSpace(requestIDs[0])
		if requestID == "" || len(requestID) > 128 {
			return academy.Course{}, academy.ErrInvalid
		}
		// Serialize only retries of the same owner's request. The query below
		// always scopes its result by signed author, even on hash collision.
		if _, err := tx.ExecContext(ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			actor+"/"+requestID); err != nil {
			return academy.Course{}, err
		}
		var existing academy.Course
		err = tx.QueryRowContext(ctx, `SELECT c.id::text,c.product_id::text,
 c.title,c.description,c.state,v.id::text,v.version_no
 FROM academy_courses c JOIN academy_course_versions v ON v.course_id=c.id
 WHERE c.author_id=$1::uuid AND c.author_request_id=$2 AND v.version_no=1`,
			actor, requestID).Scan(&existing.ID, &existing.ProductID,
			&existing.Title, &existing.Description, &existing.State,
			&existing.VersionID, &existing.VersionNo)
		if err == nil {
			if existing.Title != title || existing.Description != description {
				return academy.Course{}, academy.ErrConflict
			}
			existing.AuthorID = actor
			if err := tx.Commit(); err != nil {
				return academy.Course{}, err
			}
			return existing, nil
		}
		if err != sql.ErrNoRows {
			return academy.Course{}, err
		}
	}
	productID, err := academyID()
	if err != nil {
		return academy.Course{}, err
	}
	courseID, err := academyID()
	if err != nil {
		return academy.Course{}, err
	}
	versionID, err := academyID()
	if err != nil {
		return academy.Course{}, err
	}
	ownerRef := "identity/" + actor
	_, err = tx.ExecContext(ctx, `INSERT INTO products(
 id,owner_type,owner_id,commercial_owner_ref,revenue_beneficiary_ref,author_refs,name,status)
 VALUES($1::uuid,'IDENTITY',$2::uuid,$3,$3,ARRAY[$3]::text[],$4,'DRAFT')`,
		productID, actor, ownerRef, title)
	if err != nil {
		return academy.Course{}, fmt.Errorf("create private course product draft: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_courses(id,product_id,author_id,title,description,author_request_id)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6)`, courseID, productID, actor, title, description,
		sql.NullString{String: requestID, Valid: requestID != ""})
	if err != nil {
		return academy.Course{}, fmt.Errorf("create academy draft: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_course_versions(id,course_id,version_no)
 VALUES($1::uuid,$2::uuid,1)`, versionID, courseID)
	if err != nil {
		return academy.Course{}, err
	}
	if err = academyEvent(ctx, tx, actor, "academy/course/"+courseID, "academy.course.created",
		map[string]string{"author_id": actor, "version_id": versionID}); err != nil {
		return academy.Course{}, err
	}
	if err = tx.Commit(); err != nil {
		return academy.Course{}, err
	}
	return academy.Course{ID: courseID, ProductID: productID, Title: title, Description: description, AuthorID: actor,
		State: "DRAFT", VersionID: versionID, VersionNo: 1}, nil
}
