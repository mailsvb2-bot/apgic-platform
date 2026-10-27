package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

const deletionIntakeNotice = "Ваш запрос зарегистрирован. Стирание данных ещё не выполнено и не подтверждено."

func deletionNoticeFor(state, review string, pending int) string {
	switch state {
	case "REQUESTED":
		return deletionIntakeNotice
	case "IDENTITY_RECONFIRMED":
		if review != "" {
			return "Личность подтверждена. Выполнение ожидает проверки незавершённых обязательств; данные ещё не стёрты."
		}
		return "Личность подтверждена. Ожидается утверждённая политика хранения; данные ещё не стёрты."
	case "RETENTION_CLASSIFIED":
		return "Правила хранения определены. Ожидается фактическое исполнение и проверка удаления."
	case "PROVIDER_ERASURE_PENDING":
		if pending > 0 {
			return "Локальная обработка данных проведена. Ожидаются доказательства удаления от внешних систем."
		}
		return "Локальная обработка подтверждена. Выполняется финальная проверка и отзыв доступа."
	case "WAITING_FOR_LEGAL_HOLD_EXPIRY":
		return "Удаление ожидает истечения юридического ограничения на хранение."
	case "PARTIALLY_RETAINED_WITH_REASON":
		return "Обработка завершена и доступ отозван. Обязательные записи сохранены по указанному основанию."
	case "COMPLETED":
		return "Удаление завершено и подтверждено независимыми доказательствами."
	default:
		return "Статус обработки недоступен. Повторите проверку."
	}
}


// SubmitAccountDeletionRequest creates durable, owner-scoped evidence only.
// It never declares that personal data or external provider data were erased.
func (c *Checker) SubmitAccountDeletionRequest(ctx context.Context, identity, source string) (privacy.AccountDeletionReceipt, error) {
	var out privacy.AccountDeletionReceipt
	if c == nil || c.db == nil {
		return out, errors.New("postgres unavailable")
	}
	if source != "WEB" && source != "IOS" && source != "ANDROID" {
		return out, errors.New("invalid deletion source")
	}
	ctx, cancel := context.WithTimeout(ctx, clientPersonalTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()

	// Serialize submissions for the same owner, including older requests.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", identity); err != nil {
		return out, err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM identities WHERE id=$1::uuid)", identity).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, errors.New("identity not found")
	}
	err = tx.QueryRowContext(ctx, `SELECT id::text,state,source_surface,created_at,updated_at
		FROM delete_account_requests WHERE identity_id=$1::uuid ORDER BY created_at DESC,id DESC LIMIT 1`, identity).
		Scan(&out.ID, &out.State, &out.Source, &out.CreatedAt, &out.UpdatedAt)
	switch {
	case err == nil:
		out.Idempotent = true
	case errors.Is(err, sql.ErrNoRows):
		out.ID, err = persistentid.New()
		if err != nil {
			return privacy.AccountDeletionReceipt{}, err
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO delete_account_requests
			(id,identity_id,source_surface,state,created_at,updated_at)
			VALUES($1::uuid,$2::uuid,$3,'REQUESTED',now(),now())
			RETURNING id::text,state,source_surface,created_at,updated_at`,
			out.ID, identity, source).Scan(&out.ID, &out.State, &out.Source, &out.CreatedAt, &out.UpdatedAt)
		if err != nil {
			return privacy.AccountDeletionReceipt{}, fmt.Errorf("create deletion request: %w", err)
		}
	default:
		return privacy.AccountDeletionReceipt{}, fmt.Errorf("find deletion request: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return privacy.AccountDeletionReceipt{}, err
	}
	confirmed, err := c.ReadAccountDeletionRequest(ctx, identity)
	if err != nil {
		return privacy.AccountDeletionReceipt{}, err
	}
	confirmed.Idempotent = out.Idempotent
	return confirmed, nil
}

// ReadAccountDeletionRequest returns the canonical request only to its signed owner.
func (c *Checker) ReadAccountDeletionRequest(ctx context.Context, identity string) (privacy.AccountDeletionReceipt, error) {
	var out privacy.AccountDeletionReceipt
	if c == nil || c.db == nil {
		return out, errors.New("postgres unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, clientPersonalTimeout)
	defer cancel()
	var completed sql.NullTime
	err := c.db.QueryRowContext(ctx, `SELECT
		r.id::text,r.state,r.source_surface,r.created_at,r.updated_at,
		COALESCE(r.retained_reason,''),r.completed_at,
		(SELECT count(*) FROM provider_erasure_jobs p WHERE p.delete_request_id=r.id AND p.state='PENDING'),
		COALESCE((SELECT h.reason_code FROM deletion_processing_holds h WHERE h.request_id=r.id),'')
		FROM delete_account_requests r
		WHERE r.identity_id=$1::uuid ORDER BY r.created_at DESC,r.id DESC LIMIT 1`, identity).
		Scan(&out.ID, &out.State, &out.Source, &out.CreatedAt, &out.UpdatedAt,
			&out.RetainedReason, &completed, &out.PendingProviders, &out.ReviewCode)
	if err != nil {
		return out, err
	}
	if completed.Valid {
		out.CompletedAt = &completed.Time
	}
	out.Notice = deletionNoticeFor(out.State, out.ReviewCode, out.PendingProviders)
	return out, nil
}
