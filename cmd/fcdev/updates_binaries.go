package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/apk"
)

// `fcdev updates binaries` and `fcdev updates links` (docs/app-updates-design.md
// section 14.8): a pipeline publishes the Android build of a phone app; an
// operator makes the link people install it from.

// exitError ends fcdev with Code; Err, when set, is reported first.
type exitError struct {
	Code int
	Err  error
}

func (e *exitError) Error() string {
	if e.Err == nil {
		return "exit status " + strconv.Itoa(e.Code)
	}
	return e.Err.Error()
}

func (e *exitError) Unwrap() error { return e.Err }

func newUpdatesBinariesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "binaries",
		Short: "Publish and list the Android binaries (APKs) of a phone app",
	}
	cmd.AddCommand(newBinariesInspectCmd(), newBinariesPublishCmd(), newBinariesListCmd(), newBinariesHasCmd())
	return cmd
}

func newUpdatesLinksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "links",
		Short: "Create, replace and remove the install links people download a phone app's binaries from (operators)",
	}
	cmd.AddCommand(newLinksCreateCmd(), newLinksReplaceCmd(), newLinksRemoveCmd(), newLinksListCmd())
	return cmd
}

// ── inspect ────────────────────────────────────────────────────────────────

// inspected is what `binaries inspect` prints and `binaries publish` checks.
type inspected struct {
	File               string            `json:"file"`
	SHA256             string            `json:"sha256"`
	SizeBytes          int64             `json:"sizeBytes"`
	PackageName        string            `json:"packageName"`
	VersionCode        int64             `json:"versionCode"`
	VersionName        string            `json:"versionName,omitempty"`
	MinSDK             int               `json:"minSdk"`
	Debuggable         bool              `json:"debuggable"`
	RuntimeVersion     string            `json:"runtimeVersion,omitempty"`
	RuntimeFrom        string            `json:"runtimeVersionSource,omitempty"`
	UpdateURL          string            `json:"updateUrl,omitempty"`
	UpdateHeaders      map[string]string `json:"updateRequestHeaders,omitempty"`
	Channel            string            `json:"channel,omitempty"`
	UpdateKeyID        string            `json:"updateKeyId,omitempty"`
	SigningCertSHA256  string            `json:"signingCertificateSha256"`
	SigningCertSubject string            `json:"signingCertificateSubject"`
	DebugCertificate   bool              `json:"debugCertificate"`
	SigningSchemes     []int             `json:"signingSchemes"`
}

// inspectAPK parses the APK at path (refusing one that is not a well-formed,
// signed APK) and hashes it.
func inspectAPK(path string) (*inspected, error) {
	info, err := apk.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return nil, err
	}
	return &inspected{
		File: path, SHA256: hex.EncodeToString(h.Sum(nil)), SizeBytes: n,
		PackageName: info.Package, VersionCode: info.VersionCode, VersionName: info.VersionName, MinSDK: info.MinSDK,
		Debuggable: info.Debuggable, RuntimeVersion: info.RuntimeVersion, RuntimeFrom: info.RuntimeVersionSource,
		UpdateURL: info.UpdateURL, UpdateHeaders: info.UpdateRequestHeaders, Channel: info.Channel(),
		UpdateKeyID: info.UpdateCodeSigningKeyID, SigningCertSHA256: info.Signing.CertificateSHA256,
		SigningCertSubject: info.Signing.CertificateSubject, DebugCertificate: info.Signing.DebugCertificate,
		SigningSchemes: info.Signing.Schemes,
	}, nil
}

func (i *inspected) print(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(tw, "%s\t%s\n", k, v)
		}
	}
	row("file", i.File)
	row("sha256", i.SHA256)
	row("size", strconv.FormatInt(i.SizeBytes, 10)+" bytes")
	row("package", i.PackageName)
	row("version", strings.TrimSpace(i.VersionName+" ("+strconv.FormatInt(i.VersionCode, 10)+")"))
	row("min sdk", strconv.Itoa(i.MinSDK))
	row("debuggable", strconv.FormatBool(i.Debuggable))
	rv := i.RuntimeVersion
	if rv != "" && i.RuntimeFrom != "" {
		rv += " (from " + i.RuntimeFrom + ")"
	}
	row("runtime version", firstNonEmpty(rv, "none (the app does not use expo-updates)"))
	row("update url", i.UpdateURL)
	row("channel", i.Channel)
	row("update key id", i.UpdateKeyID)
	row("signed by", i.SigningCertSubject)
	row("certificate sha256", i.SigningCertSHA256)
	schemes := make([]string, 0, len(i.SigningSchemes))
	for _, s := range i.SigningSchemes {
		schemes = append(schemes, "v"+strconv.Itoa(s))
	}
	row("signature schemes", strings.Join(schemes, ", "))
	if i.DebugCertificate {
		row("note", "signed with an Android debug keystore: publish it as the debug variant only")
	}
	_ = tw.Flush()
}

func newBinariesInspectCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "inspect <apk>",
		Short: "Show what an APK says about itself (offline): package, version, runtime version, channel, signing certificate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			i, err := inspectAPK(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(i)
			}
			i.print(cmd.OutOrStdout())
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print as JSON")
	return cmd
}

// ── publish ────────────────────────────────────────────────────────────────

// binaryRow is the API's binary answer, as fcdev prints it.
type binaryRow struct {
	ID        string `json:"id"`
	AppKey    string `json:"appKey"`
	Channel   string `json:"channel"`
	Variant   string `json:"variant"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
	FileName  string `json:"fileName"`
	Newest    bool   `json:"newest"`
	CreatedAt string `json:"createdAt"`
	Facts     struct {
		PackageName    string  `json:"packageName"`
		VersionCode    int64   `json:"versionCode"`
		VersionName    *string `json:"versionName"`
		RuntimeVersion *string `json:"runtimeVersion"`
	} `json:"facts"`
	Message *string `json:"message"`
}

func (b binaryRow) version() string {
	if b.Facts.VersionName == nil {
		return strconv.FormatInt(b.Facts.VersionCode, 10)
	}
	return *b.Facts.VersionName + " (" + strconv.FormatInt(b.Facts.VersionCode, 10) + ")"
}

// resolvePublishFlags fills what the APK says (application, app key and
// channel from its update URL and channel header; the variant from whether it
// is debuggable) and refuses flags the APK contradicts.
func resolvePublishFlags(i *inspected, app, appKey, channel, variant *string) error {
	if code, key, ok := appupdate.UpdateURLNames(i.UpdateURL); ok {
		if *app == "" {
			*app = code
		}
		if *appKey == "" {
			*appKey = key
		}
		if *app != code || *appKey != key {
			return fmt.Errorf("the APK's update URL names application %q and app key %q, not --app %q --app-key %q", code, key, *app, *appKey)
		}
	}
	if i.Channel != "" {
		if *channel == "" {
			*channel = i.Channel
		}
		if *channel != i.Channel {
			return fmt.Errorf("the APK asks for updates of channel %q (expo-channel-name), not --channel %q", i.Channel, *channel)
		}
	}
	if *variant == "" {
		*variant = appupdate.VariantRelease
		if i.Debuggable || i.DebugCertificate {
			*variant = appupdate.VariantDebug
		}
	}
	if err := checkBranchFlags(*app, *appKey, *channel); err != nil {
		return err
	}
	if !appupdate.ValidVariant(*variant) {
		return errors.New("--variant must be release, debug or another 1-20 lower-case letters, digits and hyphens")
	}
	if *variant == appupdate.VariantRelease && i.Debuggable {
		return errors.New("the APK is debuggable: real users get the release variant; build the release variant, or pass --variant debug")
	}
	if *variant == appupdate.VariantRelease && i.DebugCertificate {
		return errors.New("the APK is signed with an Android debug keystore, whose key is public: sign the release build with the app's release key, or pass --variant debug")
	}
	return nil
}

func newBinariesPublishCmd() *cobra.Command {
	var f fnPlatformFlags
	var app, appKey, channel, variant, message, commit string
	var jsonOut, yes bool
	cmd := &cobra.Command{
		Use:   "publish <apk>",
		Short: "Upload an APK and publish it to a phone app's channel as the release or debug build",
		Long: `Reads the APK (it must be a well-formed APK signed with APK Signature Scheme v2
or v3), uploads it unless the application has it already, and publishes it to
the channel in a variant. An install link of the channel gives the newest
(highest version code) binary of each variant.

What the APK says is used, not flags: --app and --app-key default to the
application and app key its update URL names, --channel to its
expo-channel-name, and --variant to debug for a debuggable APK or one signed
with a debug keystore, else release; a flag that contradicts the APK is
refused. The release variant (the build real users get) must be neither.
Publishing the same file to the same place again changes nothing.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			i, err := inspectAPK(args[0])
			if err != nil {
				return err
			}
			if err := resolvePublishFlags(i, &app, &appKey, &channel, &variant); err != nil {
				return err
			}
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			a.http.Timeout = 45 * time.Minute
			if err := a.confirm(fmt.Sprintf("publish %s %s of %s/%s to channel %s", variant, i.VersionName, app, appKey, channel), yes); err != nil {
				return err
			}
			if err := uploadBinary(ctx, a, app, i, cmd.ErrOrStderr()); err != nil {
				return err
			}
			body := map[string]any{
				"applicationCode": app, "appKey": appKey, "platform": appupdate.PlatformAndroid,
				"channel": channel, "variant": variant, "sha256": i.SHA256,
			}
			if message != "" {
				body["message"] = message
			}
			if commit != "" {
				body["sourceCommit"] = commit
			}
			raw, err := a.doRaw(ctx, http.MethodPost, "/api/app-updates/binaries", jsonBody(body), "application/json")
			if err != nil {
				return err
			}
			if jsonOut {
				_, err := cmd.OutOrStdout().Write(raw)
				return err
			}
			var res struct {
				Binary  binaryRow `json:"binary"`
				Created bool      `json:"created"`
			}
			if err := json.Unmarshal(raw, &res); err != nil {
				return err
			}
			verb := "published"
			if !res.Created {
				verb = "already published"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s of %s/%s to channel %s: %s (%s)\n", verb, res.Binary.Variant, res.Binary.version(),
				app, appKey, res.Binary.Channel, res.Binary.ID, res.Binary.FileName)
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().StringVar(&app, "app", "", "the application's code (default: the one the APK's update URL names)")
	cmd.Flags().StringVar(&appKey, "app-key", "", "the phone app's key (default: the one the APK's update URL names)")
	cmd.Flags().StringVar(&channel, "channel", "", "the channel (default: the APK's expo-channel-name)")
	cmd.Flags().StringVar(&variant, "variant", "", "release or debug (default: debug for a debuggable or debug-signed APK, else release)")
	cmd.Flags().StringVar(&message, "message", "", "what changed")
	cmd.Flags().StringVar(&commit, "commit", "", "the source commit")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the API's answer")
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
	return cmd
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return strings.NewReader(string(b))
}

