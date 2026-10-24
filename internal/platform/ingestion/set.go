package ingestion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
)

// MaxSetApplies is how many applies of a set are kept, with their archives;
// older ones are forgotten when a newer one is recorded.
const MaxSetApplies = 10

// setNamePattern is a set's name: lowercase letters, digits and '-', as a code.
var setNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// ValidSetName reports whether name can name a set.
func ValidSetName(name string) bool { return setNamePattern.MatchString(name) }

// Set is a named group of ingestions applied together from definition files.
// It owns the ingestions it applied (ManagedBy on each) until released.
type Set struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	// Ingestions is how many ingestions it owns; filled by ListSets only.
	Ingestions int
}

// SetAction is what an apply did, or would do, to one ingestion.
type SetAction string

const (
	// SetCreate: not there yet; created and owned by the set.
	SetCreate SetAction = "CREATE"
	// SetChanged: owned by the set and different; saved as a new live version.
	SetChanged SetAction = "CHANGE"
	// SetAdopt: there, made by hand; taken into the set, and changed if it differs.
	SetAdopt SetAction = "ADOPT"
	// SetUnchanged: owned by the set and the same; left as it is.
	SetUnchanged SetAction = "UNCHANGED"
	// SetRemove: owned by the set and no longer listed; deleted.
	SetRemove SetAction = "REMOVE"
)

// SetChange is what an apply did to one ingestion.
type SetChange struct {
	Application string    `json:"application"`
	Client      string    `json:"client"`
	Code        string    `json:"code"`
	Action      SetAction `json:"action" enum:"CREATE,CHANGE,ADOPT,UNCHANGED,REMOVE"`
	// IngestionID is absent for an ingestion a dry run would create.
	IngestionID *string `json:"ingestionId,omitempty"`
	// Changed names the fields an apply changed, for a change or an adoption.
	Changed []string `json:"changed"`
}

// SetApplySummary is what an apply records about itself, beside its archive.
type SetApplySummary struct {
	// Items is how many definitions it was given, Files how many files they
	// were read from.
	Items int `json:"items"`
	Files int `json:"files"`
	// GoingBackTo is the earlier apply whose definitions this one applied again.
	GoingBackTo *string     `json:"goingBackTo,omitempty"`
	Changes     []SetChange `json:"changes"`
}

// SetApply is one recorded apply of a set. An apply that changes nothing is
// not recorded.
type SetApply struct {
	ID    string
	SetID string
	// ArchiveDigest is the sha256 of the archive in the artifact store.
	ArchiveDigest string
	Summary       SetApplySummary
	AppliedBy     *string
	AppliedAt     time.Time
}

// SetFile is a file the definitions were read from, as it is in the
// repository (before environment values are replaced), kept for people to read.
type SetFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// SetArchive is what one apply was given, kept in the artifact store so an
// earlier apply can be applied again.
type SetArchive struct {
	Set         string    `json:"set"`
	ApplyID     string    `json:"applyId"`
	AppliedAt   time.Time `json:"appliedAt"`
	AppliedBy   string    `json:"appliedBy"`
	GoingBackTo *string   `json:"goingBackTo,omitempty"`
	Items       []File    `json:"items"`
	Files       []SetFile `json:"files"`
}

// Encode renders the archive and the digest it is stored under.
func (a SetArchive) Encode() (digest string, data []byte, err error) {
	data, err = json.Marshal(a)
	if err != nil {
		return "", nil, fmt.Errorf("set archive: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), data, nil
}

// ReadSetArchive reads the archive stored under digest;
// artifact.ErrNotFound when the store no longer holds it.
func ReadSetArchive(ctx context.Context, store artifact.Store, digest string) (*SetArchive, error) {
	rc, err := store.Open(ctx, digest)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("set archive: %w", err)
	}
	var a SetArchive
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("set archive %s does not parse: %w", digest, err)
	}
	return &a, nil
}

// ChangedFields names what differs between two definitions, in the order the
// fields are listed in a file. An apply records it for a change or an adoption.
func (d Definition) ChangedFields(other Definition) []string {
	var changed []string
	differs := func(a, b any) bool {
		x, errX := json.Marshal(a)
		y, errY := json.Marshal(b)
		return errX != nil || errY != nil || !bytes.Equal(x, y)
	}
	if d.Name != other.Name {
		changed = append(changed, "name")
	}
	if d.KeepFailedPayloads != other.KeepFailedPayloads {
		changed = append(changed, "keepFailedPayloads")
	}
	if differs(d.Source, other.Source) {
		changed = append(changed, "source")
	}
	if differs(d.Split, other.Split) {
		changed = append(changed, "split")
	}
	if differs(d.Target, other.Target) {
		changed = append(changed, "target")
	}
	return changed
}
