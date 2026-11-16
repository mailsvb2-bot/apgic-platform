package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyRecordDirectVideoDelivery records only a fully transmitted,
// fingerprint-verified HTTP representation. It is NOT an attestation that a
// person watched the screen. A learner must separately acknowledge studying
// all assigned video materials before AcademyMarkProgress accepts completion.
//
// The canonical database trigger revalidates the active enrollment,
// entitlement, published version, author assignment and OAuth generation
// under locks. This prevents a refund racing a completed HTTP response from
// creating an authorized delivery fact.
func (c *Checker) AcademyRecordDirectVideoDelivery(
	actor, enrollmentID, lessonID, mediaID, contentSHA string,
	contentLength, startByte, endByte int64,
) error {
	if c == nil || c.db == nil {
		return academy.ErrUnavailable
	}
	for _, id := range []string{actor, enrollmentID, lessonID, mediaID} {
		if !academyMediaUUID(id) {
			return academy.ErrInvalid
		}
	}
	if len(contentSHA) != 64 || contentLength <= 0 ||
		startByte < 0 || endByte <= startByte || endByte > contentLength {
		return academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id, err := academyID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO academy_direct_video_deliveries (
      id,enrollment_id,lesson_id,media_id,subject_id,
      content_sha256,content_length,start_byte,end_byte
    ) VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,$7,$8,$9)
    ON CONFLICT DO NOTHING`,
		id, enrollmentID, lessonID, mediaID, actor,
		contentSHA, contentLength, startByte, endByte)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// academyDirectVideoDeliveryCompleteTx reads only the existing Academy
// evidence owner. No browser-provided playback duration, seek position, or
// ended flag can authorize a VIDEO completion.
func academyDirectVideoDeliveryCompleteTx(
	ctx context.Context, tx *sql.Tx, enrollmentID, lessonID string,
) (bool, error) {
	var complete bool
	err := tx.QueryRowContext(ctx,
		`SELECT academy_direct_video_delivery_complete($1::uuid,$2::uuid)`,
		enrollmentID, lessonID).Scan(&complete)
	if errors.Is(err, sql.ErrNoRows) {
		return false, academy.ErrNotFound
	}
	return complete, err
}