// uploadBinary uploads the APK unless the application has stored it.
func uploadBinary(ctx context.Context, a *fnAPI, app string, i *inspected, log io.Writer) error {
	path := "/api/app-updates/binary-files/" + url.PathEscape(app) + "/" + i.SHA256
	_, err := a.doRaw(ctx, http.MethodGet, path, nil, "")
	if err == nil {
		fmt.Fprintf(log, "the platform has %s already\n", short(i.SHA256))
		return nil
	}
	if ae, ok := errors.AsType[*apiError](err); !ok || ae.Status != http.StatusNotFound {
		return fmt.Errorf("ask whether the binary is stored: %w", err)
	}
	fh, err := os.Open(i.File)
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	fmt.Fprintf(log, "uploading %s (%d bytes)\n", short(i.SHA256), i.SizeBytes)
	if _, err := a.doRaw(ctx, http.MethodPut, path, fh, "application/octet-stream"); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return nil
}

// ── list / has ─────────────────────────────────────────────────────────────

func newBinariesListCmd() *cobra.Command {
	var f fnPlatformFlags
	var app, appKey, channel, variant, rv string
	var size int
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List an application's binaries, newest version first (NEWEST marks what an install link gives)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !appupdate.ValidApplicationCode(app) {
				return errors.New("--app must be an application code")
			}
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			q := url.Values{"applicationCode": {app}, "size": {strconv.Itoa(size)}}
			for k, v := range map[string]string{"appKey": appKey, "channel": channel, "variant": variant, "runtimeVersion": rv} {
				if v != "" {
					q.Set(k, v)
				}
			}
			raw, err := a.doRaw(cmd.Context(), http.MethodGet, "/api/app-updates/binaries?"+q.Encode(), nil, "")
			if err != nil {
				return err
			}
			if jsonOut {
				_, err := cmd.OutOrStdout().Write(raw)
				return err
			}
			var page struct {
				Data  []binaryRow `json:"data"`
				Total int64       `json:"total"`
			}
			if err := json.Unmarshal(raw, &page); err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tAPP KEY\tCHANNEL\tVARIANT\tVERSION\tRUNTIME\tSIZE\tPUBLISHED\tNEWEST")
			for _, b := range page.Data {
				newest := ""
				if b.Newest {
					newest = "yes"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n", b.ID, b.AppKey, b.Channel, b.Variant, b.version(),
					deref(b.Facts.RuntimeVersion), b.SizeBytes, b.CreatedAt, newest)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%d of %d\n", len(page.Data), page.Total)
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().StringVar(&app, "app", "", "the application's code (required)")
	cmd.Flags().StringVar(&appKey, "app-key", "", "only this phone app")
	cmd.Flags().StringVar(&channel, "channel", "", "only this channel")
	cmd.Flags().StringVar(&variant, "variant", "", "only this variant")
	cmd.Flags().StringVar(&rv, "runtime-version", "", "only this runtime version")
	cmd.Flags().IntVar(&size, "size", 50, "how many (at most 1000)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the API's answer")
	return cmd
}

