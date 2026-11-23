package payments

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

var ErrTBankSignature = errors.New("tbank notification signature invalid")

// TBankToken signs scalar root fields only, excluding Token, Data, Receipt.
// The terminal password participates in the digest, never the outgoing body.
func TBankToken(fields map[string]any, password string) (string, error) {
	if password == "" || len(fields) == 0 || len(fields) > 128 {
		return "", ErrTBankSignature
	}
	keys := make([]string, 0, len(fields)+1)
	for key, v := range fields {
		if key == "Token" || key == "Password" || strings.EqualFold(key, "Data") || strings.EqualFold(key, "Receipt") {
			continue
		}
		if key == "" || len(key) > 100 || strings.TrimSpace(key) != key {
			return "", ErrTBankSignature
		}
		if v == nil {
			continue
		}
		switch v.(type) {
		case string, bool, json.Number, float64, float32, int, int64, int32, uint, uint64, uint32:
		default:
			return "", ErrTBankSignature
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return "", ErrTBankSignature
	}
	keys = append(keys, "Password")
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		if key == "Password" {
			b.WriteString(password)
			continue
		}
		raw, err := json.Marshal(fields[key])
		if err != nil {
			return "", ErrTBankSignature
		}
		var scalar any
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if decoder.Decode(&scalar) != nil {
			return "", ErrTBankSignature
		}
		switch n := scalar.(type) {
		case string:
			b.WriteString(n)
		case bool:
			if n {
				b.WriteString("true")
			} else {
				b.WriteString("false")
			}
		case json.Number:
			b.WriteString(string(n))
		default:
			return "", ErrTBankSignature
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}

// Verifies a bank notification token only. Never grants Entitlement.
// Canonical Commerce must verify order, attempt, capture, ledger, refund,
// terminal, amount and idempotency before any access is made available.
func VerifyTBankNotificationToken(fields map[string]any, password string) error {
	supplied, ok := fields["Token"].(string)
	if !ok || len(supplied) != 64 {
		return ErrTBankSignature
	}
	decoded, err := hex.DecodeString(supplied)
	if err != nil {
		return ErrTBankSignature
	}
	expected, err := TBankToken(fields, password)
	if err != nil {
		return ErrTBankSignature
	}
	digest, _ := hex.DecodeString(expected)
	if subtle.ConstantTimeCompare(decoded, digest) != 1 {
		return ErrTBankSignature
	}
	return nil
}
