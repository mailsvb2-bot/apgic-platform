package runtimepostgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

var ErrDeletionBlockedByActiveObligations = errors.New("cannot erase account with active bookings or entitlements")
var ErrNoEligibleDeletionRequest = errors.New("no reconfirmed account deletion request")

// ExecuteApprovedAccountDeletion processes one durable REQUESTED->reconfirmed
// owner. The only accepted authority is the opaque, signature-verified legal
// policy. It never fabricates provider receipts.
func (c *Checker) ExecuteApprovedAccountDeletion(ctx context.Context, approved privacy.VerifiedDeletionPolicy) (privacy.AccountDeletionReceipt,error) {
	var out privacy.AccountDeletionReceipt
	if c==nil||c.db==nil||len(approved.Signature())!=64||len(approved.Digest())!=64 {
		return out,privacy.ErrDeletionPolicyUnapproved
	}
	p:=approved.Policy()
	if !p.ProductionApproved || !p.ExpiresAt.After(time.Now().UTC()) {
		return out,privacy.ErrDeletionPolicyUnapproved
	}
	ctx,cancel:=context.WithTimeout(ctx,30*time.Second)
	defer cancel()
	tx,err:=c.db.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelReadCommitted})
	if err!=nil{return out,err}
	defer tx.Rollback()
	var owner string
	// Acquire locks in the same order as signed intake and reconfirmation:
	// identity advisory lock first, canonical request row lock second.
	err=tx.QueryRowContext(ctx,`SELECT id::text,identity_id::text
		FROM delete_account_requests WHERE state='IDENTITY_RECONFIRMED'
		ORDER BY created_at,id LIMIT 1`).Scan(&out.ID,&owner)
	if errors.Is(err,sql.ErrNoRows){return out,ErrNoEligibleDeletionRequest}
	if err!=nil{return out,err}
	if _,err=tx.ExecContext(ctx,"SELECT pg_advisory_xact_lock(hashtextextended($1, 0))",owner);err!=nil{return out,err}
	err=tx.QueryRowContext(ctx,`SELECT state,source_surface,created_at,updated_at
		FROM delete_account_requests WHERE id=$1::uuid FOR UPDATE`,out.ID).
		Scan(&out.State,&out.Source,&out.CreatedAt,&out.UpdatedAt)
	if err!=nil{return out,err}
	if out.State!="IDENTITY_RECONFIRMED"{return out,ErrNoEligibleDeletionRequest}

	var active bool
	if err=tx.QueryRowContext(ctx,`SELECT
		EXISTS(SELECT 1 FROM bookings WHERE client_identity_id=$1::uuid
			AND state IN ('HELD','PENDING_PAYMENT','CONFIRMED'))
		OR EXISTS(SELECT 1 FROM store_entitlements WHERE identity_id=$1::uuid AND state='ACTIVE')`,owner).
		Scan(&active);err!=nil{return out,fmt.Errorf("check outstanding customer obligations: %w",err)}
	if active{return out,ErrDeletionBlockedByActiveObligations}

	_,err=tx.ExecContext(ctx,`INSERT INTO client_deletion_policy_approvals
		(request_id,policy_digest,policy_snapshot,approval_signature,signing_key_fingerprint,verified_at,approved_by)
		VALUES($1::uuid,$2,$3::jsonb,$4,$5,now(),$6)`,
		out.ID,approved.Digest(),string(approved.RawJSON()),approved.Signature(),approved.KeyFingerprint(),p.ApprovedBy)
	if err!=nil{return out,fmt.Errorf("record approved retention policy: %w",err)}

	err=tx.QueryRowContext(ctx,`UPDATE delete_account_requests
		SET state='RETENTION_CLASSIFIED',retention_snapshot=$2::jsonb,updated_at=now()
		WHERE id=$1::uuid AND state='IDENTITY_RECONFIRMED'
		RETURNING state,updated_at`,out.ID,string(approved.RawJSON())).
		Scan(&out.State,&out.UpdatedAt)
	if err!=nil{return out,fmt.Errorf("classify retention: %w",err)}

	// Mutations are performed in the same transaction as the receipts.
	// Immutable consent, consultation and financial evidence remain untouched,
	// with explicit signed retention reasons in the snapshot.
	classes:=[]struct{name,query string}{
		{"PERSONAL_PREFERENCES",`DELETE FROM client_personal_preferences WHERE identity_id=$1::uuid`},
		{"HELP_INTENT_PERSONAL_DATA",`UPDATE help_intents SET identity_id=NULL,
			free_text='[redacted following verified erasure request]',
			topics=ARRAY[]::text[],goals=ARRAY[]::text[],context='{}'::jsonb,
			updated_at=now() WHERE identity_id=$1::uuid`},
		{"PUSH_ENDPOINTS",`UPDATE client_installations SET state='REVOKED',push_endpoint=NULL,
			push_generation=push_generation+1,updated_at=now()
			WHERE identity_id=$1::uuid AND (state<>'REVOKED' OR push_endpoint IS NOT NULL)`},
	}
	for _,target:=range classes{
		res,e:=tx.ExecContext(ctx,target.query,owner)
		if e!=nil{return out,fmt.Errorf("erase %s: %w",target.name,e)}
		rows,e:=res.RowsAffected()
		if e!=nil{return out,e}
		_,e=tx.ExecContext(ctx,`INSERT INTO client_deletion_local_erasure_receipts
			(request_id,data_class,affected_rows,policy_digest,executed_at)
			VALUES($1::uuid,$2,$3,$4,now())`,out.ID,target.name,rows,approved.Digest())
		if e!=nil{return out,fmt.Errorf("record erasure evidence: %w",e)}
	}

	// Existing cookies must not be used for further customer-data access.
	_,err=tx.ExecContext(ctx,`INSERT INTO client_deletion_revocations(identity_id,request_id,revoked_at)
		VALUES($1::uuid,$2::uuid,now())`,owner,out.ID)
	if err!=nil{return out,fmt.Errorf("revoke client access: %w",err)}

	for _,provider:=range p.ProviderRefs{
		jobID,e:=persistentid.New()
		if e!=nil{return out,e}
		_,e=tx.ExecContext(ctx,`INSERT INTO provider_erasure_jobs
			(id,delete_request_id,provider_ref,state,updated_at)
			VALUES($1::uuid,$2::uuid,$3,'PENDING',now())`,jobID,out.ID,provider)
		if e!=nil{return out,fmt.Errorf("queue provider erasure %s: %w",provider,e)}
	}

	err=tx.QueryRowContext(ctx,`UPDATE delete_account_requests SET
		state='PROVIDER_ERASURE_PENDING',updated_at=now()
		WHERE id=$1::uuid AND state='RETENTION_CLASSIFIED'
		RETURNING state,updated_at`,out.ID).Scan(&out.State,&out.UpdatedAt)
	if err!=nil{return out,fmt.Errorf("start erasure verification: %w",err)}
	// No external providers is a signed assertion with an explicit inventory
	// evidence reference; zero jobs must not mean "forgot to inventory".
	if len(p.ProviderRefs)==0{
		err=tx.QueryRowContext(ctx,`UPDATE delete_account_requests SET
			state='PARTIALLY_RETAINED_WITH_REASON',
			retained_reason=$2,completed_at=now(),updated_at=now()
			WHERE id=$1::uuid AND state='PROVIDER_ERASURE_PENDING'
			RETURNING state,updated_at`,out.ID,p.FinancialRetentionReason).
			Scan(&out.State,&out.UpdatedAt)
		if err!=nil{return out,fmt.Errorf("finish verified local erasure: %w",err)}
	}
	if err=tx.Commit();err!=nil{return privacy.AccountDeletionReceipt{},err}
	out.Notice="Локальная обработка персональных данных выполнена. Сведения, требуемые законом, сохранены с указанием основания."
	if out.State=="PROVIDER_ERASURE_PENDING"{
		out.Notice="Локальные данные обработаны; удаление у внешних владельцев ещё ожидает независимого подтверждения."
	}
	return out,nil
}

