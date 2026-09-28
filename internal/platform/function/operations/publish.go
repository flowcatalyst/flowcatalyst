package operations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// maxPublishArtifactBytes caps how much of a stored artifact publish reads
// into memory — matches the raw-upload ceiling (api.maxArtifactBytes) so an
// artifact that could ever be stored can also ever be published (plan §8.2).
const maxPublishArtifactBytes = 64 * 1024 * 1024

// describeTimeout bounds the no-capability fc_describe call (plan §8.2 WP4
// task 2: "a 10 s context per publish; the JS engine compiles once").
const describeTimeout = 10 * time.Second

// describeMemoryCapBytes is the instance memory ceiling used only for
// reading describe — independent of any function's configured memoryMb
// (which may not exist yet: this can be the function's first version).
// Matches function.DefaultMemoryMB.
const describeMemoryCapBytes = uint64(function.DefaultMemoryMB) << 20

// PublishCommand is the input DTO.
type PublishCommand struct {
	FunctionID string `json:"functionId"`
	Digest     string `json:"digest"`
	// Runtime is "wasm" (default) or "js" (plan §9). Omitted means wasm.
	Runtime string `json:"runtime,omitempty"`
}

// PublishResult is what the API returns: the version (existing, on an
// idempotent replay, or newly created) and whether this call created it —
// the API maps Created to 201 vs 200.
type PublishResult struct {
	Version function.Version
	Created bool
}

