package commerce

import (
	"errors"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

type ClientSurface string

const (
	SurfaceIOS     ClientSurface = "IOS"
	SurfaceAndroid ClientSurface = "ANDROID"
)

type StoreCommerceOutcome string

const (
	StoreCommerceAllowed  StoreCommerceOutcome = "ALLOWED"
	StoreCommerceDisabled StoreCommerceOutcome = "PURCHASE_DISABLED"
)

var (
	ErrInvalidStorePolicy   = errors.New("invalid store policy snapshot")
	ErrStorePolicyAmbiguous = errors.New("store commerce policy matched more than one rule")
	ErrInvalidStorePurchase = errors.New("invalid verified store purchase")
)

type StorePolicyRule struct {
	ProductType  string
	Surface      ClientSurface
	Store        string
	Storefront   string
	Jurisdiction string
	Enabled      bool
	Rail         payments.RailCode
	ReasonCode   string
}

type StorePolicySnapshot struct {
	Version     string
	EffectiveAt time.Time
	Rules       []StorePolicyRule
}

type StoreCommerceContext struct {
	ProductType  string
	Surface      ClientSurface
	Store        string
	Storefront   string
	Jurisdiction string
}

type StoreCommerceDecision struct {
	PolicyVersion string
	Outcome       StoreCommerceOutcome
	Rail          payments.RailCode
	ReasonCode    string
}

func (s StorePolicySnapshot) Decide(ctx StoreCommerceContext) (StoreCommerceDecision, error) {
	if strings.TrimSpace(s.Version) == "" || s.EffectiveAt.IsZero() || len(s.Rules) == 0 {
		return StoreCommerceDecision{}, ErrInvalidStorePolicy
	}
	if !validSurface(ctx.Surface) ||
		strings.TrimSpace(ctx.ProductType) == "" ||
		strings.TrimSpace(ctx.Store) == "" ||
		strings.TrimSpace(ctx.Storefront) == "" ||
		strings.TrimSpace(ctx.Jurisdiction) == "" {
		return StoreCommerceDecision{}, ErrInvalidStorePolicy
	}

	var matched *StorePolicyRule
	for i := range s.Rules {
		rule := &s.Rules[i]
		if !validRule(*rule) {
			return StoreCommerceDecision{}, ErrInvalidStorePolicy
		}
		if rule.ProductType == ctx.ProductType &&
			rule.Surface == ctx.Surface &&
			rule.Store == ctx.Store &&
			rule.Storefront == ctx.Storefront &&
			rule.Jurisdiction == ctx.Jurisdiction {
			if matched != nil {
				return StoreCommerceDecision{}, ErrStorePolicyAmbiguous
			}
			matched = rule
		}
	}
	if matched == nil {
		return StoreCommerceDecision{
			PolicyVersion: s.Version,
			Outcome:       StoreCommerceDisabled,
			Rail:          payments.RailPurchaseDisabled,
			ReasonCode:    "STORE_POLICY_NOT_CONFIGURED",
		}, nil
	}
	if !matched.Enabled {
		return StoreCommerceDecision{
			PolicyVersion: s.Version,
			Outcome:       StoreCommerceDisabled,
			Rail:          payments.RailPurchaseDisabled,
			ReasonCode:    matched.ReasonCode,
		}, nil
	}
	return StoreCommerceDecision{
		PolicyVersion: s.Version,
		Outcome:       StoreCommerceAllowed,
		Rail:          matched.Rail,
		ReasonCode:    matched.ReasonCode,
	}, nil
}

func validRule(rule StorePolicyRule) bool {
	if strings.TrimSpace(rule.ProductType) == "" ||
		!validSurface(rule.Surface) ||
		strings.TrimSpace(rule.Store) == "" ||
		strings.TrimSpace(rule.Storefront) == "" ||
		strings.TrimSpace(rule.Jurisdiction) == "" ||
		strings.TrimSpace(rule.ReasonCode) == "" {
		return false
	}
	if rule.Enabled {
		return rule.Rail != "" && rule.Rail != payments.RailPurchaseDisabled
	}
	return rule.Rail == payments.RailPurchaseDisabled
}

func validSurface(surface ClientSurface) bool {
	return surface == SurfaceIOS || surface == SurfaceAndroid
}

type VerifiedStorePurchase struct {
	ID                    string
	IdentityID            string
	OrderID               string
	PaymentAttemptID      string
	PaymentEffectID       string
	LedgerEntryID         string
	ProviderInstanceID    string
	ExternalTransactionID string
	ProductRef            string
	EntitlementKind       string
	PolicyVersion         string
	ProviderEvidenceRef   string
	VerifiedAt            time.Time
}

func (p VerifiedStorePurchase) Validate() error {
	if strings.TrimSpace(p.ID) == "" ||
		strings.TrimSpace(p.IdentityID) == "" ||
		strings.TrimSpace(p.OrderID) == "" ||
		strings.TrimSpace(p.PaymentAttemptID) == "" ||
		strings.TrimSpace(p.PaymentEffectID) == "" ||
		strings.TrimSpace(p.LedgerEntryID) == "" ||
		strings.TrimSpace(p.ProviderInstanceID) == "" ||
		strings.TrimSpace(p.ExternalTransactionID) == "" ||
		strings.TrimSpace(p.ProductRef) == "" ||
		strings.TrimSpace(p.EntitlementKind) == "" ||
		strings.TrimSpace(p.PolicyVersion) == "" ||
		strings.TrimSpace(p.ProviderEvidenceRef) == "" ||
		p.VerifiedAt.IsZero() {
		return ErrInvalidStorePurchase
	}
	return nil
}
