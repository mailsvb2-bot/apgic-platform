package runtimepostgres

import (
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func TestAcademyBankSellerSecretRequiresApprovedImmutablePrivateFile(t *testing.T) {
 dir:=t.TempDir()
 target:=filepath.Join(dir,"seller.json")
 body:=[]byte(`{"terminal_key":"merchant-one","terminal_password":"test-only-private-secret","seller_ref":"identity/seller","commercial_owner_ref":"identity/seller","payment_recipient_ref":"identity/seller","platform_role":"DIRECT_SELLER","fiscal_responsibility_ref":"identity/seller","refund_responsibility_ref":"identity/seller","payout_beneficiary_ref":"identity/seller","legal_policy_version":"v1","pricing_policy_version":"v1","commission_policy_version":"v1","routing_policy_version":"v1","commission_minor":0,"fiscal_taxation":"usn_income","receipt_tax":"none","receipt_payment_object":"service","receipt_email":"seller@example.test","description":"Course test"}`)
 if err:=os.WriteFile(target,body,0600);err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(body)
 expected:=hex.EncodeToString(digest[:])
 cfg,err:=academyLoadApprovedBankSellerSecretAtPath(target,"merchant-one",expected)
 if err!=nil||cfg.TerminalKey!="merchant-one"||cfg.ReceiptTax!="none"{
  t.Fatalf("approved seller profile missing: %v",err)
 }
 if _,err=academyLoadApprovedBankSellerSecretAtPath(target,"merchant-other",expected);!errors.Is(err,academy.ErrDenied){
  t.Fatalf("cross-seller secret accepted: %v",err)
 }
 if _,err=academyLoadApprovedBankSellerSecretAtPath(target,"merchant-one",strings.Repeat("0",64));!errors.Is(err,academy.ErrDenied){
  t.Fatalf("changed operator policy accepted without reapproval: %v",err)
 }
 if err:=os.Chmod(target,0644);err!=nil{t.Fatal(err)}
 if _,err=academyLoadApprovedBankSellerSecretAtPath(target,"merchant-one",expected);!errors.Is(err,academy.ErrUnavailable){
  t.Fatalf("world-readable terminal secret accepted: %v",err)
 }
 if err:=os.Chmod(target,0600);err!=nil{t.Fatal(err)}
 link:=filepath.Join(dir,"alias.json")
 if err:=os.Symlink(target,link);err!=nil{t.Fatal(err)}
 if _,err=academyLoadApprovedBankSellerSecretAtPath(link,"merchant-one",expected);!errors.Is(err,academy.ErrUnavailable){
  t.Fatalf("symlinked merchant secret accepted: %v",err)
 }
 tampered:=append([]byte(nil),body...)
 tampered=append(tampered,' ')
 if err:=os.WriteFile(target,tampered,0600);err!=nil{t.Fatal(err)}
 if _,err=academyLoadApprovedBankSellerSecretAtPath(target,"merchant-one",expected);!errors.Is(err,academy.ErrDenied){
  t.Fatalf("modified seller policy bytes accepted: %v",err)
 }
}