// Publish reads a stored artifact's describe document through loader —
// exactly as the runner will (plan §3: "the platform never runs guest code,
// with one exception: at publish it instantiates the module with no
// capabilities to read its manifest") — validates it against the ABI
// package's own rules plus the platform's (emits ownership, schedule cron +
// timezone), and stores a new version. Publishing a digest already on this
// function returns the existing version unchanged — no write, no event
// (plan §4: "Publishing the same digest twice returns the existing
// version").
//
// A new version's insert, its version-number assignment, and the function's
// pool-revision bump all happen in one transaction (plan §8.1: "candidates
// appear in desired state so a runner proves them loadable before anyone
// promotes").
//
// This is a [usecaseop.TxOperation] (not a plain [usecaseop.Operation])
// because the version row is a persistence detail written directly via
// repo.InsertVersionTx on the scoped transaction (s.WithTx) rather than
// through the [usecaseop.Plan] machinery — the same pattern
// application/operations/provision_service_account.go uses for its linked
// principal row — while [FunctionVersionPublished] is still emitted as a
// proper atomic domain event via EmitEventScoped.
func Publish(repo *function.Repository, artifacts artifact.Store, loader *runtimes.Loader) usecaseop.TxOperation[PublishCommand, PublishResult] {
	return usecaseop.TxOperation[PublishCommand, PublishResult]{
		Name: "PublishFunctionVersion",
		Validate: func(_ context.Context, cmd PublishCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if !artifact.ValidDigest(strings.ToLower(strings.TrimSpace(cmd.Digest))) {
				return usecase.Validation("INVALID_DIGEST", "digest must be a lowercase hex sha256 (64 characters)")
			}
			if cmd.Runtime != "" && !runtimes.Valid(cmd.Runtime) {
				return usecase.Validation("INVALID_RUNTIME", `runtime must be "wasm" or "js"`)
			}
			return nil
		},
		// Per-resource authz needs the loaded function (ClientID), so it runs
		// post-load in Execute; the coarse "may publish" permission is on the
		// controller.
		Authorize: usecaseop.Public[PublishCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd PublishCommand, ec usecase.ExecutionContext) (PublishResult, error) {
			var zero PublishResult

			f, err := repo.FindByID(ctx, cmd.FunctionID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.FunctionID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}

			digest := strings.ToLower(strings.TrimSpace(cmd.Digest))

			// Idempotent replay: publishing the same digest again returns the
			// existing version untouched — no writes, no event.
			existing, err := repo.GetVersionByDigest(ctx, f.ID, digest)
			if err != nil {
				return zero, usecase.Internal("REPO", "get_version_by_digest failed", err)
			}
			if existing != nil {
				return PublishResult{Version: *existing, Created: false}, nil
			}

			exists, err := artifacts.Exists(ctx, digest)
			if err != nil {
				return zero, usecase.Internal("ARTIFACT_STORE", "exists check failed", err)
			}
			if !exists {
				return zero, usecase.NotFound("ARTIFACT_NOT_FOUND", "no artifact stored for digest "+digest)
			}

			rc, err := artifacts.Open(ctx, digest)
			if err != nil {
				return zero, usecase.Internal("ARTIFACT_STORE", "open failed", err)
			}
			data, readErr := io.ReadAll(io.LimitReader(rc, maxPublishArtifactBytes+1))
			_ = rc.Close()
			if readErr != nil {
				return zero, usecase.Internal("ARTIFACT_STORE", "read failed", readErr)
			}
			if len(data) > maxPublishArtifactBytes {
				return zero, usecase.Validation("ARTIFACT_TOO_LARGE", "artifact exceeds the 64 MiB publish ceiling")
			}

			runtimeName := strings.TrimSpace(cmd.Runtime)
			if runtimeName == "" {
				runtimeName = function.DefaultRuntime
			}

			describeCtx, cancel := context.WithTimeout(ctx, describeTimeout)
			defer cancel()
			doc, err := loader.Describe(describeCtx, runtimeName, data, describeMemoryCapBytes)
			if err != nil {
				return zero, publishDescribeError(err)
			}

			d, err := abi.ParseDescribe(doc)
			if err != nil {
				return zero, publishDescribeError(err)
			}

			// Platform validation on top of abi's own (plan §8.2 WP4 task 2):
			// every emits type's first segment must be this function's
			// application.
			for _, et := range d.Emits {
				app, _, _ := strings.Cut(et, ":")
				if app != f.ApplicationCode {
					return zero, usecase.Validation("EMIT_NOT_OWNED",
						fmt.Sprintf("emits %q is not owned by application %q", et, f.ApplicationCode))
				}
			}
			// Every schedule's cron must actually produce fire slots (not just
			// have a valid field count — abi.Describe.Validate only checks
			// shape) with the scheduled-job module's own firing parser, and its
			// timezone, if given, must load.
			for _, sc := range d.Schedules {
				if err := scheduledjob.ValidateCronFires(sc.Cron); err != nil {
					return zero, usecase.Validation("SCHEDULE_CRON_INVALID",
						fmt.Sprintf("schedules cron %q does not parse: %v", sc.Cron, err))
				}
				if sc.Timezone != "" {
					if _, err := time.LoadLocation(sc.Timezone); err != nil {
						return zero, usecase.Validation("SCHEDULE_TIMEZONE_INVALID",
							fmt.Sprintf("schedules timezone %q does not load: %v", sc.Timezone, err))
					}
				}
			}

			v := &function.Version{
				ID:          tsid.Generate(tsid.FunctionVersion),
				FunctionID:  f.ID,
				Digest:      digest,
				SizeBytes:   int64(len(data)),
				ABI:         int32(d.ABI),
				Runtime:     runtimeName,
				Describe:    doc,
				Status:      function.VersionPublished,
				PublishedBy: &ec.PrincipalID,
				CreatedAt:   time.Now().UTC(),
			}

			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				number, lockErr := repo.LockAndNextVersionNumber(ctx, tx, f.ID)
				if lockErr != nil {
					return lockErr
				}
				v.Number = number
				if insErr := repo.InsertVersionTx(ctx, v, tx); insErr != nil {
					return insErr
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				return zero, usecase.Internal("REPO", "publish write failed", err)
			}

			event := FunctionVersionPublished{
				Metadata:   usecase.NewEventMetadata(ec, FunctionVersionPublishedType, Source, subjectFor(f.ID)),
				FunctionID: f.ID,
				Address:    f.Address,
				Number:     v.Number,
				Digest:     v.Digest,
				Runtime:    v.Runtime,
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			return PublishResult{Version: *v, Created: true}, nil
		},
	}
}

// publishDescribeError maps a describe-read failure to a 400 listing every
// problem found (plan §8.2 WP4 task 2: "A publish failure returns 400 with
// the describe problems listed").
func publishDescribeError(err error) error {
	if de, ok := errors.AsType[*abi.DescribeError](err); ok {
		return &usecase.Error{
			Kind:    usecase.KindValidation,
			Code:    "DESCRIBE_INVALID",
			Message: "the artifact's describe document is invalid: " + strings.Join(de.Problems, "; "),
			Details: map[string]any{"problems": de.Problems},
		}
	}
	if le, ok := errors.AsType[*engine.LoadError](err); ok {
		return usecase.Validation("DESCRIBE_LOAD_FAILED",
			"could not load the artifact to read its describe document: "+le.Error())
	}
	return usecase.Validation("DESCRIBE_FAILED",
		"could not read the artifact's describe document: "+err.Error())
}

// ── Versions: list / get / retire ──────────────────────────────────────────

// ListVersions returns every version for functionID, newest first. Not a
// usecaseop.Operation: a pure read, gated by the same permission +
// CheckScopeAccess pattern as the other GET handlers, driven directly from
// api.go (mirrors getByID there).
func ListVersions(ctx context.Context, repo *function.Repository, functionID string) (*function.Function, []function.Version, error) {
	f, err := repo.FindByID(ctx, functionID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if f == nil {
		return nil, nil, httperror.NotFound("Function", functionID)
	}
	if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
		return nil, nil, err
	}
	versions, err := repo.ListVersionsByFunction(ctx, functionID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "list_versions_by_function failed", err)
	}
	return f, versions, nil
}

