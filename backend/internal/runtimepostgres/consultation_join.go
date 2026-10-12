package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/communication"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// PrepareCommunicationJoin reads the canonical booked participant, current
// entitlement, versioned join policy and configured provider. Missing
// entitlements fail closed; readiness does not manufacture entitlement truth.
func (c *Checker) PrepareCommunicationJoin(ctx context.Context, bookingID, identityID, providerID, policyVersion, key string, now time.Time) (*communication.JoinPreparation, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("consultation storage unavailable")
	}
	var state, clientID, specialistID, providerStatus, providerKind, providerConfig string
	var starts, ends time.Time
	var earlySeconds, lateSeconds, ttlSeconds int
	err := c.db.QueryRowContext(ctx, `SELECT b.state,b.client_identity_id::text,bs.specialist_identity_id::text,
        b.starts_at,b.ends_at,ci.status,ci.provider_kind,ci.config_ref,
        p.join_early_seconds,p.join_late_seconds,p.max_credential_ttl_seconds
        FROM bookings b JOIN booking_slots bs ON bs.id=b.slot_id
        JOIN connector_instances ci ON ci.id=$3::uuid AND ci.capability_class='COMMUNICATION_PROVIDER'
        JOIN communication_access_policy_versions p ON p.version=$4
        WHERE b.id=$1::uuid`, bookingID, identityID, providerID, policyVersion).
		Scan(&state, &clientID, &specialistID, &starts, &ends, &providerStatus, &providerKind, &providerConfig,
			&earlySeconds, &lateSeconds, &ttlSeconds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, communication.ErrRoomIssuerUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("load communication join policy: %w", err)
	}
	role := communication.RoleClient
	switch identityID {
	case clientID:
		role = communication.RoleClient
	case specialistID:
		role = communication.RoleSpecialist
	default:
		return nil, nil
	}
	var entitlementID, entitlementState, basisRef string
	var proofExpires time.Time
	err = c.db.QueryRowContext(ctx, `SELECT id::text,state,basis_ref,expires_at
        FROM booking_access_entitlement_versions
        WHERE booking_id=$1::uuid AND identity_id=$2::uuid AND role=$3
          AND effective_from<=$4
        ORDER BY effective_from DESC,created_at DESC,id DESC LIMIT 1`,
		bookingID, identityID, string(role), now.UTC()).Scan(&entitlementID, &entitlementState, &basisRef, &proofExpires)
	if !errors.Is(err, sql.ErrNoRows) && err != nil {
		return nil, fmt.Errorf("read communication entitlement: %w", err)
	}
	decision, err := communication.AuthorizeJoin(
		communication.JoinRequest{IdentityID: identityID, Role: role, IdempotencyKey: key, Now: now.UTC()},
		communication.BookingAccess{BookingID: bookingID, State: booking.State(state),
			ClientIdentityID: clientID, SpecialistIdentityID: specialistID},
		communication.EntitlementProof{ID: entitlementID, BookingID: bookingID,
			IdentityID: identityID, Active: entitlementState == "ACTIVE", EvidenceRef: basisRef, ExpiresAt: proofExpires},
		communication.JoinWindow{OpensAt: starts.Add(-time.Duration(earlySeconds) * time.Second),
			ClosesAt: ends.Add(time.Duration(lateSeconds) * time.Second)},
		connector.Instance{ID: providerID, Capability: connector.CapabilityCommunication,
			ProviderKind: providerKind, ConfigRef: providerConfig, Status: connector.Status(providerStatus)},
		communication.JoinPolicy{Version: policyVersion, MaxCredentialTTL: time.Duration(ttlSeconds) * time.Second},
	)
	if err != nil {
		return nil, err
	}
	return &communication.JoinPreparation{Decision: decision, Role: role, EntitlementVersionID: entitlementID}, nil
}

// RecordCommunicationJoin is checked again by the pre-existing PostgreSQL
// guard in the same transaction. A revoked entitlement/cancelled booking can't
// be bypassed by a stale Prepare result. No raw join URI is stored.
func (c *Checker) RecordCommunicationJoin(ctx context.Context, bookingID, identityID, key string, prepared communication.JoinPreparation, grant communication.IssuedRoomGrant, now time.Time) error {
	if c == nil || c.db == nil || prepared.Decision.CredentialRequest == nil ||
		prepared.Decision.Decision != communication.Allow {
		return communication.ErrRoomGrantInvalid
	}
	request := prepared.Decision.CredentialRequest
	if grant.ProviderCredentialRef == "" || !grant.ExpiresAt.After(now) ||
		grant.ExpiresAt.After(request.ExpiresAt) || request.IdentityID != identityID ||
		request.BookingID != bookingID || request.IdempotencyKey != key {
		return communication.ErrRoomGrantInvalid
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id, err := persistentid.New()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO communication_join_authorizations(
        id,booking_id,identity_id,role,provider_instance_id,policy_version,
        entitlement_version_id,decision,reason_code,credential_scope,
        provider_credential_ref,credential_expires_at,idempotency_key,decided_at)
      VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid,$6,$7::uuid,
        'ALLOW','COMM_JOIN_ALLOWED','ROOM_JOIN',$8,$9,$10,$11)
      ON CONFLICT (booking_id,identity_id,idempotency_key) DO NOTHING`,
		id, bookingID, identityID, string(prepared.Role), prepared.Decision.ProviderInstanceID,
		prepared.Decision.PolicyVersion, prepared.EntitlementVersionID,
		grant.ProviderCredentialRef, grant.ExpiresAt, key, now.UTC())
	if err != nil {
		return fmt.Errorf("persist scoped communication authorization: %w", err)
	}
	var persistedRef, persistedPolicy, persistedProvider, persistedDecision string
	var persistedExpires time.Time
	err = tx.QueryRowContext(ctx, `SELECT provider_credential_ref,policy_version,
       provider_instance_id::text,decision,credential_expires_at
       FROM communication_join_authorizations
       WHERE booking_id=$1::uuid AND identity_id=$2::uuid AND idempotency_key=$3`,
		bookingID, identityID, key).Scan(&persistedRef, &persistedPolicy, &persistedProvider, &persistedDecision, &persistedExpires)
	if err != nil {
		return fmt.Errorf("verify persisted communication join: %w", err)
	}
	if persistedRef != grant.ProviderCredentialRef || persistedPolicy != prepared.Decision.PolicyVersion ||
		persistedProvider != prepared.Decision.ProviderInstanceID || persistedDecision != "ALLOW" ||
		!persistedExpires.Equal(grant.ExpiresAt) {
		return communication.ErrRoomGrantConflict
	}
	return tx.Commit()
}
