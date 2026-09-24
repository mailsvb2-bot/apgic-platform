#!/usr/bin/env bash
set -euo pipefail

: "${PGHOST:=127.0.0.1}"
: "${PGPORT:=5432}"
: "${PGUSER:=apgic}"
: "${PGDATABASE:=apgic_ci}"
: "${RESTORE_DATABASE:=apgic_restore}"

evidence_dir="${GITHUB_WORKSPACE:-.}/evidence"
mkdir -p "$evidence_dir"

sentinel_identity="00000000-0000-0000-0000-00000000d001"
sentinel_org="00000000-0000-0000-0000-00000000d002"
sentinel_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
backup_file="$evidence_dir/apgic-ci.dump"

psql -v ON_ERROR_STOP=1 <<SQL
INSERT INTO identities (id, created_at)
VALUES ('$sentinel_identity', '$sentinel_time'::timestamptz);

INSERT INTO organizations (id, name, status, created_at, updated_at)
VALUES (
  '$sentinel_org',
  'Restore Drill Organization',
  'ACTIVE',
  '$sentinel_time'::timestamptz,
  '$sentinel_time'::timestamptz
);
SQL

backup_started_ms="$(date +%s%3N)"
pg_dump --format=custom --file="$backup_file" "$PGDATABASE"
backup_finished_ms="$(date +%s%3N)"

dropdb --if-exists "$RESTORE_DATABASE"
createdb "$RESTORE_DATABASE"

restore_started_ms="$(date +%s%3N)"
pg_restore --exit-on-error --no-owner --dbname="$RESTORE_DATABASE" "$backup_file"
restore_finished_ms="$(date +%s%3N)"

