package operations

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// ApplySetCommand is the input DTO.
type ApplySetCommand struct {
	// Name is the set's name; the set is created by its first apply that
	// changes something.
	Name string `json:"name"`
	// Items are every ingestion the set should own, each with its application
	// code and its client's id or identifier. An empty list removes every
	// ingestion the set owns.
	Items []ingestion.File `json:"items"`
	// Files are the files the items were read from, kept with the apply.
	Files []ingestion.SetFile `json:"files,omitempty"`
	// DryRun does all the checks and reports what would be done, writing nothing.
	DryRun bool `json:"dryRun"`
	// GoingBackTo is the earlier apply whose archived items these are.
	GoingBackTo *string `json:"goingBackTo,omitempty"`
}

// ApplySetResult is what an apply did, or what a dry run would do.
type ApplySetResult struct {
	Set    string
	DryRun bool
	// ApplyID is the recorded apply; nil for a dry run and for an apply that
	// changed nothing, which is not recorded.
	ApplyID *string
	Changes []ingestion.SetChange
	// Forgotten are the archives of applies no longer kept, to remove from the
	// store once the transaction has committed.
	Forgotten []string
}

// setAuditCommand is the command the audit log records for an apply: the shape,
// not the definitions or the files, which the archive keeps.
type setAuditCommand struct {
	Name        string  `json:"name"`
	Items       int     `json:"items"`
	Files       int     `json:"files"`
	DryRun      bool    `json:"dryRun"`
	GoingBackTo *string `json:"goingBackTo,omitempty"`
}

// planned is one item resolved against what exists.
type planned struct {
	file     ingestion.File
	app      *application.Application
	client   *client.Client
	existing *ingestion.Ingestion
	change   ingestion.SetChange
}

