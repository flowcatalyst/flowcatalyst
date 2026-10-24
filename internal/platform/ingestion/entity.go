// Package ingestion stores ingestions: the definitions of how data from
// outside (HTTP push, SFTP, S3, queues, polled tables and APIs, runtime
// extensions) becomes dispatch jobs. docs/ingestion-design.md is the design.
package ingestion

import (
	"errors"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Ingestion is the aggregate root: (application, client, code) plus the live
// definition. Every definition ever saved is a Version; Version here is the
// number of the live one (0 while nothing is live), LatestVersion the highest
// number saved. A saved version runs only once it is made live.
type Ingestion struct {
	ID            string            `json:"id"`
	ApplicationID ids.ApplicationID `json:"applicationId"`
	ClientID      ids.ClientID      `json:"clientId"`
	Code          string            `json:"code"`
	// Active is false while the ingestion is paused.
	Active        bool `json:"active"`
	Version       int  `json:"version"`
	LatestVersion int  `json:"latestVersion"`
	// Definition is the live version's, or the latest saved one while nothing
	// is live.
	Definition Definition `json:"definition"`
	// ManagedBy is the id of the set that owns the ingestion, nil when it is
	// edited by hand.
	ManagedBy *string   `json:"managedBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// What the row held when it was read: Persist updates only if both still
	// hold, so a concurrent save or promote is a conflict, not a lost update.
	loadedVersion, loadedLatest int
	loaded                      bool
	// Versions saved since the read, written with the row.
	pending []Version
}

// IDStr satisfies usecase.HasID.
func (i Ingestion) IDStr() string { return i.ID }

// Version is one saved definition of an ingestion.
type Version struct {
	ID          string     `json:"id"`
	IngestionID string     `json:"ingestionId"`
	Number      int        `json:"version"`
	Definition  Definition `json:"definition"`
	Note        *string    `json:"note,omitempty"`
	// AppliedFrom is the id of the set apply that produced it.
	AppliedFrom *string   `json:"appliedFrom,omitempty"`
	CreatedBy   *string   `json:"createdBy,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ErrNotSaved is returned when a version is asked for that the ingestion has
// not saved.
var ErrNotSaved = errors.New("ingestion has no such version")

// New constructs an ingestion with its first version saved. live makes that
// version the one runtimes receive; otherwise it waits to be made live.
func New(applicationID ids.ApplicationID, clientID ids.ClientID, code string, def Definition, live bool, note, by *string) *Ingestion {
	now := time.Now().UTC()
	i := &Ingestion{
		ID:            tsid.Generate(tsid.Ingestion),
		ApplicationID: applicationID,
		ClientID:      clientID,
		Code:          code,
		Active:        true,
		Definition:    def,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	i.SaveVersion(def, live, note, by)
	return i
}

// SaveVersion records def as the next version and returns it. When live, it
// also becomes what the ingestion runs; otherwise it is kept for later, except
// while nothing is live, when it is the draft the row shows.
func (i *Ingestion) SaveVersion(def Definition, live bool, note, by *string) Version {
	now := time.Now().UTC()
	i.LatestVersion++
	v := Version{
		ID:          tsid.Generate(tsid.IngestionVersion),
		IngestionID: i.ID,
		Number:      i.LatestVersion,
		Definition:  def,
		Note:        note,
		CreatedBy:   by,
		CreatedAt:   now,
	}
	i.pending = append(i.pending, v)
	if live {
		i.Version = v.Number
	}
	if live || i.Version == 0 {
		i.Definition = def
	}
	i.UpdatedAt = now
	return v
}

// MakeLive moves the live pointer to v, a saved version of this ingestion.
func (i *Ingestion) MakeLive(v Version) error {
	if v.IngestionID != i.ID || v.Number < 1 || v.Number > i.LatestVersion {
		return ErrNotSaved
	}
	i.Version = v.Number
	i.Definition = v.Definition
	i.UpdatedAt = time.Now().UTC()
	return nil
}

// TakeInto makes the set the ingestion's owner, and stamps the versions saved
// since the read as produced by the set apply. Taking the ingestion into the
// set it is already in changes nothing.
func (i *Ingestion) TakeInto(setID, applyID string) {
	i.ManagedBy = &setID
	for n := range i.pending {
		i.pending[n].AppliedFrom = &applyID
	}
	i.UpdatedAt = time.Now().UTC()
}

// Release ends the set's ownership; the definition stays as it is.
func (i *Ingestion) Release() {
	i.ManagedBy = nil
	i.UpdatedAt = time.Now().UTC()
}

// Pause stops the ingestion running without changing its definition.
func (i *Ingestion) Pause() {
	i.Active = false
	i.UpdatedAt = time.Now().UTC()
}

// Resume undoes Pause.
func (i *Ingestion) Resume() {
	i.Active = true
	i.UpdatedAt = time.Now().UTC()
}

// Live reports whether runtimes receive the ingestion: active, with a live
// version.
func (i *Ingestion) Live() bool { return i.Active && i.Version > 0 }
