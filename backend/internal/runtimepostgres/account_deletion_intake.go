package runtimepostgres

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "time"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

const deletionIntakeNotice = "Ваш запрос зарегистрирован. Стирание данных ещё не выполнено и не подтверждено. Сроки хранения и необходимые проверки будут определены отдельно."

// SubmitAccountDeletionRequest creates durable, owner-scoped evidence only.
// It never declares that personal data or external provider data were erased.
func (c *Checker) SubmitAccountDeletionRequest(ctx context.Context, identity, source string) (privacy.AccountDeletionReceipt, error) {
 var out privacy.AccountDeletionReceipt
 if c == nil || c.db == nil { return out, errors.New("postgres unavailable") }
 if source != "WEB" && source != "IOS" && source != "ANDROID" { return out, errors.New("invalid deletion source") }
 ctx, cancel := context.WithTimeout(ctx, clientPersonalTimeout)
 defer cancel()
 tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation:sql.LevelSerializable})
 if err != nil { return out, err }
 defer tx.Rollback()
 // Serialize submissions for the same owner even without a unique index on
 // legacy requests. This also handles records created by older deployments.
 if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", identity); err != nil { return out, err }
 var exists bool
 if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM identities WHERE id=$1::uuid)",identity).Scan(&exists); err != nil { return out, err }
 if !exists { return out, errors.New("identity not found") }
 err = tx.QueryRowContext(ctx,`SELECT id::text,state,source_surface,created_at,updated_at
 FROM delete_account_requests WHERE identity_id=$1::uuid ORDER BY created_at DESC,id DESC LIMIT 1`,identity).
 Scan(&out.ID,&out.State,&out.Source,&out.CreatedAt,&out.UpdatedAt)
 switch {
 case err == nil:
  out.Idempotent = true
 case errors.Is(err,sql.ErrNoRows):
  out.ID,err = persistentid.New()
  if err != nil { return privacy.AccountDeletionReceipt{},err }
  err=tx.QueryRowContext(ctx,`INSERT INTO delete_account_requests
   (id,identity_id,source_surface,state,created_at,updated_at)
   VALUES($1::uuid,$2::uuid,$3,'REQUESTED',now(),now())
   RETURNING id::text,state,source_surface,created_at,updated_at`,
   out.ID,identity,source).Scan(&out.ID,&out.State,&out.Source,&out.CreatedAt,&out.UpdatedAt)
  if err != nil { return privacy.AccountDeletionReceipt{},fmt.Errorf("create deletion request: %w",err) }
 default:
  return privacy.AccountDeletionReceipt{},fmt.Errorf("find deletion request: %w",err)
 }
 if err=tx.Commit(); err != nil { return privacy.AccountDeletionReceipt{},err }
 out.Notice=deletionIntakeNotice
 return out,nil
}

func (c *Checker) ReadAccountDeletionRequest(ctx context.Context, identity string) (privacy.AccountDeletionReceipt,error) {
 var out privacy.AccountDeletionReceipt
 if c == nil || c.db == nil { return out,errors.New("postgres unavailable") }
 ctx,cancel:=context.WithTimeout(ctx,clientPersonalTimeout)
 defer cancel()
 err:=c.db.QueryRowContext(ctx,`SELECT id::text,state,source_surface,created_at,updated_at
 FROM delete_account_requests WHERE identity_id=$1::uuid ORDER BY created_at DESC,id DESC LIMIT 1`,identity).
 Scan(&out.ID,&out.State,&out.Source,&out.CreatedAt,&out.UpdatedAt)
 if err != nil { return out,err }
 out.Notice=deletionIntakeNotice
 return out,nil
}