// ApplySet makes the set's ingestions what its items say, in one transaction:
// creates what is new, saves a changed definition as a new live version, takes
// in an ingestion of the same application, client and code that was made by
// hand (changing it if it differs), and deletes what the set owns but no
// longer lists. An ingestion owned by another set stops the whole apply.
// Whatever fails, anything unresolvable included, nothing is written.
//
// An apply that changes something is recorded, with the files it was given in
// the artifact store, and emits [IngestionSetApplied] after the events of the
// ingestions it touched. The newest [ingestion.MaxSetApplies] applies are kept.
func ApplySet(repo *ingestion.Repository, resolver ingestion.Resolver, archive artifact.Store) usecaseop.TxOperation[ApplySetCommand, ApplySetResult] {
	return usecaseop.TxOperation[ApplySetCommand, ApplySetResult]{
		Name: "ApplyIngestionSet",
		Validate: func(_ context.Context, cmd ApplySetCommand) error {
			return validateApply(cmd).Err()
		},
		// Reach is checked per item in Execute, once its client and application
		// are resolved; the coarse "may apply sets" permission is the controller's.
		Authorize: usecaseop.Public[ApplySetCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd ApplySetCommand, ec usecase.ExecutionContext) (ApplySetResult, error) {
			var zero ApplySetResult
			if !cmd.DryRun {
				if err := s.WithTx(ctx, func(tx pgx.Tx) error { return repo.LockSets(ctx, tx) }); err != nil {
					return zero, usecase.Internal("REPO", "lock sets failed", err)
				}
			}
			set, err := repo.FindSetByName(ctx, cmd.Name)
			if err != nil {
				return zero, usecase.Internal("REPO", "find set failed", err)
			}
			items, err := planItems(ctx, repo, resolver, set, cmd)
			if err != nil {
				return zero, err
			}
			removals, err := planRemovals(ctx, repo, resolver, set, items)
			if err != nil {
				return zero, err
			}
			result := ApplySetResult{Set: cmd.Name, DryRun: cmd.DryRun}
			for _, p := range items {
				result.Changes = append(result.Changes, p.change)
			}
			for _, p := range removals {
				result.Changes = append(result.Changes, p.change)
			}
			if cmd.DryRun || !slices.ContainsFunc(result.Changes, func(c ingestion.SetChange) bool { return c.Action != ingestion.SetUnchanged }) {
				return result, nil
			}

			now := time.Now().UTC()
			if set == nil {
				set = &ingestion.Set{ID: tsid.Generate(tsid.IngestionSet), Name: cmd.Name, CreatedAt: now, UpdatedAt: now}
				err := s.WithTx(ctx, func(tx pgx.Tx) error { return repo.InsertSet(ctx, tx, *set) })
				if err != nil {
					return zero, usecase.Internal("REPO", "insert set failed", err)
				}
			}
			applyID := tsid.Generate(tsid.IngestionSetApply)
			audit := setAuditCommand{Name: cmd.Name, Items: len(cmd.Items), Files: len(cmd.Files), DryRun: false, GoingBackTo: cmd.GoingBackTo}
			for n, p := range items {
				if err := writeItem(ctx, s, repo, set.ID, applyID, &p, ec, audit); err != nil {
					return zero, err
				}
				result.Changes[n] = p.change
			}
			for _, p := range removals {
				event := IngestionDeleted{ingestionEvent: newEvent(ec, IngestionDeletedType, p.existing.ID), Code: p.existing.Code}
				if _, err := usecase.Into(usecasepgx.CommitDeleteScoped(ctx, s, p.existing, repo, event, audit)); err != nil {
					return zero, err
				}
			}

			applied := ingestion.SetArchive{
				Set: cmd.Name, ApplyID: applyID, AppliedAt: now, AppliedBy: ec.PrincipalID,
				GoingBackTo: cmd.GoingBackTo, Items: cmd.Items, Files: cmd.Files,
			}
			digest, data, err := applied.Encode()
			if err != nil {
				return zero, usecase.Internal("MARSHAL", "set archive does not render", err)
			}
			// Inside the transaction: when the archive cannot be kept, nothing is
			// applied. The store is content-addressed, so an archive left behind
			// by a failed transaction is garbage, never a wrong answer.
			if err := archive.Put(ctx, digest, bytes.NewReader(data)); err != nil {
				return zero, usecase.Internal("INGESTION_SET_ARCHIVE_UNAVAILABLE",
					"what the apply was given could not be kept in the set archive; nothing was applied", err)
			}
			record := ingestion.SetApply{
				ID: applyID, SetID: set.ID, ArchiveDigest: digest, AppliedBy: new(ec.PrincipalID), AppliedAt: now,
				Summary: ingestion.SetApplySummary{Items: len(cmd.Items), Files: len(cmd.Files), GoingBackTo: cmd.GoingBackTo, Changes: result.Changes},
			}
			err = s.WithTx(ctx, func(tx pgx.Tx) error {
				forgotten, err := repo.RecordApply(ctx, tx, record)
				result.Forgotten = forgotten
				if err != nil {
					return err
				}
				return repo.TouchSet(ctx, tx, set.ID, now)
			})
			if err != nil {
				return zero, usecase.Internal("REPO", "record apply failed", err)
			}
			result.ApplyID = &applyID

			event := IngestionSetApplied{
				setEvent: newSetEvent(ec, IngestionSetAppliedType, set.ID), Name: cmd.Name, ApplyID: applyID, GoingBackTo: cmd.GoingBackTo,
				Created: count(result.Changes, ingestion.SetCreate), Changed: count(result.Changes, ingestion.SetChanged),
				Adopted: count(result.Changes, ingestion.SetAdopt), Removed: count(result.Changes, ingestion.SetRemove),
			}
			if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s, event, audit)); err != nil {
				return zero, err
			}
			return result, nil
		},
	}
}

func count(changes []ingestion.SetChange, action ingestion.SetAction) int {
	return len(slices.DeleteFunc(slices.Clone(changes), func(c ingestion.SetChange) bool { return c.Action != action }))
}

// validateApply is the shape check: the name, and each item as a create would
// check it, with the item's index in the field path.
func validateApply(cmd ApplySetCommand) ingestion.Problems {
	var problems ingestion.Problems
	if !ingestion.ValidSetName(cmd.Name) {
		problems = append(problems, ingestion.FieldError{
			Field: "name", Code: "INVALID",
			Message: "must be lowercase letters, digits and '-', at most 64",
		})
	}
	for n, f := range cmd.Items {
		var item ingestion.Problems
		item = append(item, ingestion.ValidateCode(f.Code)...)
		item = append(item, f.Validate()...)
		if strings.TrimSpace(f.Application) == "" {
			item = append(item, ingestion.FieldError{Field: "application", Code: "NOT_NULL", Message: "is required"})
		}
		if strings.TrimSpace(f.Client) == "" {
			item = append(item, ingestion.FieldError{Field: "client", Code: "NOT_NULL", Message: "is required"})
		}
		for _, p := range item {
			p.Field = fmt.Sprintf("items[%d].%s", n, p.Field)
			problems = append(problems, p)
		}
	}
	return problems
}

