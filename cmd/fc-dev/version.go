package main

import (
	_ "embed"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// versionRaw is the released fc-dev version, embedded from the committed
// cmd/fc-dev/VERSION file at build time. `make release-dev BUMP=…` bumps that
// file and tags fc-dev/vX.Y.Z; the release workflow checks out the tag, so a
// release binary self-reports exactly the tagged version. A local `make build`
// reports whatever VERSION currently holds (the last released number until the
// next release bumps it).
//
// VERSION is seeded from the Rust monorepo's last fc-dev release (0.4.16) so
// this repo's numbering continues the sequence rather than restarting.
//
//go:embed VERSION
var versionRaw string

// version returns the trimmed semver string (the file carries a trailing newline).
func version() string { return strings.TrimSpace(versionRaw) }

// newVersionCmd prints the version plus the VCS revision Go embeds into the
// build — no ldflags plumbing needed: `go build` records vcs.revision and
// vcs.modified in debug.BuildInfo when built inside a git tree.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the fc-dev version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "fc-dev %s%s\n", version(), vcsSuffix())
			return nil
		},
	}
}

// vcsSuffix renders " (<rev>[-dirty])" from the build-embedded VCS info, or ""
// when the binary wasn't built inside a git tree (e.g. `go install` of a zip).
func vcsSuffix() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	rev, dirty := "", ""
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 12 {
				rev = rev[:12]
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return ""
	}
	return fmt.Sprintf(" (%s%s)", rev, dirty)
}
