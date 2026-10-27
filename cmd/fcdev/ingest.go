package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

// This file is `fcdev ingest`: ingestion definitions as files, over the
// platform API (docs/ingestion-design.md section 5). Connection, profiles,
// --yes, --json and the prod confirmation are those of `fcdev fn`; an error
// is exit 1, as there.

// newIngestCmd is `fcdev ingest`.
func newIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Manage ingestions and their definition files",
	}
	cmd.AddCommand(
		newIngestListCmd(), newIngestGetCmd(), newIngestApplyCmd(), newIngestExportCmd(),
		newIngestVersionsCmd(), newIngestLiveCmd(), newIngestPauseCmd(), newIngestResumeCmd(),
		newIngestTestCmd(), newIngestConfigCmd(), newIngestSetsCmd(),
	)
	return cmd
}

// ingRow is the part of an ingestion the commands print.
type ingRow struct {
	ID               string          `json:"id"`
	ApplicationCode  string          `json:"applicationCode"`
	ClientID         string          `json:"clientId"`
	ClientIdentifier string          `json:"clientIdentifier"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	Version          int64           `json:"version"`
	LatestVersion    int64           `json:"latestVersion"`
	ManagedBy        string          `json:"managedBy"`
	Source           json.RawMessage `json:"source"`
	Split            json.RawMessage `json:"split"`
	Warnings         []string        `json:"warnings"`
	Target           struct {
		Code       string `json:"code"`
		TargetURL  string `json:"targetUrl"`
		Pool       string `json:"pool"`
		Connection string `json:"connection"`
		Account    string `json:"serviceAccount"`
	} `json:"target"`
	raw json.RawMessage
}

// ref is the name the commands take back: application/client/code.
func (r *ingRow) ref() string {
	return r.ApplicationCode + "/" + firstNonEmpty(r.ClientIdentifier, r.ClientID) + "/" + r.Code
}

// kindOf reads the kind of a source or split.
func kindOf(raw json.RawMessage) string {
	var k struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(raw, &k)
	return k.Kind
}

func decodeRow(raw json.RawMessage) (*ingRow, error) {
	var r ingRow
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	r.raw = raw
	return &r, nil
}

// listIngestions reads every ingestion matching the filters, following the
// cursor.
func (a *fnAPI) listIngestions(ctx context.Context, application, client, q string) ([]*ingRow, error) {
	var rows []*ingRow
	after := ""
	for {
		v := url.Values{"limit": {"200"}}
		for k, s := range map[string]string{"applicationCode": application, "clientId": client, "q": q, "after": after} {
			if s != "" {
				v.Set(k, s)
			}
		}
		var page struct {
			Ingestions []json.RawMessage `json:"ingestions"`
			HasMore    bool              `json:"hasMore"`
			NextCursor string            `json:"nextCursor"`
		}
		if err := a.json(ctx, http.MethodGet, "/api/ingestions?"+v.Encode(), nil, &page); err != nil {
			return nil, err
		}
		for _, raw := range page.Ingestions {
			r, err := decodeRow(raw)
			if err != nil {
				return nil, err
			}
			rows = append(rows, r)
		}
		if !page.HasMore || page.NextCursor == "" {
			return rows, nil
		}
		after = page.NextCursor
	}
}

// findIngestion resolves application/client/code, or an ing_ id, to its
// ingestion.
func (a *fnAPI) findIngestion(ctx context.Context, ref string) (*ingRow, error) {
	if strings.HasPrefix(ref, "ing_") && !strings.Contains(ref, "/") {
		var raw json.RawMessage
		if err := a.json(ctx, http.MethodGet, "/api/ingestions/"+url.PathEscape(ref), nil, &raw); err != nil {
			return nil, err
		}
		return decodeRow(raw)
	}
	parts := strings.Split(ref, "/")
	if len(parts) != 3 || slices.Contains(parts, "") {
		return nil, fmt.Errorf("%q is not an ingestion: write <application>/<client>/<code>", ref)
	}
	rows, err := a.listIngestions(ctx, parts[0], parts[1], parts[2])
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Code == parts[2] && r.ApplicationCode == parts[0] && (r.ClientIdentifier == parts[1] || r.ClientID == parts[1]) {
			return r, nil
		}
	}
	return nil, fmt.Errorf("no ingestion %s on %s", ref, a.target.banner())
}

func newIngestListCmd() *cobra.Command {
	var pf fnPlatformFlags
	var application, client, q string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ingestions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			rows, err := api.listIngestions(cmd.Context(), application, client, q)
			if err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			out := make([]json.RawMessage, len(rows))
			for i, r := range rows {
				out[i] = r.raw
				rep.say("%-48s %-7s %-12s v%d", r.ref(), r.Status, kindOf(r.Source), r.Version)
			}
			if len(rows) == 0 {
				rep.say("no ingestions")
			}
			return rep.emit(map[string]any{"ingestions": out})
		},
	}
	pf.add(cmd)
	cmd.Flags().StringVar(&application, "application", "", "only this application (code)")
	cmd.Flags().StringVar(&client, "client", "", "only this client (id or identifier)")
	cmd.Flags().StringVar(&q, "q", "", "search code and name")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON object on stdout; human output goes to stderr")
	return cmd
}

func newIngestGetCmd() *cobra.Command {
	var pf fnPlatformFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "get <application>/<client>/<code>",
		Short: "Show an ingestion",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			r, err := api.findIngestion(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			rep.say("%s (%s)", r.ref(), r.ID)
			rep.say("  name    %s", r.Name)
			rep.say("  status  %s, live v%d of %d", r.Status, r.Version, r.LatestVersion)
			rep.say("  source  %s", kindOf(r.Source))
			rep.say("  split   %s", kindOf(r.Split))
			rep.say("  target  %s", ingestTargetLine(r))
			if r.ManagedBy != "" {
				rep.say("  set     %s owns it (read-only until released)", r.ManagedBy)
			}
			printWarnings(rep, "  ", r.Warnings)
			return rep.emit(r.raw)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the ingestion as JSON on stdout; human output goes to stderr")
	return cmd
}

// ingestTargetLine renders where an ingestion's jobs go and what signs them.
func ingestTargetLine(r *ingRow) string {
	t := r.Target
	line := fmt.Sprintf("%s → %s (pool %s", t.Code, t.TargetURL, t.Pool)
	if t.Connection != "" {
		line += ", connection " + t.Connection
	}
	if t.Account != "" {
		line += ", service account " + t.Account
	}
	return line + ")"
}

// --- apply ----------------------------------------------------------------

// envPlaceholder finds the ${env:NAME} of the files this command used to read.
var envPlaceholder = regexp.MustCompile(`\$\{env:`)

// refuseEnv refuses a file that still holds ${env:NAME} outside comment lines.
// Environment variables are not read any more, and leaving the text in would
// send it to the platform as a literal, so it is an error, not a quiet pass.
func refuseEnv(text, file string) error {
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if envPlaceholder.MatchString(line) {
			return fmt.Errorf("%s line %d holds ${env:...}: environment variables are no longer read; put per-environment values in the ingestion's settings (`fcdev ingest config set`) and name them with `config:` in the source", file, i+1)
		}
	}
	return nil
}

// printWarnings prints what the platform says will not run as meant.
func printWarnings(rep *fnReport, indent string, warnings []string) {
	for _, w := range warnings {
		rep.say("%swarning: %s", indent, w)
	}
}

// setFile is a definition file as the API keeps it with an apply.
type setFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// readDefinitionFiles reads a folder (every .yaml and .yml under it, in name
// order) or the named files into definitions, and the files as they are.
func readDefinitionFiles(args []string) ([]ingestion.File, []setFile, error) {
	var paths, names []string
	folder := false
	if len(args) == 1 {
		if info, err := os.Stat(args[0]); err == nil && info.IsDir() {
			folder = true
			err := filepath.WalkDir(args[0], func(p string, d os.DirEntry, err error) error {
				if err == nil && d.Type().IsRegular() && isYAML(p) {
					rel, _ := filepath.Rel(args[0], p)
					paths, names = append(paths, p), append(names, filepath.ToSlash(rel))
				}
				return err
			})
			if err != nil {
				return nil, nil, err
			}
		}
	}
	if !folder {
		for _, a := range args {
			if info, err := os.Stat(a); err != nil || info.IsDir() {
				return nil, nil, fmt.Errorf("%s is not a definition file", a)
			}
			if slices.Contains(names, filepath.Base(a)) {
				return nil, nil, fmt.Errorf("two files are named %s", filepath.Base(a))
			}
			paths, names = append(paths, a), append(names, filepath.Base(a))
		}
	}
	var defs []ingestion.File
	var files []setFile
	for i, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, err
		}
		if err := refuseEnv(string(data), names[i]); err != nil {
			return nil, nil, err
		}
		parsed, err := ingestion.ParseFiles(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", names[i], err)
		}
		defs = append(defs, parsed...)
		files = append(files, setFile{Path: names[i], Content: string(data)})
	}
	return defs, files, nil
}

func isYAML(p string) bool { return strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml") }

// ingestChange is one line of a set's plan.
type ingestChange struct {
	Application string   `json:"application"`
	Client      string   `json:"client"`
	Code        string   `json:"code"`
	Action      string   `json:"action"`
	Changed     []string `json:"changed"`
	Warnings    []string `json:"warnings"`
}

type ingestPlan struct {
	Name    string         `json:"name"`
	DryRun  bool           `json:"dryRun"`
	ApplyID string         `json:"applyId"`
	Changes []ingestChange `json:"changes"`
}

// printPlan prints a set's plan: one line per ingestion, with the fields that
// change.
func printPlan(rep *fnReport, p ingestPlan, items int) {
	head := fmt.Sprintf("set %s applied", p.Name)
	switch {
	case p.DryRun:
		head = fmt.Sprintf("set %s, dry run: nothing was changed", p.Name)
	case p.ApplyID != "":
		head += " as apply " + p.ApplyID
	default:
		head += ", nothing changed"
	}
	if items > 0 {
		head += ", " + strconv.Itoa(items) + " definition(s)"
	}
	rep.say("%s", head)
	for _, c := range p.Changes {
		line := fmt.Sprintf("  %-9s %s/%s/%s", strings.ToLower(c.Action), c.Application, c.Client, c.Code)
		if len(c.Changed) > 0 {
			line += " (" + strings.Join(c.Changed, ", ") + ")"
		}
		rep.say("%s", line)
		printWarnings(rep, "    ", c.Warnings)
	}
}

func newIngestApplyCmd() *cobra.Command {
	var pf fnPlatformFlags
	var set string
	var dryRun, asJSON, yes bool
	cmd := &cobra.Command{
		Use:   "apply <folder | file...> [--set name]",
		Short: "Make a set's ingestions what its definition files say",
		Long: `Reads definition files (a folder's *.yaml and *.yml, or the files named) and
applies them as one set: ingestions the files list are created or changed,
ones made by hand that they list are taken into the set, and ones the set
owns but no longer lists are deleted. A file is the same in every environment:
what differs per environment (a vendor's login, the delivery URL) is the
ingestion's settings, named by "config:" in the source and written with
` + "`fcdev ingest config set`" + `. A file that still holds ${env:NAME} is refused.
A source naming settings that are not set yet is saved with a CONFIG_MISSING
warning, printed under its line; it runs once they are set.

The set is named by --set; for a folder it defaults to the folder's name.
--dry-run prints the plan and writes nothing.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defs, files, err := readDefinitionFiles(args)
			if err != nil {
				return err
			}
			if len(defs) == 0 {
				return errors.New("no definitions found: applying none would delete everything the set owns; use `fcdev ingest sets release` to let go of a set")
			}
			if set == "" {
				info, err := os.Stat(args[0])
				if len(args) != 1 || err != nil || !info.IsDir() {
					return errors.New("--set names the set the files belong to")
				}
				abs, err := filepath.Abs(args[0])
				if err != nil {
					return err
				}
				set = strings.ToLower(filepath.Base(abs))
			}
			items := make([]json.RawMessage, len(defs))
			for i, d := range defs {
				if items[i], err = json.Marshal(d); err != nil {
					return err
				}
			}
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			if !dryRun {
				if err := api.confirm("apply set "+set, yes); err != nil {
					return err
				}
			}
			return runSetPlan(cmd, api, asJSON, http.MethodPut, "/api/ingestion-sets/"+url.PathEscape(set), dryRun, len(items),
				map[string]any{"items": items, "files": files})
		},
	}
	pf.add(cmd)
	cmd.Flags().StringVar(&set, "set", "", "the set's name (a folder's name by default)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would change; writes nothing")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the plan as JSON on stdout; human output goes to stderr")
	addYesFlag(cmd, &yes)
	return cmd
}

// runSetPlan sends an apply or a reapply and prints its plan.
func runSetPlan(cmd *cobra.Command, api *fnAPI, asJSON bool, method, path string, dryRun bool, items int, body any) error {
	if dryRun {
		path += "?dryRun=true"
	}
	var raw json.RawMessage
	if err := api.json(cmd.Context(), method, path, body, &raw); err != nil {
		return err
	}
	var plan ingestPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return err
	}
	rep := newFnReport(cmd, asJSON)
	printPlan(rep, plan, items)
	return rep.emit(raw)
}

