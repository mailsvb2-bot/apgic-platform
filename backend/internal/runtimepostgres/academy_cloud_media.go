package runtimepostgres

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type AcademyCloudMedia = academy.CloudMedia
type AcademyMediaAssignment = academy.MediaAssignment

// AcademyAssignCloudMedia atomically replaces the complete DRAFT version's
// ordered VIDEO media snapshot. Omitted lessons lose old assignments, and
// an empty snapshot explicitly clears all video assignments. This prevents
// removed videos from remaining accessible after an author saves edits.
// Neither source URLs nor provider identifiers are exposed to learner APIs.
func (c *Checker) AcademyAssignCloudMedia(actor, courseID, versionID string, items []AcademyMediaAssignment) error {
	if len(items) > 2000 {
		return academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	owner, err := academyAuthor(ctx, tx, actor, courseID)
	if err != nil {
		return err
	}
	if !owner {
		return academy.ErrDenied
	}
	var state string
	err = tx.QueryRowContext(ctx, `SELECT state FROM academy_course_versions WHERE id=$1::uuid AND course_id=$2::uuid FOR UPDATE`, versionID, courseID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != "DRAFT" {
		return academy.ErrState
	}
	used := map[string]map[int]bool{}
	seenMedia := map[string]map[string]bool{}
	for _, item := range items {
		if item.Ordinal < 1 || item.Ordinal > 2000 || !academyMediaUUID(item.LessonID) || !academyMediaUUID(item.MediaID) {
			return academy.ErrInvalid
		}
		if used[item.LessonID] == nil {
			used[item.LessonID] = map[int]bool{}
		}
		if used[item.LessonID][item.Ordinal] {
			return academy.ErrConflict
		}
		used[item.LessonID][item.Ordinal] = true
		if seenMedia[item.LessonID] == nil {
			seenMedia[item.LessonID] = map[string]bool{}
		}
		if seenMedia[item.LessonID][item.MediaID] {
			return academy.ErrConflict
		}
		seenMedia[item.LessonID][item.MediaID] = true
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM academy_lessons l JOIN academy_modules m ON m.id=l.module_id
   JOIN academy_cloud_media media ON media.id=$3::uuid AND media.author_id=$4::uuid
   WHERE l.id=$1::uuid AND m.version_id=$2::uuid AND l.kind='VIDEO')`,
			item.LessonID, versionID, item.MediaID, actor).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return academy.ErrDenied
		}
	}
	// Exact retry after a lost HTTP response must not rewrite rows or emit a
	// duplicate academy.media.assigned event. The course version row lock
	// serializes concurrent retries from the same author.
	var current []AcademyMediaAssignment
	rows, queryErr := tx.QueryContext(ctx, `SELECT lm.lesson_id::text,lm.media_id::text,lm.ordinal
 FROM academy_lesson_media lm
 JOIN academy_lessons l ON l.id=lm.lesson_id
 JOIN academy_modules m ON m.id=l.module_id
 WHERE m.version_id=$1::uuid ORDER BY lm.lesson_id,lm.ordinal`, versionID)
	if queryErr != nil {
		return queryErr
	}
	for rows.Next() {
		var item AcademyMediaAssignment
		if scanErr := rows.Scan(&item.LessonID, &item.MediaID, &item.Ordinal); scanErr != nil {
			rows.Close()
			return scanErr
		}
		current = append(current, item)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		rows.Close()
		return rowsErr
	}
	rows.Close()
	equal := len(current) == len(items)
	if equal {
		lookup := make(map[string]AcademyMediaAssignment, len(current))
		for _, item := range current {
			lookup[item.LessonID+"/"+item.MediaID] = item
		}
		for _, item := range items {
			existing, ok := lookup[item.LessonID+"/"+item.MediaID]
			if !ok || existing.Ordinal != item.Ordinal {
				equal = false
				break
			}
		}
	}
	if equal {
		return tx.Commit()
	}
	// Replace the entire version snapshot under the existing version row
	// lock. Clearing the final assignment is a valid DRAFT edit, not a
	// separate authorization or publication path.
	if _, err = tx.ExecContext(ctx, `DELETE FROM academy_lesson_media lm
 USING academy_lessons l, academy_modules m
 WHERE lm.lesson_id=l.id AND l.module_id=m.id AND m.version_id=$1::uuid`, versionID); err != nil {
		return err
	}
	for _, item := range items {
		if _, err = tx.ExecContext(ctx, `INSERT INTO academy_lesson_media(lesson_id,media_id,ordinal)
  VALUES($1::uuid,$2::uuid,$3)`, item.LessonID, item.MediaID, item.Ordinal); err != nil {
			return err
		}
	}
	if err = academyEvent(ctx, tx, actor, "academy/version/"+versionID, "academy.media.assigned",
		map[string]any{"version_id": versionID, "count": len(items)}); err != nil {
		return err
	}
	return tx.Commit()
}

// AcademyImportCloudMedia inserts only opaque provider references. Public URLs
// are not returned by the learner API; author access is authenticated.
// Direct catalog imports (used by legacy/internal callers) are deliberately
// UNBOUND. Only the verified OAuth import below can create playable media.
func (c *Checker) AcademyImportCloudMedia(actor string, items []AcademyCloudMedia) ([]AcademyCloudMedia, error) {
	return c.academyImportCloudMediaWithGeneration(actor, items, "00000000-0000-0000-0000-000000000000")
}

func (c *Checker) academyImportCloudMediaWithGeneration(actor string, items []AcademyCloudMedia, generation string) ([]AcademyCloudMedia, error) {
	if !academyMediaUUID(generation) {
		return nil, academy.ErrInvalid
	}
	if !academyMediaUUID(actor) || len(items) == 0 || len(items) > 2000 {
		return nil, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// A reconnect between provider verification and persistence must not
	// attach the old provider's file to the new account's generation. The
	// row lock serializes the final comparison with concurrent OAuth writes.
	if generation != "00000000-0000-0000-0000-000000000000" {
		var current string
		err = tx.QueryRowContext(ctx, `SELECT generation::text FROM academy_cloud_connections
 WHERE author_id=$1::uuid AND provider='YANDEX_DISK' FOR SHARE`, actor).Scan(&current)
		if err != nil || current != generation {
			return nil, academy.ErrDenied
		}
	}
	output := make([]AcademyCloudMedia, 0, len(items))
	// The verification layer deduplicates provider calls, but without this
	// guard repeated rows still perform redundant database updates and return
	// the same media ID multiple times. Keep the first occurrence so the
	// batch result and title selection are stable across retries.
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		if it.Provider != "YANDEX_DISK" && it.Provider != "EXTERNAL" {
			return nil, academy.ErrInvalid
		}
		// A verified Yandex path must be persisted byte-for-byte. Trimming
		// it here would change the source after provider verification.
		if it.Provider == "YANDEX_DISK" {
			if !academy.ValidYandexDiskPath(it.ProviderFileRef) {
				return nil, academy.ErrInvalid
			}
		} else {
			it.ProviderFileRef = strings.TrimSpace(it.ProviderFileRef)
		}
		it.Title = strings.TrimSpace(it.Title)
		if len(it.ProviderFileRef) < 1 || len(it.ProviderFileRef) > 2048 || len(it.Title) < 1 || len(it.Title) > 300 || len(it.SourceURL) > 4096 {
			return nil, academy.ErrInvalid
		}
		if it.SourceURL != "" && !strings.HasPrefix(it.SourceURL, "https://") {
			return nil, academy.ErrInvalid
		}
		if generation != "00000000-0000-0000-0000-000000000000" &&
			(it.ResourceID == "" || len(it.ResourceID) > 512 || len(it.SHA256) != 64 || it.FileSize <= 0) {
			return nil, academy.ErrInvalid
		}
		key := it.Provider + "\x00" + it.ProviderFileRef
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		id, err := academyID()
		if err != nil {
			return nil, err
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO academy_cloud_media(id,author_id,provider,provider_file_ref,source_url,title,connection_generation,resource_id,sha256,file_size)
   VALUES($1::uuid,$2::uuid,$3,$4,NULLIF($5,''),$6,$7::uuid,NULLIF($8,''),NULLIF($9,''),NULLIF($10::bigint,0))
   ON CONFLICT(author_id,provider,provider_file_ref,connection_generation) DO UPDATE SET title=EXCLUDED.title
   WHERE academy_cloud_media.resource_id IS NOT DISTINCT FROM EXCLUDED.resource_id
     AND academy_cloud_media.sha256 IS NOT DISTINCT FROM EXCLUDED.sha256
     AND academy_cloud_media.file_size IS NOT DISTINCT FROM EXCLUDED.file_size
   RETURNING id::text`, id, actor, it.Provider, it.ProviderFileRef, it.SourceURL, it.Title, generation, it.ResourceID, it.SHA256, it.FileSize).Scan(&it.ID)
		if errors.Is(err, sql.ErrNoRows) {
			// The same path was overwritten within one OAuth connection.
			// Never relabel an already verified video as a different file.
			return nil, academy.ErrState
		}
		if err != nil {
			return nil, fmt.Errorf("import media: %w", err)
		}
		output = append(output, it)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return output, nil
}
func academyMediaUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
func (c *Checker) AcademyAuthorCloudMedia(actor string) ([]AcademyCloudMedia, error) {
	if !academyMediaUUID(actor) {
		return nil, academy.ErrDenied
	}
	if c == nil || c.db == nil {
		return nil, academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT media.id::text,media.provider,media.provider_file_ref,COALESCE(media.source_url,''),media.title,
 (connection.generation IS NOT NULL AND media.connection_generation=connection.generation
  AND media.resource_id IS NOT NULL AND media.sha256 IS NOT NULL AND media.file_size>0)
 FROM academy_cloud_media media
 LEFT JOIN academy_cloud_connections connection
 ON connection.author_id=media.author_id AND connection.provider=media.provider
 WHERE media.author_id=$1::uuid ORDER BY media.created_at,media.id LIMIT 5000`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AcademyCloudMedia{}
	for rows.Next() {
		var m AcademyCloudMedia
		if err = rows.Scan(&m.ID, &m.Provider, &m.ProviderFileRef, &m.SourceURL, &m.Title, &m.Active); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
