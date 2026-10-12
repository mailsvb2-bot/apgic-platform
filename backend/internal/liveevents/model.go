// Package liveevents owns APGIC live event and registration semantics.
// Communication/media and money execution belong to replaceable providers.
package liveevents

import (
 "errors"
 "strings"
 "time"
)

type State string
const (
 Draft State = "DRAFT"
 Published State = "PUBLISHED"
 Live State = "LIVE"
 Completed State = "COMPLETED"
 Cancelled State = "CANCELLED"
)
type Format string
const (
 Webinar Format = "WEBINAR"
 Group Format = "GROUP"
 Workshop Format = "WORKSHOP"
 QA Format = "QA"
 Conference Format = "CONFERENCE"
)
var (
 ErrInvalid = errors.New("LIVE_EVENT_INVALID")
 ErrForbidden = errors.New("LIVE_EVENT_FORBIDDEN")
 ErrNotFound = errors.New("LIVE_EVENT_NOT_FOUND")
 ErrConflict = errors.New("LIVE_EVENT_CONFLICT")
 ErrFull = errors.New("LIVE_EVENT_FULL")
 ErrUnavailable = errors.New("LIVE_PROVIDER_UNAVAILABLE")
 ErrPaymentRequired = errors.New("LIVE_PAYMENT_NOT_CONFIGURED")
)
type DraftInput struct {
 Title string `json:"title"`
 Description string `json:"description"`
 Format Format `json:"format"`
 StartsAt time.Time `json:"starts_at"`
 EndsAt time.Time `json:"ends_at"`
 Capacity int `json:"capacity"`
 PriceMinor int64 `json:"price_minor"`
 Currency string `json:"currency"`
}
type Event struct {
 ID string `json:"id"`
 OrganizationID string `json:"organization_id"`
 OrganizerIdentityID string `json:"organizer_identity_id,omitempty"`
 Title string `json:"title"`
 Description string `json:"description"`
 Format Format `json:"format"`
 StartsAt time.Time `json:"starts_at"`
 EndsAt time.Time `json:"ends_at"`
 Capacity int `json:"capacity"`
 Registered int `json:"registered"`
 State State `json:"state"`
 PriceMinor int64 `json:"price_minor"`
 Currency string `json:"currency"`
 RecordingAvailable bool `json:"recording_available"`
}
type Registration struct {
 ID string `json:"id"`
 EventID string `json:"event_id"`
 IdentityID string `json:"identity_id"`
 State string `json:"state"`
 RegisteredAt time.Time `json:"registered_at"`
}
type Access struct {
 Allowed bool `json:"allowed"`
 ReasonCode string `json:"reason_code"`
 ProviderReady bool `json:"provider_ready"`
 // Never return fabricated SFU credentials or expose the provider service secret.
}
func NormalizeDraft(input DraftInput, now time.Time) (DraftInput, error) {
 input.Title = strings.TrimSpace(input.Title)
 input.Description = strings.TrimSpace(input.Description)
 if len([]rune(input.Title)) < 5 || len([]rune(input.Title)) > 160 || len([]rune(input.Description)) > 4000 {
  return DraftInput{}, ErrInvalid
 }
 switch input.Format { case Webinar,Group,Workshop,QA,Conference: default: return DraftInput{},ErrInvalid }
 if input.StartsAt.IsZero() || input.EndsAt.IsZero() || !input.StartsAt.After(now.Add(5*time.Minute)) ||
  !input.EndsAt.After(input.StartsAt) || input.EndsAt.Sub(input.StartsAt) > 8*time.Hour ||
  input.Capacity < 2 || input.Capacity > 1000 { return DraftInput{},ErrInvalid }
 // Monetary registration MUST go through a verified PSP/Order/Entitlement flow.
 // No fake capture, "paid" flag, locally held money or default commission.
 if input.PriceMinor != 0 { return DraftInput{},ErrPaymentRequired }
 if input.Currency != "" && input.Currency != "RUB" { return DraftInput{},ErrInvalid }
 input.Currency = "RUB"
 input.StartsAt=input.StartsAt.UTC(); input.EndsAt=input.EndsAt.UTC()
 return input,nil
}
func CanTransition(from,to State) bool {
 switch from {
 case Draft:return to==Published||to==Cancelled
 case Published:return to==Live||to==Cancelled
 case Live:return to==Completed||to==Cancelled
 default:return false
 }
}
func CanRegister(state State, at, starts time.Time) bool {
 return state==Published && at.Before(starts)
}
func CanJoin(state State, registered bool, at, starts, ends time.Time, capabilityReady bool) Access {
 if !registered{return Access{ReasonCode:"LIVE_REGISTRATION_REQUIRED"}}
 if state!=Published&&state!=Live{return Access{ReasonCode:"LIVE_EVENT_NOT_JOINABLE"}}
 if at.Before(starts.Add(-15*time.Minute))||!at.Before(ends.Add(15*time.Minute)) {return Access{ReasonCode:"LIVE_OUTSIDE_JOIN_WINDOW"}}
 if !capabilityReady{return Access{ReasonCode:"LIVE_PROVIDER_UNAVAILABLE"}}
 return Access{Allowed:true,ReasonCode:"LIVE_JOIN_ELIGIBLE",ProviderReady:true}
}
