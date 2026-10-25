package operations

import (
	"context"
	"strconv"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// Outcome says what a save of a definition did. A save is deduplicated by the
// digest of the definition (owner ruling 2026-10-25): a definition that an
// existing version already says is never saved twice.
type Outcome string

// The outcomes of a save.
const (
	// OutcomeUnchanged: the live version already says it; nothing was written.
	OutcomeUnchanged Outcome = "UNCHANGED"
	// OutcomeSaved: saved as a new version and made live.
	OutcomeSaved Outcome = "SAVED"
	// OutcomeMadeLive: a saved version that was not live already says it; that
	// version was made live and nothing new was saved.
	OutcomeMadeLive Outcome = "MADE_LIVE"
	// OutcomeDraft: saved as a new version that is not live, a draft that waits
	// for someone who holds promote to make it live.
	OutcomeDraft Outcome = "DRAFT"
	// OutcomeDraftExists: a saved version that is not live already says it;
	// nothing was saved, and that draft still waits for promote.
	OutcomeDraftExists Outcome = "DRAFT_EXISTS"
)

// SaveResult is what a save of a definition did.
type SaveResult struct {
	Ingestion *ingestion.Ingestion
	Outcome   Outcome
	// Version is the number of the version the outcome names: the live one
	// (UNCHANGED, SAVED, MADE_LIVE) or the draft (DRAFT, DRAFT_EXISTS).
	Version int
}

// AwaitingPromote reports whether the save left the definition in a version
// that is not live.
func (r SaveResult) AwaitingPromote() bool {
	return r.Outcome == OutcomeDraft || r.Outcome == OutcomeDraftExists
}

// Message says what the save did, for a person.
func (r SaveResult) Message() string {
	n := strconv.Itoa(r.Version)
	switch r.Outcome {
	case OutcomeUnchanged:
		return "The definition is what the live version " + n + " says; nothing was saved."
	case OutcomeMadeLive:
		return "Version " + n + " already said this; it is now live and nothing new was saved."
	case OutcomeDraft:
		return "Saved as a draft (version " + n + "); someone with promote makes it live."
	case OutcomeDraftExists:
		return "Version " + n + ", a draft, already says this, so nothing was saved; someone with promote makes it live."
	case OutcomeSaved:
		return "Saved as version " + n + " and made live."
	}
	return "Version " + n + "."
}

// LiveIfPermitted is what a save that asks for a live version gets: live for
// a caller who holds platform:ingestion:ingestion:promote, otherwise a draft
// (owner ruling 2026-10-25: a save without promote becomes a draft, never a
// refusal). requested false is always a draft.
func LiveIfPermitted(ctx context.Context, requested bool) bool {
	return requested && auth.CanPromoteIngestions(auth.FromContext(ctx)) == nil
}

// reuseOrDraft is the part of a save that does not write a new live version.
// Called after the authoring checks, for a definition that is not the live
// version's, inside the operation's transaction s:
//
//   - a saved version already says def: live makes it live (MADE_LIVE, audited
//     as MakeVersionLiveCommand and emitting IngestionVersionLive, as the make
//     live call would); otherwise nothing is written (DRAFT_EXISTS);
//   - none does and the save is not live: def is saved as a draft (DRAFT,
//     audited as a SaveVersionCommand that is not live, emitting
//     IngestionVersionSaved), whichever call asked;
//   - none does and the save is live: done is false, and the caller saves the
//     new live version with its own event.
//
// The audit row names what was written, not the call that asked, so the
// history reads "saved as a draft" or "made live" for an update that became
// one.
func reuseOrDraft(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, repo *ingestion.Repository, i *ingestion.Ingestion,
	def ingestion.Definition, live bool, note *string, ec usecase.ExecutionContext,
) (res SaveResult, done bool, err error) {
	match, err := repo.FindVersionByDefinition(ctx, i.ID, def)
	if err != nil {
		return res, true, usecase.Internal("REPO", "find_version_by_definition failed", err)
	}
	if match != nil {
		switch {
		case match.Number == i.Version:
			return SaveResult{Ingestion: i, Outcome: OutcomeUnchanged, Version: i.Version}, true, nil
		case !live:
			return SaveResult{Ingestion: i, Outcome: OutcomeDraftExists, Version: match.Number}, true, nil
		}
		if err := i.MakeLive(*match); err != nil {
			return res, true, usecase.Internal("REPO", "make_live failed", err)
		}
		event := IngestionVersionLive{ingestionEvent: newEvent(ec, IngestionVersionLiveType, i.ID), Code: i.Code, Version: match.Number}
		if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, i, repo, event,
			MakeVersionLiveCommand{ID: i.ID, Version: match.Number})); err != nil {
			return res, true, err
		}
		return SaveResult{Ingestion: i, Outcome: OutcomeMadeLive, Version: match.Number}, true, nil
	}
	if live {
		return res, false, nil
	}
	v := i.SaveVersion(def, false, note, new(ec.PrincipalID))
	event := IngestionVersionSaved{ingestionEvent: newEvent(ec, IngestionVersionSavedType, i.ID), Code: i.Code, Version: v.Number}
	if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, i, repo, event,
		SaveVersionCommand{ID: i.ID, Definition: def, Note: note})); err != nil {
		return res, true, err
	}
	return SaveResult{Ingestion: i, Outcome: OutcomeDraft, Version: v.Number}, true, nil
}
