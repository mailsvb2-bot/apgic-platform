package runtimepostgres

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientpersonal"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

func testSignedDeletionPolicy(t *testing.T,providers []string) privacy.VerifiedDeletionPolicy {
	t.Helper()
	now:=time.Now().UTC()
	pub,private,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	p:=privacy.ApprovedDeletionPolicy{
		Version:"test-signed-approval",Jurisdiction:"CI_ONLY",
		ApprovedBy:"test-independent-legal-review",ApprovedAt:now.Add(-time.Minute),
		ExpiresAt:now.Add(time.Hour),ProductionApproved:true,
		DataClasses:map[string]string{
			"PERSONAL_PREFERENCES":"ERASE",
			"HELP_INTENT_PERSONAL_DATA":"ANONYMIZE",
			"PUSH_ENDPOINTS":"ERASE",
			"FINANCIAL_EVIDENCE":"RETAIN_WITH_REASON",
			"CONSULTATION_EVIDENCE":"RETAIN_WITH_REASON",
			"CONSENT_EVIDENCE":"RETAIN_WITH_REASON",
		},
		FinancialRetentionReason:"test financial evidence retention",
		ConsultationRetentionReason:"test consultation evidence retention",
		ConsentRetentionReason:"test consent evidence retention",
		ProviderRefs:providers,ProviderInventoryEvidence:"test complete provider inventory",
	}
	raw,err:=json.Marshal(p)
	if err!=nil{t.Fatal(err)}
	result,err:=privacy.VerifyDeletionPolicy(raw,ed25519.Sign(private,raw),pub,now)
	if err!=nil{t.Fatal(err)}
	return result
}

func TestR5ActualErasureRevocationAndSignedProviderCompletion(t *testing.T) {
	url:=os.Getenv("APGIC_PERSONAL_TEST_DATABASE_URL")
	if url==""{t.Skip("actual PostgreSQL integration URL unavailable")}
	ctx:=context.Background()
	store,err:=Open(ctx,url)
	if err!=nil{t.Fatal(err)}
	defer store.Close()
	for _,providers:=range [][]string{{},{"object-storage"}}{
		owner,err:=persistentid.New()
		if err!=nil{t.Fatal(err)}
		intent,err:=persistentid.New()
		if err!=nil{t.Fatal(err)}
		installation,err:=persistentid.New()
		if err!=nil{t.Fatal(err)}
		if _,err=store.db.ExecContext(ctx,"INSERT INTO identities(id) VALUES($1::uuid)",owner);err!=nil{t.Fatal(err)}
		if _,err=store.db.ExecContext(ctx,`INSERT INTO help_intents(id,identity_id,free_text)
			VALUES($1::uuid,$2::uuid,'Private user text requiring erasure')`,intent,owner);err!=nil{t.Fatal(err)}
		if _,err=store.UpdateClientPersonalPreferences(ctx,owner,clientpersonal.Preferences{
			DisplayName:"DELETE ME",Locale:"ru",TimeZone:"UTC",
		});err!=nil{t.Fatal(err)}
		if _,err=store.db.ExecContext(ctx,`INSERT INTO client_installations
			(id,identity_id,platform,push_endpoint,state,updated_at)
			VALUES($1::uuid,$2::uuid,'IOS',$3,'ACTIVE',now())`,installation,owner,"https://push.example.invalid/"+installation);err!=nil{t.Fatal(err)}
		req,err:=store.SubmitAccountDeletionRequest(ctx,owner,"WEB")
		if err!=nil{t.Fatal(err)}
		if _,err=store.ReconfirmAccountDeletion(ctx,owner,"session:step-up-confirmed","WEBAUTHN",time.Now().UTC());err!=nil{t.Fatal(err)}
		policy:=testSignedDeletionPolicy(t,providers)
		processed,err:=store.ExecuteApprovedAccountDeletionRequest(ctx,req.ID,policy)
		if err!=nil{t.Fatalf("execute signed erasure: %v",err)}
		expected:="PARTIALLY_RETAINED_WITH_REASON"
		if len(providers)>0{expected="PROVIDER_ERASURE_PENDING"}
		if processed.State!=expected{t.Fatalf("expected %s got %#v",expected,processed)}
		var count int
		for _,query:=range []string{
			"SELECT count(*) FROM client_personal_preferences WHERE identity_id=$1::uuid",
			"SELECT count(*) FROM client_installations WHERE identity_id=$1::uuid AND push_endpoint IS NOT NULL",
			"SELECT count(*) FROM help_intents WHERE id=$1::uuid AND identity_id IS NOT NULL",
		}{
			id:=owner
			if query==("SELECT count(*) FROM help_intents WHERE id=$1::uuid AND identity_id IS NOT NULL"){id=intent}
			if err=store.db.QueryRowContext(ctx,query,id).Scan(&count);err!=nil||count!=0{
				t.Fatalf("erasure incomplete query=%s count=%d err=%v",query,count,err)
			}
		}
		var scrubbed string
		if err=store.db.QueryRowContext(ctx,"SELECT free_text FROM help_intents WHERE id=$1::uuid",intent).Scan(&scrubbed);err!=nil||scrubbed=="Private user text requiring erasure"{
			t.Fatalf("raw request survived erasure: %v",err)
		}
		revoked,err:=store.IsAccountAccessRevoked(ctx,owner)
		if err!=nil||!revoked{t.Fatalf("canonical session revocation absent: %t %v",revoked,err)}
		if _,err=store.UpdateClientPersonalPreferences(ctx,owner,clientpersonal.Preferences{
			DisplayName:"resurrection",Locale:"ru",TimeZone:"UTC",
		});err==nil{t.Fatal("stale account resurrected personal preferences")}
		if err=store.db.QueryRowContext(ctx,`SELECT count(*) FROM client_deletion_local_erasure_receipts
			WHERE request_id=$1::uuid`,req.ID).Scan(&count);err!=nil||count!=3{
			t.Fatalf("missing durable erasure receipts: count=%d err=%v",count,err)
		}
		if len(providers)>0{
			pub,private,e:=ed25519.GenerateKey(rand.Reader)
			if e!=nil{t.Fatal(e)}
			statement:=privacy.ProviderErasureStatement{
				RequestID:req.ID,ProviderRef:providers[0],EvidenceRef:"provider-authenticated-eraser-1",
				State:"ERASED",ErasedAt:time.Now().UTC(),
			}
			raw,e:=json.Marshal(statement)
			if e!=nil{t.Fatal(e)}
			proof,e:=privacy.VerifyProviderErasureReceipt(raw,ed25519.Sign(private,raw),pub,providers[0],time.Now().UTC())
			if e!=nil{t.Fatal(e)}
			finished,e:=store.MarkVerifiedProviderErasure(ctx,proof)
			if e!=nil||finished.State!="PARTIALLY_RETAINED_WITH_REASON"{
				t.Fatalf("verified provider completion failed: %#v %v",finished,e)
			}
			replay,e:=store.MarkVerifiedProviderErasure(ctx,proof)
			if e!=nil||!replay.Idempotent{t.Fatalf("signed receipt replay unsafe: %#v %v",replay,e)}
		}
	}
}
