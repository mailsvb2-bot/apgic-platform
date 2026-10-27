package privacy

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

var ErrProviderErasureReceiptInvalid = errors.New("provider erasure requires independently signed and verified evidence")

type ProviderErasureStatement struct {
	RequestID string `json:"request_id"`
	ProviderRef string `json:"provider_ref"`
	EvidenceRef string `json:"evidence_ref"`
	State string `json:"state"`
	ErasedAt time.Time `json:"erased_at"`
}
type VerifiedProviderErasureReceipt struct {
	raw []byte
	signature []byte
	keyFingerprint string
	statement ProviderErasureStatement
}
func (v VerifiedProviderErasureReceipt) Statement() ProviderErasureStatement { return v.statement }
func (v VerifiedProviderErasureReceipt) RawJSON() []byte { return append([]byte(nil),v.raw...) }
func (v VerifiedProviderErasureReceipt) Signature() []byte { return append([]byte(nil),v.signature...) }
func (v VerifiedProviderErasureReceipt) KeyFingerprint() string { return v.keyFingerprint }

func VerifyProviderErasureReceipt(raw,signature []byte,key ed25519.PublicKey,expectedProviderRef string,now time.Time) (VerifiedProviderErasureReceipt,error) {
	var v VerifiedProviderErasureReceipt
	if len(key)!=ed25519.PublicKeySize||len(signature)!=ed25519.SignatureSize||
		!ed25519.Verify(key,raw,signature) {
		return v,ErrProviderErasureReceiptInvalid
	}
	d:=json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err:=d.Decode(&v.statement);err!=nil{return VerifiedProviderErasureReceipt{},ErrProviderErasureReceiptInvalid}
	var extra any
	if err:=d.Decode(&extra);err!=io.EOF{return VerifiedProviderErasureReceipt{},ErrProviderErasureReceiptInvalid}
	p:=v.statement
	if expectedProviderRef==""||p.ProviderRef!=expectedProviderRef||p.State!="ERASED"||
		strings.TrimSpace(p.RequestID)==""||strings.TrimSpace(p.EvidenceRef)==""||
		p.ErasedAt.IsZero()||p.ErasedAt.After(now.Add(time.Minute))||
		now.Sub(p.ErasedAt)>30*24*time.Hour {
		return VerifiedProviderErasureReceipt{},ErrProviderErasureReceiptInvalid
	}
	// Synthetic test markers are never a substitute for an actual provider signature.
	if strings.HasPrefix(p.EvidenceRef,"provider-erasure:")||strings.HasPrefix(p.EvidenceRef,"synthetic:") {
		return VerifiedProviderErasureReceipt{},ErrProviderErasureReceiptInvalid
	}
	digest:=sha256.Sum256(key)
	v.raw=append([]byte(nil),raw...)
	v.signature=append([]byte(nil),signature...)
	v.keyFingerprint=hex.EncodeToString(digest[:])
	return v,nil
}