identity_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM identities WHERE id = '$sentinel_identity';"
)"
organization_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM organizations WHERE id = '$sentinel_org' AND status = 'ACTIVE';"
)"
audit_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'audit_records_append_only' AND NOT tgisinternal;"
)"
ledger_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'ledger_entries_append_only' AND NOT tgisinternal;"
)"
legal_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'legal_acceptances_append_only' AND NOT tgisinternal;"
)"
direction_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'organization_directions_no_delete' AND NOT tgisinternal;"
)"
product_owner_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'products_owner_exists' AND NOT tgisinternal;"
)"
outbox_delivery_constraint_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_constraint WHERE conname = 'outbox_delivery_evidence_check';"
)"
outbox_transition_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'outbox_events_transition_guard' AND NOT tgisinternal;"
)"
direction_archive_constraint_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_constraint WHERE conname = 'organization_directions_archive_timestamp_check';"
)"
legal_transaction_snapshot_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'legal_transaction_snapshots_append_only' AND NOT tgisinternal;"
)"
delete_request_transition_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'delete_account_requests_transition_guard' AND NOT tgisinternal;"
)"
delete_request_no_delete_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'delete_account_requests_no_delete' AND NOT tgisinternal;"
)"
provider_erasure_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'provider_erasure_jobs_transition_guard' AND NOT tgisinternal;"
)"
specialist_publication_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'specialist_publications_guard' AND NOT tgisinternal;"
)"
specialist_profile_revalidation_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'specialist_profile_revalidation_guard' AND NOT tgisinternal;"
)"
specialist_capability_revalidation_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'specialist_capability_revalidation_guard' AND NOT tgisinternal;"
)"
qualification_append_only_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'qualification_evaluations_append_only' AND NOT tgisinternal;"
)"
booking_hold_transition_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'booking_holds_transition_guard' AND NOT tgisinternal;"
)"
booking_hold_insert_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'booking_holds_insert_guard' AND NOT tgisinternal;"
)"
booking_transition_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'bookings_transition_guard' AND NOT tgisinternal;"
)"
booking_insert_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'bookings_insert_guard' AND NOT tgisinternal;"
)"
booking_consume_hold_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'bookings_consume_hold' AND NOT tgisinternal;"
)"
orders_append_only_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'orders_append_only' AND NOT tgisinternal;"
)"
orders_snapshot_guard_trigger_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'orders_snapshot_guard' AND NOT tgisinternal;"
)"
booking_acquire_function_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_proc WHERE proname = 'apgic_acquire_slot_hold';"
)"
payment_provider_config_append_only_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_provider_config_versions_append_only' AND NOT tgisinternal;"
)"
payment_routing_append_only_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_routing_decisions_append_only' AND NOT tgisinternal;"
)"
payment_attempt_transition_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_attempts_transition_guard' AND NOT tgisinternal;"
)"
payment_attempt_no_delete_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_attempts_no_delete' AND NOT tgisinternal;"
)"
payment_webhook_append_only_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_webhook_receipts_append_only' AND NOT tgisinternal;"
)"
payment_effect_append_only_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_effects_append_only' AND NOT tgisinternal;"
)"
payment_webhook_function_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_proc WHERE proname = 'apgic_record_payment_webhook';"
)"
refund_transition_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'refund_requests_transition_guard' AND NOT tgisinternal;"
)"
refund_no_delete_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'refund_requests_no_delete' AND NOT tgisinternal;"
)"
refund_effect_append_only_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'refund_effects_append_only' AND NOT tgisinternal;"
)"
refund_apply_function_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_proc WHERE proname = 'apgic_apply_refund_success';"
)"
notification_intent_append_only_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'notification_intents_append_only' AND NOT tgisinternal;"
)"
notification_delivery_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'notification_deliveries_transition_guard' AND NOT tgisinternal;"
)"
calendar_sync_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'calendar_sync_jobs_transition_guard' AND NOT tgisinternal;"
)"
client_mutation_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'client_mutation_records_transition_guard' AND NOT tgisinternal;"
)"
notification_intent_function_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_proc WHERE proname = 'apgic_create_notification_intent';"
)"
client_mutation_claim_function_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_proc WHERE proname = 'apgic_claim_client_mutation';"
)"
payment_control_event_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_provider_control_events_insert_guard' AND NOT tgisinternal;"
)"
payment_control_event_append_only_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_provider_control_events_append_only' AND NOT tgisinternal;"
)"
payment_health_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_provider_health_snapshots_insert_guard' AND NOT tgisinternal;"
)"
payment_health_append_only_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'payment_provider_health_snapshots_append_only' AND NOT tgisinternal;"
)"
store_policy_append_only_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'store_policy_snapshots_append_only' AND NOT tgisinternal;"
)"
store_decision_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'store_commerce_decisions_insert_guard' AND NOT tgisinternal;"
)"
store_verification_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'store_transaction_verifications_insert_guard' AND NOT tgisinternal;"
)"
store_entitlement_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'store_entitlements_insert_guard' AND NOT tgisinternal;"
)"
store_exit_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'store_subscription_exit_actions_insert_guard' AND NOT tgisinternal;"
)"
device_integrity_evidence_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'device_integrity_evidence_insert_guard' AND NOT tgisinternal;"
)"
device_integrity_risk_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'device_integrity_risk_decisions_insert_guard' AND NOT tgisinternal;"
)"
noncash_entitlement_guard_count="$(
  psql --dbname="$RESTORE_DATABASE" -Atc     "SELECT count(*) FROM pg_trigger WHERE tgname = 'noncash_entitlement_entries_insert_guard' AND NOT tgisinternal;"
)"

if [[ "$identity_count" != "1" || "$organization_count" != "1" ]]; then
  echo "restore drill failed: business sentinel missing" >&2
  exit 1
fi

if [[ "$audit_trigger_count" != "1" || "$ledger_trigger_count" != "1" || "$legal_trigger_count" != "1" ]]; then
  echo "restore drill failed: append-only trigger missing" >&2
  exit 1
fi

