package migrate

import "fmt"

// Effect probes: for a shipped migration, a SQL expression that is true iff
// the migration's schema effect is visible in the connected database.
//
// They exist because a migration NUMBER is only evidence about the database
// that numbered it. Another implementation of the platform shares this schema
// and numbers its migrations identically through 030 and differently from 031
// (its 034 is the function registry, ours is 059), so a foreign tracker saying
// "034 applied" says nothing about our 034. The bootstrap therefore never
// copies numbers across: it seeds goose only with versions whose effect it can
// see, and lets goose apply the rest.
//
// Rules for adding one (migration authors: add a probe with every new
// migration that has a schema effect):
//   - the probe must describe the FINAL state after every later migration, not
//     the state the migration left (a later migration may drop what an earlier
//     one added); TestProbesHoldOnFullyMigratedDatabase enforces this;
//   - data-only migrations get no probe: they are re-run for real on a
//     foreign database, so they must stay idempotent (each already is);
//   - a migration whose effect cannot be seen on its own may be "implied" by a
//     later probe in the same family (see impliedBy).

func colExists(table, column string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = '%s' AND column_name = '%s')`, table, column)
}

func tblExists(table string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = '%s')`, table)
}

func idxExists(index string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM pg_indexes
		WHERE schemaname = current_schema() AND indexname = '%s')`, index)
}

func conExists(constraint string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '%s')`, constraint)
}

func partitioned(table string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM pg_partitioned_table pt
		JOIN pg_class c ON c.oid = pt.partrelid WHERE c.relname = '%s')`, table)
}

func both(a, b string) string { return "(" + a + ") AND (" + b + ")" }

// dispatchIndexFamily is the end state of 065-068: the scheduler indexes
// replaced by the status/group index, and the retired queue table gone.
var dispatchIndexFamily = both(idxExists("idx_dispatch_jobs_status_group"), "NOT "+tblExists("msg_dispatch_queue"))

var effectProbes = map[int64]string{
	1:  tblExists("tnt_clients"),
	2:  tblExists("iam_principals"),
	3:  tblExists("app_applications"),
	4:  tblExists("msg_events"),
	5:  tblExists("msg_event_projection_feed"),
	6:  tblExists("aud_logs"),
	7:  tblExists("oauth_identity_providers"),
	8:  tblExists("iam_password_reset_tokens"),
	10: tblExists("iam_oidc_login_states"),
	11: tblExists("msg_dispatch_jobs_read"),
	19: partitioned("msg_dispatch_jobs"),
	20: tblExists("webauthn_credentials"),
	21: tblExists("msg_scheduled_jobs"),
	22: partitioned("msg_scheduled_job_instances"),
	24: colExists("msg_scheduled_jobs", "target_url"),
	25: tblExists("app_application_openapi_specs"),
	26: tblExists("msg_processes"),
	27: conExists("oauth_clients_service_account_fk"),
	28: conExists("app_applications_service_account_fk"),
	29: tblExists("oauth_client_post_logout_redirect_uris"),
	30: tblExists("iam_rate_limit_events"),

	31: tblExists("tnt_email_domain_mapping_2fa_methods"),
	32: tblExists("iam_reset_approval_requests"),
	33: colExists("iam_password_reset_tokens", "factor_attempts"),
	34: colExists("iam_principals", "all_applications"),
	35: colExists("oauth_idp_role_mappings", "idp_type"),
	36: idxExists("idx_msg_dispatch_jobs_read_client_created"),
	37: idxExists("idx_iam_login_attempts_failure_throttle"),
	38: colExists("msg_scheduled_jobs", "application_id"),
	39: colExists("iam_principals", "dev_client_secret_ref"),
	40: tblExists("oauth_identity_provider_allowed_roles"),
	41: tblExists("portal_identities"),
	42: colExists("oauth_clients", "api_access"),
	44: tblExists("app_docs"),
	45: idxExists("idx_msg_dispatch_jobs_dirty"),
	46: colExists("oauth_clients", "previous_secret_ref"),
	47: colExists("oauth_clients", "previous_secret_last_used_at"),
	48: `EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'msg_dispatch_jobs'
		  AND column_name = 'mode' AND column_default LIKE '%NEXT_ON_ERROR%')`,
	49: partitioned("iam_login_attempts"),
	51: conExists("chk_msg_scheduled_job_instances_status"),
	52: conExists("chk_msg_dispatch_job_attempts_error_type"),
	53: tblExists("portal_apps"),
	54: colExists("msg_dispatch_jobs", "queue"),
	56: colExists("msg_connections", "application_code"),
	57: colExists("msg_dispatch_jobs", "descriptor"),
	58: `EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'aud_logs'
		  AND column_name = 'entity_id' AND character_maximum_length >= 100)`,
	59: tblExists("fng_functions"),
	60: colExists("msg_scheduled_job_instances", "not_before"),
	61: colExists("fng_versions", "runtime"),
	62: tblExists("fng_routes"),
	64: colExists("fng_routes", "alias_prefixes"),
	65: dispatchIndexFamily,
	66: dispatchIndexFamily,
	67: dispatchIndexFamily,
	68: dispatchIndexFamily,
	71: tblExists("msg_subscription_event_types_archive"),
	78: colExists("msg_dispatch_pools", "application_id"),
	81: colExists("fng_versions", "requeued_at_revision"),
	82: colExists("msg_dispatch_jobs", "connection_id"),
	84: colExists("fng_functions", "service_account_id"),
}

// impliedBy: a migration with no effect of its own (it only alters columns of
// tables that later migrations rebuild) counts as applied when the later
// migration that depends on it is visibly applied. Only used within 001-030,
// where every implementation numbers identically.
var impliedBy = map[int64]int64{
	9: 10, 12: 19, 13: 19, 14: 19, 15: 19, 16: 19, 17: 19, 18: 19,
}

// 043 drops a column an earlier migration added and 063, 069, 070, 072-077,
// 079, 080 are data migrations; none has a schema effect to probe, so
// foreign-database bootstrap never seeds them (goose runs them, and they are
// idempotent). 055 likewise.