// MarkVerifiedProviderErasure only accepts the opaque capability returned by
// ed25519 verification against the pinned provider public key. A caller cannot
// submit a fabricated string and declare external erasure complete.
func (c *Checker) MarkVerifiedProviderErasure(ctx context.Context, verified privacy.VerifiedProviderErasureReceipt) (privacy.AccountDeletionReceipt,error) {
	var out privacy.AccountDeletionReceipt
	receipt:=verified.Statement()
	if c==nil||c.db==nil||len(verified.Signature())!=64||receipt.RequestID==""||
		receipt.ProviderRef==""||receipt.EvidenceRef==""||receipt.State!="ERASED" {
		return out,privacy.ErrProviderErasureReceiptInvalid
	}
	ctx,cancel:=context.WithTimeout(ctx,clientPersonalTimeout)
	defer cancel()
	tx,err:=c.db.BeginTx(ctx,nil)
	if err!=nil{return out,err}
	defer tx.Rollback()
	var policyJSON []byte
	err=tx.QueryRowContext(ctx,`SELECT d.id::text,d.state,d.source_surface,d.created_at,d.updated_at,
		a.policy_snapshot FROM delete_account_requests d
		JOIN client_deletion_policy_approvals a ON a.request_id=d.id
		WHERE d.id=$1::uuid FOR UPDATE OF d`,receipt.RequestID).
		Scan(&out.ID,&out.State,&out.Source,&out.CreatedAt,&out.UpdatedAt,&policyJSON)
	if err!=nil{return out,err}
	var existingSignature []byte
	err=tx.QueryRowContext(ctx,`SELECT receipt_signature FROM client_deletion_provider_receipts
		WHERE request_id=$1::uuid AND provider_ref=$2`,receipt.RequestID,receipt.ProviderRef).
		Scan(&existingSignature)
	if err==nil{
		if !bytes.Equal(existingSignature,verified.Signature()){
			return out,errors.New("conflicting provider receipt replay")
		}
		out.Idempotent=true
	} else if !errors.Is(err,sql.ErrNoRows) {
		return out,err
	} else {
		if out.State!="PROVIDER_ERASURE_PENDING"{return out,errors.New("deletion not awaiting provider receipt")}
		res,e:=tx.ExecContext(ctx,`UPDATE provider_erasure_jobs
			SET state='SUCCEEDED',evidence_ref=$3,updated_at=now()
			WHERE delete_request_id=$1::uuid AND provider_ref=$2 AND state='PENDING'`,
			receipt.RequestID,receipt.ProviderRef,receipt.EvidenceRef)
		if e!=nil{return out,e}
		n,e:=res.RowsAffected()
		if e!=nil{return out,e}
		if n!=1{return out,errors.New("provider erasure job missing or already completed")}
		_,e=tx.ExecContext(ctx,`INSERT INTO client_deletion_provider_receipts
			(request_id,provider_ref,receipt_json,receipt_signature,signing_key_fingerprint,verified_at)
			VALUES($1::uuid,$2,$3::jsonb,$4,$5,now())`,
			receipt.RequestID,receipt.ProviderRef,string(verified.RawJSON()),
			verified.Signature(),verified.KeyFingerprint())
		if e!=nil{return out,e}
	}
	if out.State=="PROVIDER_ERASURE_PENDING" {
		var pending bool
		if err=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM provider_erasure_jobs
			WHERE delete_request_id=$1::uuid AND state<>'SUCCEEDED')`,receipt.RequestID).Scan(&pending);err!=nil{return out,err}
		if !pending {
			var policy privacy.ApprovedDeletionPolicy
			if err=json.Unmarshal(policyJSON,&policy);err!=nil{return out,err}
			err=tx.QueryRowContext(ctx,`UPDATE delete_account_requests SET
				state='PARTIALLY_RETAINED_WITH_REASON',
				retained_reason=$2,completed_at=now(),updated_at=now()
				WHERE id=$1::uuid AND state='PROVIDER_ERASURE_PENDING'
				RETURNING state,updated_at`,receipt.RequestID,policy.FinancialRetentionReason).
				Scan(&out.State,&out.UpdatedAt)
			if err!=nil{return out,err}
		}
	}
	if err=tx.Commit();err!=nil{return privacy.AccountDeletionReceipt{},err}
	out.Notice="Результат удаления сохранён; обязательные к хранению сведения не стирались."
	return out,nil
}