if [[ "$direction_trigger_count" != "1" || "$product_owner_trigger_count" != "1" || "$outbox_delivery_constraint_count" != "1" || "$outbox_transition_trigger_count" != "1" || "$direction_archive_constraint_count" != "1" || "$legal_transaction_snapshot_trigger_count" != "1" || "$delete_request_transition_trigger_count" != "1" || "$delete_request_no_delete_trigger_count" != "1" || "$provider_erasure_trigger_count" != "1" || "$specialist_publication_guard_count" != "1" || "$specialist_profile_revalidation_guard_count" != "1" || "$specialist_capability_revalidation_guard_count" != "1" || "$qualification_append_only_trigger_count" != "1" || "$booking_hold_transition_trigger_count" != "1" || "$booking_hold_insert_trigger_count" != "1" || "$booking_transition_trigger_count" != "1" || "$booking_insert_trigger_count" != "1" || "$booking_consume_hold_trigger_count" != "1" || "$orders_append_only_trigger_count" != "1" || "$orders_snapshot_guard_trigger_count" != "1" || "$booking_acquire_function_count" != "1" || "$payment_provider_config_append_only_count" != "1" || "$payment_routing_append_only_count" != "1" || "$payment_attempt_transition_guard_count" != "1" || "$payment_attempt_no_delete_count" != "1" || "$payment_webhook_append_only_count" != "1" || "$payment_effect_append_only_count" != "1" || "$payment_webhook_function_count" != "1" || "$refund_transition_guard_count" != "1" || "$refund_no_delete_count" != "1" || "$refund_effect_append_only_count" != "1" || "$refund_apply_function_count" != "1" || "$notification_intent_append_only_count" != "1" || "$notification_delivery_guard_count" != "1" || "$calendar_sync_guard_count" != "1" || "$client_mutation_guard_count" != "1" || "$notification_intent_function_count" != "1" || "$client_mutation_claim_function_count" != "1" || "$payment_control_event_guard_count" != "1" || "$payment_control_event_append_only_count" != "1" || "$payment_health_guard_count" != "1" || "$payment_health_append_only_count" != "1" || "$store_policy_append_only_count" != "1" || "$store_decision_guard_count" != "1" || "$store_verification_guard_count" != "1" || "$store_entitlement_guard_count" != "1" || "$store_exit_guard_count" != "1" || "$device_integrity_evidence_guard_count" != "1" || "$device_integrity_risk_guard_count" != "1" || "$noncash_entitlement_guard_count" != "1" ]]; then
  echo "restore drill failed: semantic invariant object missing" >&2
  exit 1
fi

rto_ms=$((restore_finished_ms - restore_started_ms))
backup_ms=$((backup_finished_ms - backup_started_ms))

