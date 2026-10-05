// Package outcome is the CLI's closed set of results and the one table that
// maps them to process exit codes and to the `outcome` field of --json
// output. Agents branch on these, so the spellings and codes are a contract
// (docs/cli-plan.md §3 in the server repository): each run maps exactly one
// Outcome to one exit code.
package outcome

// Outcome is a run's result.
type Outcome string

const (
	OK              Outcome = "ok"
	Deployed        Outcome = "deployed"
	Failed          Outcome = "failed"
	Timeout         Outcome = "timeout"
	Transient       Outcome = "transient"
	Auth            Outcome = "auth"
	Usage           Outcome = "usage"
	Internal        Outcome = "internal"
	UpgradeRequired Outcome = "upgrade_required"
	NotDeployable   Outcome = "not_deployable"
	PlanLocked      Outcome = "plan_locked"
	// InProgress is MCP-only (a bounded wait that has not finished); it is
	// never a process exit.
	InProgress Outcome = "in_progress"
)

// ExitCode is the process exit status for an outcome.
func (o Outcome) ExitCode() int {
	switch o {
	case OK, Deployed:
		return 0
	case Failed:
		return 1
	case Timeout:
		return 2
	case Transient:
		return 3
	case Auth:
		return 4
	case Usage:
		return 5
	case UpgradeRequired:
		return 7
	case NotDeployable:
		return 8
	case PlanLocked:
		return 9
	default: // Internal, and anything unrecognised
		return 6
	}
}

// Error kinds the CLI produces itself, beside the server's v1 kinds. They
// share the {kind, message} shape in --json output.
const (
	KindTransport              = "transport"
	KindInternal               = "internal"
	KindConflictingCredentials = "conflicting_credentials"
	KindAmbiguousRepo          = "ambiguous_repo"
	KindNotPushed              = "not_pushed"
)

// ForKind maps an error kind — a v1 server kind or a local one — to its
// outcome. ok is false for a kind this CLI does not know: the caller treats
// that as Internal (fail loud: the server is newer than this CLI).
func ForKind(kind string) (Outcome, bool) {
	switch kind {
	case "provider_unavailable", "rate_limited", "workspace_unavailable", "storage_unavailable", KindTransport:
		return Transient, true
	case "unauthenticated", "token_revoked":
		return Auth, true
	case "invalid_request", "not_found", KindConflictingCredentials, KindAmbiguousRepo, KindNotPushed:
		return Usage, true
	case "cli_upgrade_required":
		return UpgradeRequired, true
	case "plan_feature_locked":
		return PlanLocked, true
	case KindInternal:
		return Internal, true
	default:
		return Internal, false
	}
}
