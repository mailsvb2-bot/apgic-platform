package runtimepostgres

import (
	"database/sql"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademySetCourseOffer configures the SAME Commerce product owned by Academy.
// It does not create an order, a payment, or a learner entitlement.
func (c *Checker) AcademySetCourseOffer(actor, courseID, mode string, priceMinor int64) (academy.CourseOffer, error) {
	if !academyMediaUUID(actor) || !academyMediaUUID(courseID) ||
		(mode != "FREE" && mode != "PAID") ||
		(mode == "FREE" && priceMinor != 0) ||
		(mode == "PAID" && (priceMinor <= 0 || priceMinor > 10000000000)) {
		return academy.CourseOffer{}, academy.ErrInvalid
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return academy.CourseOffer{}, err
	}
	defer tx.Rollback()
	owned, err := academyAuthor(ctx, tx, actor, courseID)
	if err != nil {
		return academy.CourseOffer{}, err
	}
	if !owned {
		return academy.CourseOffer{}, academy.ErrDenied
	}
	var productID string
	err = tx.QueryRowContext(ctx, `SELECT product_id::text FROM academy_courses
 WHERE id=$1::uuid`, courseID).Scan(&productID)
	if err != nil {
		return academy.CourseOffer{}, err
	}
	allowed, err := academyOwnedProduct(ctx, tx, actor, productID)
	if err != nil {
		return academy.CourseOffer{}, err
	}
	if !allowed {
		return academy.CourseOffer{}, academy.ErrDenied
	}
	var currentMode string
	var currentPrice, revision int64
	err = tx.QueryRowContext(ctx, `SELECT access_mode,price_minor,revision
 FROM commerce_product_access_terms WHERE product_id=$1::uuid FOR UPDATE`, productID).
		Scan(&currentMode, &currentPrice, &revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return academy.CourseOffer{}, err
	}
	if currentMode != mode || currentPrice != priceMinor {
		err = tx.QueryRowContext(ctx, `INSERT INTO commerce_product_access_terms(
 product_id,access_mode,price_minor,currency,changed_by)
 VALUES($1::uuid,$2,$3,'RUB',$4::uuid)
 ON CONFLICT(product_id) DO UPDATE SET access_mode=EXCLUDED.access_mode,
 price_minor=EXCLUDED.price_minor,changed_by=EXCLUDED.changed_by,
 changed_at=now(),revision=commerce_product_access_terms.revision+1
 RETURNING revision`, productID, mode, priceMinor, actor).Scan(&revision)
		if err != nil {
			return academy.CourseOffer{}, err
		}
		if err = academyEvent(ctx, tx, actor, "academy/course/"+courseID,
			"academy.offer.updated", map[string]any{
				"product_id": productID, "access_mode": mode,
				"price_minor": priceMinor, "currency": "RUB",
				"revision": revision,
			}); err != nil {
			return academy.CourseOffer{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return academy.CourseOffer{}, err
	}
	return academy.CourseOffer{
		CourseID: courseID, AccessMode: mode, PriceMinor: priceMinor,
		Currency: "RUB", Revision: revision,
	}, nil
}
