package function

import (
	"slices"
	"time"
)

// DefaultKeepVersions is how many UNTAGGED versions a function keeps when
// FC_FUNCTIONS_KEEP_VERSIONS is unset (docs/function-runner-plan.md §4,
// "Version pruning").
const DefaultKeepVersions = 20

// YoungPublishedGrace is how long a PUBLISHED version is exempt from pruning
// after it was created: it may be waiting for a runner to load it (and prove
// it READY) so it can be promoted.
const YoungPublishedGrace = time.Hour

// SelectPrunable picks the versions pruning deletes, oldest first.
//
// A version is TAGGED when an alias (live or any other) points at it. Tagged
// versions are never pruned and do not count toward keep. Every other
// version, whatever its status (READY, RETIRED, FAILED, PUBLISHED), is
// UNTAGGED and counts. When there are more than keep untagged versions the
// oldest are pruned until keep remain, except that two kinds of untagged
// version are protected (they still count toward keep, but are skipped):
//
//   - the highest-numbered version of the function: a publish's own result,
//     and the version a promote is most likely about to target;
//   - a PUBLISHED version younger than [YoungPublishedGrace]: a runner may
//     still be loading it.
//
// So when protected versions leave fewer than the excess prunable, fewer are
// pruned this time rather than a protected one being deleted. aliased holds
// the version ids aliases point at. keep < 1 selects nothing (callers
// validate it).
func SelectPrunable(versions []Version, aliased map[string]bool, keep int, now time.Time) []Version {
	if keep < 1 {
		return nil
	}
	var newest int32
	var untagged []Version
	for _, v := range versions {
		newest = max(newest, v.Number)
		if !aliased[v.ID] {
			untagged = append(untagged, v)
		}
	}
	excess := len(untagged) - keep
	if excess <= 0 {
		return nil
	}
	slices.SortFunc(untagged, func(a, b Version) int { return int(a.Number) - int(b.Number) })
	var out []Version
	for _, v := range untagged {
		if len(out) == excess {
			break
		}
		if v.Number == newest {
			continue
		}
		if v.Status == VersionPublished && now.Sub(v.CreatedAt) < YoungPublishedGrace {
			continue
		}
		out = append(out, v)
	}
	return out
}