func newBinariesHasCmd() *cobra.Command {
	var f fnPlatformFlags
	var app, appKey, channel, platform, rv string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "has",
		Short: "Whether a binary of a runtime version is published on a channel: exit 0 yes, 1 no, 2 the question failed",
		Long: `For an update pipeline: before publishing an update for a runtime version, ask
whether any phone installed from this platform runs it.

  if fcdev updates binaries has --app acme --app-key driver-app --channel production --runtime-version "$RV"; then ...

Exit status 0: a binary (any variant) of the runtime version is published on
the channel; 1: none is; 2: the question could not be answered.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fail := func(err error) error { return &exitError{Code: 2, Err: err} }
			if err := checkBranchFlags(app, appKey, channel); err != nil {
				return fail(err)
			}
			if !appupdate.ValidPlatform(platform) {
				return fail(errors.New("--os must be android or ios"))
			}
			if !appupdate.ValidRuntimeVersion(rv) {
				return fail(errors.New("--runtime-version is required"))
			}
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return fail(err)
			}
			q := url.Values{"applicationCode": {app}, "appKey": {appKey}, "platform": {platform}, "channel": {channel}, "runtimeVersion": {rv}}
			raw, err := a.doRaw(cmd.Context(), http.MethodGet, "/api/app-updates/binary-check?"+q.Encode(), nil, "")
			if err != nil {
				return fail(err)
			}
			var res struct {
				Published bool       `json:"published"`
				Count     int64      `json:"count"`
				Newest    *binaryRow `json:"newest"`
			}
			if err := json.Unmarshal(raw, &res); err != nil {
				return fail(err)
			}
			switch {
			case jsonOut:
				_, _ = cmd.OutOrStdout().Write(raw)
			case res.Published && res.Newest != nil:
				fmt.Fprintf(cmd.OutOrStdout(), "yes: %d binaries of runtime %s on %s/%s/%s; newest %s %s (%s)\n",
					res.Count, rv, appKey, platform, channel, res.Newest.Variant, res.Newest.version(), res.Newest.ID)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "no: no binary of runtime %s is published on %s/%s/%s\n", rv, appKey, platform, channel)
			}
			if !res.Published {
				return &exitError{Code: 1}
			}
			return nil
		},
	}
	f.add(cmd)
	addBranchFlags(cmd, &app, &appKey, &channel)
	cmd.Flags().StringVar(&platform, "os", appupdate.PlatformAndroid, "android or ios")
	cmd.Flags().StringVar(&rv, "runtime-version", "", "the runtime version (required)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the API's answer")
	cmd.SilenceUsage = true
	return cmd
}

// ── links ──────────────────────────────────────────────────────────────────

type linkRow struct {
	ID         string `json:"id"`
	AppKey     string `json:"appKey"`
	Channel    string `json:"channel"`
	Generation int    `json:"generation"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type linkWithToken struct {
	Link linkRow `json:"link"`
	URL  string  `json:"url"`
}

func printLink(cmd *cobra.Command, raw []byte, jsonOut bool, what string) error {
	if jsonOut {
		_, err := cmd.OutOrStdout().Write(raw)
		return err
	}
	var res linkWithToken
	if err := json.Unmarshal(raw, &res); err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s %s (%s/%s, generation %d); the address is shown only now:\n", what, res.Link.ID, res.Link.AppKey, res.Link.Channel, res.Link.Generation)
	fmt.Fprintln(cmd.OutOrStdout(), res.URL)
	return nil
}

func newLinksCreateCmd() *cobra.Command {
	var f fnPlatformFlags
	var app, appKey, channel string
	var jsonOut, yes bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create the install link of a phone app's channel and print its address (shown once)",
		Long: `Creates the link through which anyone holding it downloads the newest Android
build of each variant of the channel. The address carries a 256-bit token the
platform keeps only as a hash: it is printed once. Replace the link to cut old
copies off. Needs the install-link permission (platform:app-update-admin).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkBranchFlags(app, appKey, channel); err != nil {
				return err
			}
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			if err := a.confirm("create an install link for "+app+"/"+appKey+" channel "+channel, yes); err != nil {
				return err
			}
			raw, err := a.doRaw(cmd.Context(), http.MethodPost, "/api/app-updates/install-links",
				jsonBody(map[string]string{"applicationCode": app, "appKey": appKey, "channel": channel}), "application/json")
			if err != nil {
				return err
			}
			return printLink(cmd, raw, jsonOut, "created")
		},
	}
	f.add(cmd)
	addBranchFlags(cmd, &app, &appKey, &channel)
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the API's answer (with the address and token)")
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
	return cmd
}

func newLinksReplaceCmd() *cobra.Command {
	var f fnPlatformFlags
	var jsonOut, yes bool
	cmd := &cobra.Command{
		Use:   "replace <linkId>",
		Short: "Give an install link a new address; the old one stops working at once",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			if err := a.confirm("replace install link "+args[0]+" (its current address stops working)", yes); err != nil {
				return err
			}
			raw, err := a.doRaw(cmd.Context(), http.MethodPost, "/api/app-updates/install-links/"+url.PathEscape(args[0])+"/replace", nil, "")
			if err != nil {
				return err
			}
			return printLink(cmd, raw, jsonOut, "replaced")
		},
	}
	f.add(cmd)
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the API's answer (with the address and token)")
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
	return cmd
}

func newLinksRemoveCmd() *cobra.Command {
	var f fnPlatformFlags
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <linkId>",
		Short: "Remove an install link; its address stops working at once",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			if err := a.confirm("remove install link "+args[0], yes); err != nil {
				return err
			}
			if _, err := a.doRaw(cmd.Context(), http.MethodDelete, "/api/app-updates/install-links/"+url.PathEscape(args[0]), nil, ""); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", args[0])
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
	return cmd
}

func newLinksListCmd() *cobra.Command {
	var f fnPlatformFlags
	var app, appKey string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List an application's install links (their addresses are never shown again)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !appupdate.ValidApplicationCode(app) {
				return errors.New("--app must be an application code")
			}
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			q := url.Values{"applicationCode": {app}}
			if appKey != "" {
				q.Set("appKey", appKey)
			}
			raw, err := a.doRaw(cmd.Context(), http.MethodGet, "/api/app-updates/install-links?"+q.Encode(), nil, "")
			if err != nil {
				return err
			}
			if jsonOut {
				_, err := cmd.OutOrStdout().Write(raw)
				return err
			}
			var res struct {
				Links []linkRow `json:"links"`
			}
			if err := json.Unmarshal(raw, &res); err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tAPP KEY\tCHANNEL\tGENERATION\tCREATED\tREPLACED")
			for _, l := range res.Links {
				replaced := "-"
				if l.Generation > 1 {
					replaced = l.UpdatedAt
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", l.ID, l.AppKey, l.Channel, l.Generation, l.CreatedAt, replaced)
			}
			return tw.Flush()
		},
	}
	f.add(cmd)
	cmd.Flags().StringVar(&app, "app", "", "the application's code (required)")
	cmd.Flags().StringVar(&appKey, "app-key", "", "only this phone app")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the API's answer")
	return cmd
}