cat >"$evidence_dir/restore-drill.json" <<JSON
{
  "evidence_type": "CI_RESTORE_DRILL",
  "candidate_sha": "${APGIC_CANDIDATE_SHA:-${GITHUB_SHA:-unknown}}",
  "source_database": "$PGDATABASE",
  "target_database": "$RESTORE_DATABASE",
  "sentinel_created_at": "$sentinel_time",
  "backup_duration_ms": $backup_ms,
  "measured_restore_rto_ms": $rto_ms,
  "observed_data_loss_records": 0,
  "business_probe_identity_count": $identity_count,
  "business_probe_organization_count": $organization_count,
  "integrity_probe_audit_trigger_count": $audit_trigger_count,
  "integrity_probe_ledger_trigger_count": $ledger_trigger_count,
  "integrity_probe_legal_trigger_count": $legal_trigger_count,
  "integrity_probe_direction_trigger_count": $direction_trigger_count,
  "integrity_probe_product_owner_trigger_count": $product_owner_trigger_count,
  "integrity_probe_outbox_delivery_constraint_count": $outbox_delivery_constraint_count,
  "integrity_probe_outbox_transition_trigger_count": $outbox_transition_trigger_count,
  "integrity_probe_direction_archive_constraint_count": $direction_archive_constraint_count,
  "integrity_probe_legal_transaction_snapshot_trigger_count": $legal_transaction_snapshot_trigger_count,
  "integrity_probe_delete_request_transition_trigger_count": $delete_request_transition_trigger_count,
  "integrity_probe_delete_request_no_delete_trigger_count": $delete_request_no_delete_trigger_count,
  "integrity_probe_provider_erasure_trigger_count": $provider_erasure_trigger_count,
  "integrity_probe_specialist_publication_guard_count": $specialist_publication_guard_count,
  "integrity_probe_specialist_profile_revalidation_guard_count": $specialist_profile_revalidation_guard_count,
  "integrity_probe_specialist_capability_revalidation_guard_count": $specialist_capability_revalidation_guard_count,
  "integrity_probe_qualification_append_only_trigger_count": $qualification_append_only_trigger_count,
  "integrity_probe_booking_hold_transition_trigger_count": $booking_hold_transition_trigger_count,
  "integrity_probe_booking_hold_insert_trigger_count": $booking_hold_insert_trigger_count,
  "integrity_probe_booking_transition_trigger_count": $booking_transition_trigger_count,
  "integrity_probe_booking_insert_trigger_count": $booking_insert_trigger_count,
  "integrity_probe_booking_consume_hold_trigger_count": $booking_consume_hold_trigger_count,
  "integrity_probe_orders_append_only_trigger_count": $orders_append_only_trigger_count,
  "integrity_probe_orders_snapshot_guard_trigger_count": $orders_snapshot_guard_trigger_count,
  "integrity_probe_booking_acquire_function_count": $booking_acquire_function_count,
  "integrity_probe_payment_provider_config_append_only_count": $payment_provider_config_append_only_count,
  "integrity_probe_payment_routing_append_only_count": $payment_routing_append_only_count,
  "integrity_probe_payment_attempt_transition_guard_count": $payment_attempt_transition_guard_count,
  "integrity_probe_payment_attempt_no_delete_count": $payment_attempt_no_delete_count,
  "integrity_probe_payment_webhook_append_only_count": $payment_webhook_append_only_count,
  "integrity_probe_payment_effect_append_only_count": $payment_effect_append_only_count,
  "integrity_probe_payment_webhook_function_count": $payment_webhook_function_count,
  "integrity_probe_refund_transition_guard_count": $refund_transition_guard_count,
  "integrity_probe_refund_no_delete_count": $refund_no_delete_count,
  "integrity_probe_refund_effect_append_only_count": $refund_effect_append_only_count,
  "integrity_probe_refund_apply_function_count": $refund_apply_function_count,
  "integrity_probe_notification_intent_append_only_count": $notification_intent_append_only_count,
  "integrity_probe_notification_delivery_guard_count": $notification_delivery_guard_count,
  "integrity_probe_calendar_sync_guard_count": $calendar_sync_guard_count,
  "integrity_probe_client_mutation_guard_count": $client_mutation_guard_count,
  "integrity_probe_notification_intent_function_count": $notification_intent_function_count,
  "integrity_probe_client_mutation_claim_function_count": $client_mutation_claim_function_count,
  "integrity_probe_payment_control_event_guard_count": $payment_control_event_guard_count,
  "integrity_probe_payment_control_event_append_only_count": $payment_control_event_append_only_count,
  "integrity_probe_payment_health_guard_count": $payment_health_guard_count,
  "integrity_probe_payment_health_append_only_count": $payment_health_append_only_count,
  "integrity_probe_store_policy_append_only_count": $store_policy_append_only_count,
  "integrity_probe_store_decision_guard_count": $store_decision_guard_count,
  "integrity_probe_store_verification_guard_count": $store_verification_guard_count,
  "integrity_probe_store_entitlement_guard_count": $store_entitlement_guard_count,
  "integrity_probe_store_exit_guard_count": $store_exit_guard_count,
  "integrity_probe_device_integrity_evidence_guard_count": $device_integrity_evidence_guard_count,
  "integrity_probe_device_integrity_risk_guard_count": $device_integrity_risk_guard_count,
  "integrity_probe_noncash_entitlement_guard_count": $noncash_entitlement_guard_count,
  "production_evidence": false
}
JSON

cat "$evidence_dir/restore-drill.json"