// GetVersion returns one version by number.
func GetVersion(ctx context.Context, repo *function.Repository, functionID string, number int32) (*function.Function, *function.Version, error) {
	f, err := repo.FindByID(ctx, functionID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if f == nil {
		return nil, nil, httperror.NotFound("Function", functionID)
	}
	if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
		return nil, nil, err
	}
	v, err := repo.GetVersionByNumber(ctx, functionID, number)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "get_version_by_number failed", err)
	}
	if v == nil {
		return nil, nil, usecase.NotFound("VERSION_NOT_FOUND", fmt.Sprintf("function %s has no version %d", functionID, number))
	}
	return f, v, nil
}

// RetireCommand is the input DTO for retiring a version.
type RetireCommand struct {
	FunctionID string `json:"functionId"`
	Number     int32  `json:"number"`
}

// RetireVersion refuses (RETIRE_IN_USE) if any alias — including `live` —
// still points at the version; otherwise it flips the version to RETIRED and
// bumps the pool revision (a retired version is no longer a promote
// candidate).
func RetireVersion(repo *function.Repository) usecaseop.TxOperation[RetireCommand, function.Version] {
	return usecaseop.TxOperation[RetireCommand, function.Version]{
		Name: "RetireFunctionVersion",
		Validate: func(_ context.Context, cmd RetireCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if cmd.Number < 1 {
				return usecase.Validation("NUMBER_REQUIRED", "number must be >= 1")
			}
			return nil
		},
		Authorize: usecaseop.Public[RetireCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RetireCommand, ec usecase.ExecutionContext) (function.Version, error) {
			var zero function.Version

			f, err := repo.FindByID(ctx, cmd.FunctionID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.FunctionID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}

			v, err := repo.GetVersionByNumber(ctx, f.ID, cmd.Number)
			if err != nil {
				return zero, usecase.Internal("REPO", "get_version_by_number failed", err)
			}
			if v == nil {
				return zero, usecase.NotFound("VERSION_NOT_FOUND", fmt.Sprintf("function %s has no version %d", f.ID, cmd.Number))
			}
			if v.Status == function.VersionRetired {
				return *v, nil // already retired: idempotent no-op
			}

			aliases, err := repo.ListAliases(ctx, f.ID)
			if err != nil {
				return zero, usecase.Internal("REPO", "list_aliases failed", err)
			}
			var inUse []string
			for _, a := range aliases {
				if a.VersionID == v.ID {
					inUse = append(inUse, a.Name)
				}
			}
			if len(inUse) > 0 {
				return zero, usecase.Conflict("RETIRE_IN_USE",
					fmt.Sprintf("version %d is still pointed at by alias(es): %s", cmd.Number, strings.Join(inUse, ", ")))
			}

			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if _, execErr := tx.Exec(ctx, `UPDATE fn_versions SET status = $2 WHERE id = $1`, v.ID, string(function.VersionRetired)); execErr != nil {
					return execErr
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				return zero, usecase.Internal("REPO", "retire write failed", err)
			}

			event := FunctionVersionRetired{
				Metadata:   usecase.NewEventMetadata(ec, FunctionVersionRetiredType, Source, subjectFor(f.ID)),
				FunctionID: f.ID,
				Address:    f.Address,
				Number:     cmd.Number,
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			v.Status = function.VersionRetired
			return *v, nil
		},
	}
}