// planItems resolves every item (its application, client, target and reach),
// finds what exists for it, and decides what the apply does. Nothing is
// written: a dry run is this and no more.
func planItems(ctx context.Context, repo *ingestion.Repository, resolver ingestion.Resolver, set *ingestion.Set, cmd ApplySetCommand) ([]planned, error) {
	ac := auth.FromContext(ctx)
	apps := map[string]*application.Application{}
	seen := map[string]int{}
	out := make([]planned, 0, len(cmd.Items))
	for n, f := range cmd.Items {
		at := func(err error) error { return inItem(n, err) }
		app, ok := apps[f.Application]
		if !ok {
			var err error
			if app, err = resolver.Application(ctx, strings.TrimSpace(f.Application)); err != nil {
				return nil, at(err)
			}
			apps[f.Application] = app
		}
		c, err := resolver.Client(ctx, clientRef(f.Client))
		if err != nil {
			return nil, at(err)
		}
		if err := auth.CheckScopeAccess(ac, new(string(c.ID))); err != nil {
			return nil, at(err)
		}
		if err := ingestion.CheckApplicationReach(ac, string(app.ID)); err != nil {
			return nil, at(err)
		}
		key := string(app.ID) + "/" + string(c.ID) + "/" + f.Code
		if first, dup := seen[key]; dup {
			return nil, usecase.Validation("INGESTION_INVALID",
				fmt.Sprintf("items[%d].code is also item %d's, for the same application and client", n, first))
		}
		seen[key] = n

		existing, err := repo.FindByCode(ctx, app.ID, c.ID, f.Code)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_by_code failed", err)
		}
		// What saving the file over what exists asks of the caller: the kind's
		// permission, the runtime identity, the secrets it points.
		// A source that names a config entry is judged on the effective
		// definition when the entry is there, and warned about when it is not.
		effective, err := checkAuthor(ctx, repo, existing, identity{Application: app.Code, Client: c.ID, Code: f.Code}, f.Definition)
		if err != nil {
			return nil, at(err)
		}
		if _, err := resolver.Target(ctx, app.Code, c.ID, effective.Definition.Target); err != nil {
			return nil, at(err)
		}
		p := planned{file: f, app: app, client: c, existing: existing}
		p.change = ingestion.SetChange{Application: app.Code, Client: c.Identifier, Code: f.Code, Changed: []string{}, Warnings: effective.Warnings()}
		if err := p.decide(ctx, repo, set); err != nil {
			return nil, at(err)
		}
		out = append(out, p)
	}
	return out, nil
}

// inItem says which item an error is about, keeping its kind and code so the
// caller still sees the right status.
func inItem(n int, err error) error {
	ue := usecase.AsError(err)
	if ue == nil {
		return err
	}
	at := *ue
	at.Message = fmt.Sprintf("items[%d]: %s", n, ue.Message)
	return &at
}

// clientRef is a client reference as the resolver finds it: a clt_ id as it
// is, an identifier in the lower case it is stored in.
func clientRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if _, err := ids.ParseClientID(ref); err != nil {
		return strings.ToLower(ref)
	}
	return ref
}

// decide sets the item's action from what exists.
func (p *planned) decide(ctx context.Context, repo *ingestion.Repository, set *ingestion.Set) error {
	if p.existing == nil {
		p.change.Action = ingestion.SetCreate
		return nil
	}
	i := p.existing
	p.change.IngestionID = &i.ID
	owned := i.ManagedBy != nil && set != nil && *i.ManagedBy == set.ID
	if i.ManagedBy != nil && !owned {
		name := *i.ManagedBy
		if names, err := repo.SetNames(ctx, []string{name}); err == nil && names[name] != "" {
			name = names[name]
		}
		return usecase.Conflict("INGESTION_SET_OWNED",
			fmt.Sprintf("ingestion %s is owned by set %s; release that set first", i.Code, name))
	}
	p.change.Changed = i.Definition.ChangedFields(p.file.Definition)
	// A file that does not say keeps whatever a pause or resume since made.
	if p.file.Active != nil && i.Active != *p.file.Active {
		p.change.Changed = append(p.change.Changed, "active")
	}
	// A draft (nothing live) is not what the file says even when it reads the same.
	if i.Version == 0 && len(p.change.Changed) == 0 {
		p.change.Changed = append(p.change.Changed, "live")
	}
	switch {
	case !owned:
		p.change.Action = ingestion.SetAdopt
	case len(p.change.Changed) == 0:
		p.change.Action = ingestion.SetUnchanged
	default:
		p.change.Action = ingestion.SetChanged
	}
	return nil
}