// --- export ---------------------------------------------------------------

func newIngestExportCmd() *cobra.Command {
	var pf fnPlatformFlags
	var all, asJSON bool
	var out, application, client string
	cmd := &cobra.Command{
		Use:   "export <application>/<client>/<code>... | --all",
		Short: "Write ingestions as definition files",
		Long: `Writes each ingestion as the definition file apply reads, to
<out>/<application>/<client>/<code>.yaml. Applying them as a set takes the
ingestions into it. --all exports every ingestion (narrowed by --application
and --client).`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return errors.New("name the ingestions to export, or pass --all")
			}
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			var rows []*ingRow
			if all {
				if rows, err = api.listIngestions(cmd.Context(), application, client, ""); err != nil {
					return err
				}
			}
			for _, ref := range args {
				r, err := api.findIngestion(cmd.Context(), ref)
				if err != nil {
					return err
				}
				rows = append(rows, r)
			}
			rep := newFnReport(cmd, asJSON)
			written := []string{}
			for _, r := range rows {
				elems := []string{r.ApplicationCode, firstNonEmpty(r.ClientIdentifier, r.ClientID), r.Code + ".yaml"}
				for _, e := range elems {
					if e == "" || e == ".yaml" || e == ".." || strings.ContainsAny(e, `/\`) {
						return fmt.Errorf("%s: cannot be written as a file name", r.ref())
					}
				}
				body, err := api.doRaw(cmd.Context(), http.MethodGet, "/api/ingestions/"+url.PathEscape(r.ID)+"/definition", nil, "")
				if err != nil {
					return fmt.Errorf("%s: %w", r.ref(), err)
				}
				file := filepath.Join(append([]string{out}, elems...)...)
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					return err
				}
				// #nosec G306 -- a definition file names its secrets, never holds one.
				if err := os.WriteFile(file, body, 0o644); err != nil {
					return err
				}
				written = append(written, file)
				rep.say("  %s", file)
			}
			rep.say("%d ingestion(s) written", len(written))
			return rep.emit(map[string]any{"written": written})
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&all, "all", false, "export every ingestion")
	cmd.Flags().StringVar(&out, "out", ".", "the folder to write to")
	cmd.Flags().StringVar(&application, "application", "", "with --all: only this application (code)")
	cmd.Flags().StringVar(&client, "client", "", "with --all: only this client (id or identifier)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the files written as JSON on stdout; human output goes to stderr")
	return cmd
}

// --- versions, live, pause, resume -----------------------------------------

func newIngestVersionsCmd() *cobra.Command {
	var pf fnPlatformFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "versions <application>/<client>/<code>",
		Short: "List an ingestion's saved versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			r, err := api.findIngestion(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := api.json(cmd.Context(), http.MethodGet, "/api/ingestions/"+r.ID+"/versions", nil, &raw); err != nil {
				return err
			}
			var list struct {
				Versions []struct {
					Version   int64  `json:"version"`
					Live      bool   `json:"live"`
					CreatedAt string `json:"createdAt"`
					CreatedBy string `json:"createdBy"`
					Note      string `json:"note"`
				} `json:"versions"`
			}
			if err := json.Unmarshal(raw, &list); err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			rep.say("%s", r.ref())
			for _, v := range list.Versions {
				mark := ""
				if v.Live {
					mark = "  ← live"
				}
				rep.say("  v%-4d %s  %s%s%s", v.Version, v.CreatedAt, v.CreatedBy, mark, strings.TrimRight("  "+v.Note, " "))
			}
			return rep.emit(raw)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the versions as JSON on stdout; human output goes to stderr")
	return cmd
}

func newIngestLiveCmd() *cobra.Command {
	var pf fnPlatformFlags
	var asJSON, yes bool
	cmd := &cobra.Command{
		Use:   "live <application>/<client>/<code> <version>",
		Short: "Make a saved version the one runtimes run",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil || n < 1 {
				return fmt.Errorf("%q is not a version number", args[1])
			}
			return ingestChangeState(cmd, &pf, args[0], asJSON, yes,
				fmt.Sprintf("make version %d of %s live", n, args[0]), "/versions/"+args[1]+"/live")
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the ingestion as JSON on stdout; human output goes to stderr")
	addYesFlag(cmd, &yes)
	return cmd
}

func newIngestPauseCmd() *cobra.Command {
	return newIngestToggleCmd("pause", "Stop an ingestion running")
}

func newIngestResumeCmd() *cobra.Command {
	return newIngestToggleCmd("resume", "Let a paused ingestion run again")
}

func newIngestToggleCmd(verb, short string) *cobra.Command {
	var pf fnPlatformFlags
	var asJSON, yes bool
	cmd := &cobra.Command{
		Use:   verb + " <application>/<client>/<code>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return ingestChangeState(cmd, &pf, args[0], asJSON, yes, verb+" "+args[0], "/"+verb)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the ingestion as JSON on stdout; human output goes to stderr")
	addYesFlag(cmd, &yes)
	return cmd
}

// ingestChangeState posts to an ingestion's pause, resume or live route and
// prints the ingestion it answers with.
func ingestChangeState(cmd *cobra.Command, pf *fnPlatformFlags, ref string, asJSON, yes bool, action, suffix string) error {
	api, err := newFnAPI(cmd, pf)
	if err != nil {
		return err
	}
	r, err := api.findIngestion(cmd.Context(), ref)
	if err != nil {
		return err
	}
	if err := api.confirm(action, yes); err != nil {
		return err
	}
	var raw json.RawMessage
	if err := api.json(cmd.Context(), http.MethodPost, "/api/ingestions/"+r.ID+suffix, nil, &raw); err != nil {
		return err
	}
	now, err := decodeRow(raw)
	if err != nil {
		return err
	}
	rep := newFnReport(cmd, asJSON)
	rep.say("%s: %s, live v%d", now.ref(), now.Status, now.Version)
	printWarnings(rep, "  ", now.Warnings)
	return rep.emit(raw)
}

// --- test -----------------------------------------------------------------

// ingestTestPoll is how often --wait asks for a test's result.
var ingestTestPoll = 2 * time.Second

type ingestTest struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"`
	Outcome string   `json:"outcome"`
	Steps   []string `json:"steps"`
	Records *int64   `json:"records"`
	Excerpt string   `json:"excerpt"`
	Notice  string   `json:"notice"`
}

func newIngestTestCmd() *cobra.Command {
	var pf fnPlatformFlags
	var wait, asJSON bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "test <application>/<client>/<code> | <file>",
		Short: "Try an ingestion's source from a runtime, dispatching nothing",
		Long: `Queues a test of a saved ingestion, or of a definition file that is not saved.
A runtime that carries the source's kind takes it in its next poll, so a
result takes up to a minute. --wait polls for it and prints what the runtime
did; the exit code is 1 when the source failed, was refused, is not supported
or no runtime answered in time.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if info, err := os.Stat(args[0]); err == nil && info.Mode().IsRegular() {
				defs, _, err := readDefinitionFiles(args)
				if err != nil {
					return err
				}
				if len(defs) != 1 {
					return fmt.Errorf("%s holds %d definitions: test one at a time", args[0], len(defs))
				}
				body["application"], body["client"], body["definition"] = defs[0].Application, defs[0].Client, defs[0].Definition
			}
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			if len(body) == 0 {
				r, err := api.findIngestion(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				body["ingestionId"] = r.ID
			}
			var raw json.RawMessage
			if err := api.json(cmd.Context(), http.MethodPost, "/api/ingestion-tests", body, &raw); err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			var t ingestTest
			if err := json.Unmarshal(raw, &t); err != nil {
				return err
			}
			rep.say("test %s queued", t.ID)
			if t.Notice != "" {
				rep.say("  %s", t.Notice)
			}
			if wait {
				if raw, t, err = waitIngestTest(cmd.Context(), api, t.ID, timeout); err != nil {
					return err
				}
			}
			if t.Status == "DONE" || t.Status == "EXPIRED" {
				printIngestTest(rep, t)
			}
			if err := rep.emit(raw); err != nil {
				return err
			}
			switch {
			case t.Status == "EXPIRED":
				return fmt.Errorf("test %s expired: no runtime answered", t.ID)
			case t.Status == "DONE" && (t.Outcome == "FAILED" || t.Outcome == "REFUSED" || t.Outcome == "NOT_SUPPORTED"):
				return fmt.Errorf("test %s: %s", t.ID, strings.ToLower(t.Outcome))
			}
			return nil
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for the result")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "with --wait: how long to wait")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the test as JSON on stdout; human output goes to stderr")
	return cmd
}

// waitIngestTest polls a test until a runtime has answered or it has expired.
func waitIngestTest(ctx context.Context, api *fnAPI, id string, timeout time.Duration) (json.RawMessage, ingestTest, error) {
	deadline := time.Now().Add(timeout)
	for {
		var raw json.RawMessage
		var t ingestTest
		if err := api.json(ctx, http.MethodGet, "/api/ingestion-tests/"+url.PathEscape(id), nil, &raw); err != nil {
			return nil, t, err
		}
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, t, err
		}
		if t.Status == "DONE" || t.Status == "EXPIRED" {
			return raw, t, nil
		}
		if time.Now().After(deadline) {
			return nil, t, fmt.Errorf("test %s is still %s after %s", id, t.Status, timeout)
		}
		select {
		case <-ctx.Done():
			return nil, t, ctx.Err()
		case <-time.After(ingestTestPoll):
		}
	}
}

func printIngestTest(rep *fnReport, t ingestTest) {
	if t.Status == "EXPIRED" {
		rep.say("test %s expired: no runtime answered", t.ID)
		return
	}
	line := fmt.Sprintf("test %s: %s", t.ID, t.Outcome)
	if t.Records != nil {
		line += fmt.Sprintf(", %d record(s)", *t.Records)
	}
	rep.say("%s", line)
	for _, s := range t.Steps {
		rep.say("  %s", s)
	}
	if t.Excerpt != "" {
		rep.say("  excerpt: %s", t.Excerpt)
	}
}

// --- sets -----------------------------------------------------------------

func newIngestSetsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sets",
		Short: "List, re-apply and release the sets that own ingestions",
	}
	cmd.AddCommand(newIngestSetsListCmd(), newIngestSetsAppliesCmd(), newIngestSetsReapplyCmd(), newIngestSetsReleaseCmd())
	return cmd
}

func newIngestSetsListCmd() *cobra.Command {
	var pf fnPlatformFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the sets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := api.json(cmd.Context(), http.MethodGet, "/api/ingestion-sets", nil, &raw); err != nil {
				return err
			}
			var list struct {
				Sets []struct {
					Name       string `json:"name"`
					Ingestions int64  `json:"ingestions"`
					UpdatedAt  string `json:"updatedAt"`
				} `json:"sets"`
			}
			if err := json.Unmarshal(raw, &list); err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			for _, s := range list.Sets {
				rep.say("%-32s %4d ingestion(s)  updated %s", s.Name, s.Ingestions, s.UpdatedAt)
			}
			if len(list.Sets) == 0 {
				rep.say("no sets")
			}
			return rep.emit(raw)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the sets as JSON on stdout; human output goes to stderr")
	return cmd
}

func newIngestSetsAppliesCmd() *cobra.Command {
	var pf fnPlatformFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "applies <set>",
		Short: "List a set's kept applies, newest first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := api.json(cmd.Context(), http.MethodGet, "/api/ingestion-sets/"+url.PathEscape(args[0])+"/applies", nil, &raw); err != nil {
				return err
			}
			var list struct {
				Applies []struct {
					ID          string         `json:"id"`
					AppliedAt   string         `json:"appliedAt"`
					AppliedBy   string         `json:"appliedBy"`
					Items       int64          `json:"items"`
					GoingBackTo string         `json:"goingBackTo"`
					Changes     []ingestChange `json:"changes"`
				} `json:"applies"`
			}
			if err := json.Unmarshal(raw, &list); err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			for _, a := range list.Applies {
				line := fmt.Sprintf("%s  %s  by %s  %d item(s), %d change(s)", a.ID, a.AppliedAt, a.AppliedBy, a.Items, len(a.Changes))
				if a.GoingBackTo != "" {
					line += ", going back to " + a.GoingBackTo
				}
				rep.say("%s", line)
			}
			if len(list.Applies) == 0 {
				rep.say("no applies")
			}
			return rep.emit(raw)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the applies as JSON on stdout; human output goes to stderr")
	return cmd
}

func newIngestSetsReapplyCmd() *cobra.Command {
	var pf fnPlatformFlags
	var dryRun, asJSON, yes bool
	cmd := &cobra.Command{
		Use:   "reapply <set> <apply-id>",
		Short: "Apply a set again with the items an earlier apply was given",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			if !dryRun {
				if err := api.confirm("reapply "+args[1]+" to set "+args[0], yes); err != nil {
					return err
				}
			}
			return runSetPlan(cmd, api, asJSON, http.MethodPost,
				"/api/ingestion-sets/"+url.PathEscape(args[0])+"/applies/"+url.PathEscape(args[1])+"/reapply", dryRun, 0, nil)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would change; writes nothing")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the plan as JSON on stdout; human output goes to stderr")
	addYesFlag(cmd, &yes)
	return cmd
}

func newIngestSetsReleaseCmd() *cobra.Command {
	var pf fnPlatformFlags
	var asJSON, yes bool
	cmd := &cobra.Command{
		Use:   "release <set>",
		Short: "Let go of a set: its ingestions stay and are edited one at a time again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			if err := api.confirm("release set "+args[0], yes); err != nil {
				return err
			}
			var raw json.RawMessage
			if err := api.json(cmd.Context(), http.MethodPost, "/api/ingestion-sets/"+url.PathEscape(args[0])+"/release", nil, &raw); err != nil {
				return err
			}
			var out struct {
				Released int64 `json:"released"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			rep.say("set %s released: %d ingestion(s) are no longer owned by it", args[0], out.Released)
			return rep.emit(raw)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON on stdout; human output goes to stderr")
	addYesFlag(cmd, &yes)
	return cmd
}

// --- config ---------------------------------------------------------------

// ingestConfigPath is the route of the settings entry an ingestion names.
func ingestConfigPath(id string) string { return "/api/ingestions/" + url.PathEscape(id) + "/config" }

func newIngestConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and write an ingestion's settings in this environment",
		Long: `The settings a source names with "config:" are platform config of the
environment (application, section ingest, at the ingestion's client), written
only through the ingestion. They are the part of a definition that differs per
environment: a vendor's login, an organisation id, the delivery URL.`,
	}
	cmd.AddCommand(newIngestConfigGetCmd(), newIngestConfigSetCmd(), newIngestConfigDeleteCmd())
	return cmd
}

type ingestConfigProblem struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ingestConfig struct {
	Key           string                `json:"key"`
	Status        string                `json:"status"`
	Problems      []ingestConfigProblem `json:"problems"`
	UpdatedAt     string                `json:"updatedAt"`
	SchemaChecked bool                  `json:"schemaChecked"`
	Value         json.RawMessage       `json:"value"`
	ValueHidden   bool                  `json:"valueHidden"`
	Ingestions    []string              `json:"ingestions"`
	Warnings      []string              `json:"warnings"`
}

func printConfig(rep *fnReport, c ingestConfig) {
	if c.Status == "INLINE" {
		rep.say("  status  INLINE: the settings are in the definition, not in an entry")
		return
	}
	rep.say("  key     %s", c.Key)
	rep.say("  status  %s", c.Status)
	if c.UpdatedAt != "" {
		rep.say("  updated %s", c.UpdatedAt)
	}
	for _, p := range c.Problems {
		rep.say("  problem %s: %s", p.Field, p.Message)
	}
	if !c.SchemaChecked {
		rep.say("  no live runtime reports this kind's settings: a value is not checked against them when written")
	}
	if c.ValueHidden {
		rep.say("  value   the value is hidden: it needs ingestion:update")
	}
	if len(c.Value) > 0 && string(c.Value) != "null" {
		var pretty bytes.Buffer
		if json.Indent(&pretty, c.Value, "  ", "  ") == nil {
			rep.say("  value   %s", pretty.String())
		}
	}
}

func newIngestConfigGetCmd() *cobra.Command {
	var pf fnPlatformFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "get <application>/<client>/<code>",
		Short: "Show the settings entry an ingestion's source names",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			r, err := api.findIngestion(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := api.json(cmd.Context(), http.MethodGet, ingestConfigPath(r.ID), nil, &raw); err != nil {
				return err
			}
			var c ingestConfig
			if err := json.Unmarshal(raw, &c); err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			rep.say("%s", r.ref())
			printConfig(rep, c)
			return rep.emit(raw)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the answer as JSON on stdout; human output goes to stderr")
	return cmd
}

func newIngestConfigSetCmd() *cobra.Command {
	var pf fnPlatformFlags
	var asJSON, yes bool
	cmd := &cobra.Command{
		Use:   "set <application>/<client>/<code> <file.json | ->",
		Short: "Write the settings entry an ingestion's source names",
		Long: `Writes the JSON object in the file (or on stdin, with -) as the entry: the
kind's settings as an inline source holds them, and optionally targetUrl. A
secret setting holds the secret's NAME; the secret itself is set in section
secrets at the client. The platform checks the value against every ingestion of
the application and client that names the entry, and writes nothing unless all
pass.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var data []byte
			var err error
			if args[1] == "-" {
				data, err = io.ReadAll(cmd.InOrStdin())
			} else {
				data, err = os.ReadFile(args[1])
			}
			if err != nil {
				return err
			}
			var value map[string]any
			if err := json.Unmarshal(data, &value); err != nil || value == nil {
				return fmt.Errorf("%s is not a JSON object: the entry is an object of settings", args[1])
			}
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			r, err := api.findIngestion(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := api.confirm("write the settings of "+r.ref(), yes); err != nil {
				return err
			}
			var raw json.RawMessage
			err = api.json(cmd.Context(), http.MethodPut, ingestConfigPath(r.ID), map[string]any{"value": value}, &raw)
			if err != nil {
				if ae, ok := errors.AsType[*apiError](err); ok {
					for _, p := range ae.problems() {
						fmt.Fprintf(cmd.ErrOrStderr(), "  problem %s: %s\n", p.Field, p.Message)
					}
				}
				return err
			}
			var c ingestConfig
			if err := json.Unmarshal(raw, &c); err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			rep.say("%s: settings written to %s, %s", r.ref(), c.Key, c.Status)
			if len(c.Ingestions) > 0 {
				rep.say("  checked against %s", strings.Join(c.Ingestions, ", "))
			}
			for _, p := range c.Problems {
				rep.say("  problem %s: %s", p.Field, p.Message)
			}
			printWarnings(rep, "  ", c.Warnings)
			return rep.emit(raw)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the answer as JSON on stdout; human output goes to stderr")
	addYesFlag(cmd, &yes)
	return cmd
}

func newIngestConfigDeleteCmd() *cobra.Command {
	var pf fnPlatformFlags
	var asJSON, yes bool
	cmd := &cobra.Command{
		Use:   "delete <application>/<client>/<code>",
		Short: "Delete the settings entry an ingestion's source names",
		Long: `Deletes the entry. Every ingestion of the application and client that names it
is then MISSING and stops running until the entry is set again.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := newFnAPI(cmd, &pf)
			if err != nil {
				return err
			}
			r, err := api.findIngestion(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := api.confirm("delete the settings of "+r.ref()+" (every ingestion naming them stops)", yes); err != nil {
				return err
			}
			var raw json.RawMessage
			if err := api.json(cmd.Context(), http.MethodDelete, ingestConfigPath(r.ID), nil, &raw); err != nil {
				return err
			}
			var out struct {
				Key        string   `json:"key"`
				Ingestions []string `json:"ingestions"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				return err
			}
			rep := newFnReport(cmd, asJSON)
			rep.say("%s: settings %s deleted", r.ref(), out.Key)
			if len(out.Ingestions) > 0 {
				rep.say("  stopped until they are set again: %s", strings.Join(out.Ingestions, ", "))
			}
			return rep.emit(raw)
		},
	}
	pf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the answer as JSON on stdout; human output goes to stderr")
	addYesFlag(cmd, &yes)
	return cmd
}