// createActive is whether the ingestion is created running. A file that says
// so decides; one that does not leaves a source that names a config entry
// paused, since its settings and secrets are set after the apply and resuming
// then checks they are there (owner ruling 2026-10-29).
func (p *planned) createActive() bool {
	if p.file.Active != nil {
		return *p.file.Active
	}
	return ingestion.ConfigKeyOf(p.file.Definition, p.file.Code) == ""
}

// planRemovals finds what the set owns but the items no longer list.
func planRemovals(ctx context.Context, repo *ingestion.Repository, resolver ingestion.Resolver, set *ingestion.Set, items []planned) ([]planned, error) {
	if set == nil {
		return nil, nil
	}
	owned, err := repo.ListManagedBy(ctx, set.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list managed failed", err)
	}
	ac := auth.FromContext(ctx)
	var out []planned
	for _, i := range owned {
		if slices.ContainsFunc(items, func(p planned) bool { return p.existing != nil && p.existing.ID == i.ID }) {
			continue
		}
		if err := ingestion.CheckReach(ac, i); err != nil {
			return nil, err
		}
		app, err := resolver.ApplicationByID(ctx, string(i.ApplicationID))
		if err != nil || app == nil {
			return nil, usecase.Internal("INGESTION_APPLICATION_MISSING", "the ingestion's application no longer exists", err)
		}
		c, err := resolver.Client(ctx, string(i.ClientID))
		if err != nil {
			return nil, err
		}
		out = append(out, planned{existing: i, change: ingestion.SetChange{
			Application: app.Code, Client: c.Identifier, Code: i.Code, Action: ingestion.SetRemove, IngestionID: &i.ID, Changed: []string{},
		}})
	}
	return out, nil
}

// writeItem commits one planned item with its event, and fills in the id of a
// created ingestion.
func writeItem(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, repo *ingestion.Repository, setID, applyID string, p *planned, ec usecase.ExecutionContext, audit setAuditCommand) error {
	by := new(ec.PrincipalID)
	switch p.change.Action {
	case ingestion.SetUnchanged:
		return nil
	case ingestion.SetCreate:
		i := ingestion.New(p.app.ID, p.client.ID, p.file.Code, p.file.Definition, true, nil, by)
		if !p.createActive() {
			i.Pause()
		}
		i.TakeInto(setID, applyID)
		event := IngestionCreated{
			ingestionEvent: newEvent(ec, IngestionCreatedType, i.ID),
			ApplicationID:  string(i.ApplicationID), ClientID: string(i.ClientID), Code: i.Code, Version: i.Version,
		}
		if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, i, repo, event, audit)); err != nil {
			return err
		}
		p.change.IngestionID = &i.ID
		return nil
	case ingestion.SetChanged, ingestion.SetAdopt, ingestion.SetRemove:
	}
	// Adopted or changed: the definition is saved as the next live version when
	// it differs from the live one; a paused flag or the owner alone writes none.
	i := p.existing
	if i.Version == 0 || len(i.Definition.ChangedFields(p.file.Definition)) > 0 {
		i.SaveVersion(p.file.Definition, true, nil, by)
	}
	if p.file.Active != nil {
		if *p.file.Active {
			i.Resume()
		} else {
			i.Pause()
		}
	}
	wasOwned := i.ManagedBy != nil
	i.TakeInto(setID, applyID)
	if len(p.change.Changed) == 0 && !wasOwned {
		event := IngestionManaged{ingestionEvent: newEvent(ec, IngestionManagedType, i.ID), Code: i.Code, ManagedBy: &setID}
		_, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, i, repo, event, audit))
		return err
	}
	event := IngestionUpdated{ingestionEvent: newEvent(ec, IngestionUpdatedType, i.ID), Code: i.Code, Version: i.Version}
	_, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, i, repo, event, audit))
	return err
}
