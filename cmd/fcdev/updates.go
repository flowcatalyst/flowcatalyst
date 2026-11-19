package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
)

// newUpdatesCmd is `fcdev updates`: over-the-air updates of phone apps built
// with Expo (docs/app-updates-design.md section 11). The platform commands
// connect like `fcdev fn` (a profile, or the local fcdev's credential).
func newUpdatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "updates",
		Short: "Publish and manage over-the-air updates of Expo phone apps",
	}
	cmd.AddCommand(newUpdatesKeygenCmd(), newUpdatesCertificateCmd(), newUpdatesPublishCmd(), newUpdatesListCmd(),
		newUpdatesBranchesCmd(), newUpdatesRolloutCmd(), newUpdatesRepublishCmd(), newUpdatesRollbackCmd(),
		newUpdatesBinariesCmd(), newUpdatesLinksCmd())
	return cmd
}

// appUpdatesDevDir is where `fcdev start` keeps the development signing key,
// its certificate and the file store.
func appUpdatesDevDir() string { return filepath.Join(userDataDir(), "flowcatalyst", "app-updates") }

const (
	devSigningKeyFile  = "signing-key.pem"
	devCertificateFile = "certificate.pem"
	devSigningKeyID    = "main"
)

// ensureDevSigningKey makes (once) the development signing key and its
// certificate under dir and returns the key's path.
func ensureDevSigningKey(dir string) (keyPath, certPath string, err error) {
	keyPath, certPath = filepath.Join(dir, devSigningKeyFile), filepath.Join(dir, devCertificateFile)
	if fileExists(keyPath) && fileExists(certPath) {
		return keyPath, certPath, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	keyPEM, certPEM, err := newSigningKeyAndCertificate("FlowCatalyst fcdev app updates (development)", 10)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil { //nolint:gosec // G306: a certificate is public
		return "", "", err
	}
	return keyPath, certPath, nil
}

// ── keygen / certificate ───────────────────────────────────────────────────

func newUpdatesKeygenCmd() *cobra.Command {
	var keyID, commonName, outDir string
	var years int
	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Create an update-signing key and the code-signing certificate an app embeds",
		Long: `Creates a 2048-bit RSA key (private-key.pem, mode 0600) and a self-signed
code-signing certificate (certificate.pem) as expo-updates requires: key usage
digital signature, extended key usage code signing, valid from one day ago for
--years years.

The private key goes into the platform's secret store and is named in
FC_APP_UPDATES_SIGNING_KEYS as <key-id>=aws-sm://... (or aws-ps://, env:, file:).
The certificate goes into the app: updates.codeSigningCertificate, with
updates.codeSigningMetadata {"keyid": "<key-id>", "alg": "rsa-v1_5-sha256"}.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !appupdate.ValidKeyID(keyID) {
				return fmt.Errorf("--key-id %q: letters, digits, '.', '_' and '-' only", keyID)
			}
			if years < 1 || years > 30 {
				return errors.New("--years must be from 1 to 30")
			}
			keyPath, certPath := filepath.Join(outDir, "private-key.pem"), filepath.Join(outDir, "certificate.pem")
			for _, p := range []string{keyPath, certPath} {
				if fileExists(p) {
					return fmt.Errorf("%s exists; nothing was written (choose another --out-dir)", p)
				}
			}
			if err := os.MkdirAll(outDir, 0o700); err != nil {
				return err
			}
			keyPEM, certPEM, err := newSigningKeyAndCertificate(commonName, years)
			if err != nil {
				return err
			}
			if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
				return err
			}
			if err := os.WriteFile(certPath, certPEM, 0o644); err != nil { //nolint:gosec // G306: a certificate is public
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "wrote %s (keep it secret) and %s\n", keyPath, certPath)
			fmt.Fprintf(out, "platform: FC_APP_UPDATES_SIGNING_KEYS=%s=<where the key is kept, e.g. aws-sm://...>\n", keyID)
			fmt.Fprintf(out, "app:      updates.codeSigningCertificate = ./certificate.pem, updates.codeSigningMetadata = {\"keyid\": %q, \"alg\": %q}\n", keyID, appupdate.AlgRSASHA256)
			return nil
		},
	}
	cmd.Flags().StringVar(&keyID, "key-id", devSigningKeyID, "the key id the app names in codeSigningMetadata.keyid")
	cmd.Flags().StringVar(&commonName, "common-name", "app updates", "the certificate's common name")
	cmd.Flags().IntVar(&years, "years", 10, "how long the certificate is valid; an expired certificate stops every update")
	cmd.Flags().StringVar(&outDir, "out-dir", ".", "where to write private-key.pem and certificate.pem")
	return cmd
}

// newSigningKeyAndCertificate makes a key and its code-signing certificate,
// valid from a day ago (a phone with a slow clock refuses a certificate not
// yet valid) for years, and checks the certificate as the client will.
func newSigningKeyAndCertificate(commonName string, years int) (keyPEM, certPEM []byte, err error) {
	keyPEM = generateRSAPEMForUpdates()
	key, err := appupdate.ParsePrivateKeyPEM(keyPEM)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	certPEM, err = appupdate.NewCodeSigningCertificate(key, appupdate.CertificateOptions{
		CommonName: commonName, NotBefore: now.Add(-24 * time.Hour), NotAfter: now.AddDate(years, 0, 0),
	})
	if err != nil {
		return nil, nil, err
	}
	if _, err := appupdate.CheckCodeSigningCertificate(certPEM, &key.PublicKey, now); err != nil {
		return nil, nil, fmt.Errorf("the new certificate fails the client's checks: %w", err)
	}
	return keyPEM, certPEM, nil
}

func newUpdatesCertificateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "certificate",
		Short: "Print the local fcdev's development code-signing certificate (key id main)",
		Long: `Prints the certificate of the signing key a local fcdev uses (it creates both on
first start). Embed it in a development build: updates.codeSigningCertificate,
with updates.codeSigningMetadata {"keyid": "main", "alg": "rsa-v1_5-sha256"}.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p := filepath.Join(appUpdatesDevDir(), devCertificateFile)
			b, err := os.ReadFile(p)
			if err != nil {
				return fmt.Errorf("no development certificate at %s: start fcdev once to create it (%w)", p, err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "# %s (key id %s)\n", p, devSigningKeyID)
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	}
}

// ── publish ────────────────────────────────────────────────────────────────

type publishedUpdate struct {
	ID             string `json:"id"`
	Platform       string `json:"platform"`
	RuntimeVersion string `json:"runtimeVersion"`
	Channel        string `json:"channel"`
	ManifestID     string `json:"manifestId"`
	CreatedAt      string `json:"createdAt"`
	RolloutPercent int    `json:"rolloutPercent"`
	// BinariesOnBranch is how many binaries of the runtime version are
	// published on the channel (docs/app-updates-design.md section 14.4).
	BinariesOnBranch *int64 `json:"binariesOnBranch,omitempty"`
}

func newUpdatesPublishCmd() *cobra.Command {
	var f fnPlatformFlags
	var app, appKey, channel, platform, rv, rvAndroid, rvIOS, expoConfigPath, message, commit string
	var rollout int
	var jsonOut, yes, requireBinary bool
	cmd := &cobra.Command{
		Use:   "publish <export-dir>",
		Short: "Upload an expo export's files and publish it as an update",
		Long: `Reads an expo export (npx expo export), uploads the files the application has
not stored, and registers one update per platform; each is live when this
returns. Fails before uploading anything when the export is malformed or a
platform has no runtime version (an export does not record one: pass
--runtime-version, or --expo-config / <export-dir>/expoConfig.json with a
runtimeVersion string).

The app config (npx expo config --type public --json) is served to the app as
Constants.expoConfig; it is read from --expo-config, else
<export-dir>/expoConfig.json when present.

An Android update for a runtime version no binary on the channel carries
(fcdev updates binaries publish) is published with a warning: phones may run
binaries distributed elsewhere. --require-binary refuses it instead.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := checkBranchFlags(app, appKey, channel); err != nil {
				return err
			}
			if rollout < 0 || rollout > 100 {
				return errors.New("--rollout must be from 0 to 100")
			}
			var platforms []string
			switch platform {
			case "", "all":
			case appupdate.PlatformAndroid, appupdate.PlatformIOS:
				platforms = []string{platform}
			default:
				return errors.New("--os must be android, ios or all")
			}
			exp, err := readExport(args[0], platforms, appupdate.DefaultMaxFileBytes)
			if err != nil {
				return err
			}
			if exp.ExpoConfig, expoConfigPath, err = readExpoConfig(exp.Dir, expoConfigPath); err != nil {
				return err
			}
			if exp.ExpoConfig == nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "fcdev: warning: no app config (--expo-config or expoConfig.json): Constants.expoConfig will be empty in this update")
			}
			rvs := map[string]string{}
			for _, p := range exp.Platforms {
				per := map[string]string{"android": rvAndroid, "ios": rvIOS}[p.Platform]
				if rvs[p.Platform], err = runtimeVersionFor(p.Platform, per, rv, exp.ExpoConfig); err != nil {
					return err
				}
			}
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			a.http.Timeout = 15 * time.Minute
			if err := a.confirm("publish an update of "+app+"/"+appKey+" to channel "+channel, yes); err != nil {
				return err
			}
			if err := uploadMissing(ctx, a, app, exp, cmd.ErrOrStderr()); err != nil {
				return err
			}
			// Every file is stored: registering makes each update live.
			out, err := publishExport(ctx, a, exp, publishOpts{
				App: app, AppKey: appKey, Channel: channel, Rollout: rollout, Message: message, Commit: commit, RuntimeVersions: rvs,
				RequireBinary: requireBinary,
			})
			if err != nil {
				return err
			}
			for _, u := range out {
				if u.Platform == appupdate.PlatformAndroid && u.BinariesOnBranch != nil && *u.BinariesOnBranch == 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "fcdev: warning: no binary of runtime version %s is published on channel %s: "+
						"no phone installed from this platform's install links runs this update (--require-binary refuses instead)\n", u.RuntimeVersion, u.Channel)
				}
			}
			if !jsonOut {
				for _, u := range out {
					fmt.Fprintf(cmd.OutOrStdout(), "published %s: %s (runtime %s, channel %s, manifest %s, rollout %d%%)\n",
						u.Platform, u.ID, u.RuntimeVersion, u.Channel, u.ManifestID, u.RolloutPercent)
				}
			}
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"updates": out})
			}
			return nil
		},
	}
	f.add(cmd)
	addBranchFlags(cmd, &app, &appKey, &channel)
	cmd.Flags().StringVar(&platform, "os", "all", "android, ios or all (every platform in the export); --platform is the platform URL")
	cmd.Flags().StringVar(&rv, "runtime-version", "", "the runtime version of the binaries the update is for (all platforms)")
	cmd.Flags().StringVar(&rvAndroid, "runtime-version-android", "", "the android runtime version, when it differs")
	cmd.Flags().StringVar(&rvIOS, "runtime-version-ios", "", "the ios runtime version, when it differs")
	cmd.Flags().StringVar(&expoConfigPath, "expo-config", "", "the public app config JSON (default <export-dir>/expoConfig.json when present)")
	cmd.Flags().IntVar(&rollout, "rollout", 100, "share of the branch's phones that receive it (0-100)")
	cmd.Flags().StringVar(&message, "message", "", "what changed")
	cmd.Flags().StringVar(&commit, "commit", "", "the source commit")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the published updates as JSON")
	cmd.Flags().BoolVar(&requireBinary, "require-binary", false, "refuse an update for a runtime version no binary on the channel carries (use with --os android: iOS binaries are not published here)")
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
	return cmd
}

// publishOpts is what publishExport registers.
type publishOpts struct {
	App, AppKey, Channel, Message, Commit string
	Rollout                               int
	// RuntimeVersions is each platform's runtime version.
	RuntimeVersions map[string]string
	// RequireBinary asks the platform to refuse an update no published binary carries.
	RequireBinary bool
}

// publishExport registers one update per platform of exp (its files stored).
// A platform that fails stops the rest; the ones before it are published.
func publishExport(ctx context.Context, a *fnAPI, exp *exportBundle, o publishOpts) ([]publishedUpdate, error) {
	var out []publishedUpdate
	for _, p := range exp.Platforms {
		assets := make([]map[string]string, 0, len(p.Assets))
		for _, x := range p.Assets {
			assets = append(assets, map[string]string{"sha256": x.SHA256, "fileExtension": x.Ext})
		}
		body := map[string]any{
			"applicationCode": o.App, "appKey": o.AppKey, "platform": p.Platform, "runtimeVersion": o.RuntimeVersions[p.Platform],
			"channel": o.Channel, "rolloutPercent": o.Rollout, "launchAsset": map[string]string{"sha256": p.Launch.SHA256},
			"assets": assets,
		}
		if o.Message != "" {
			body["message"] = o.Message
		}
		if o.Commit != "" {
			body["sourceCommit"] = o.Commit
		}
		if exp.ExpoConfig != nil {
			body["expoClient"] = exp.ExpoConfig
		}
		if o.RequireBinary {
			body["requireBinary"] = true
		}
		var u publishedUpdate
		if err := a.json(ctx, http.MethodPost, "/api/app-updates/updates", body, &u); err != nil {
			return out, fmt.Errorf("publish %s: %w", p.Platform, err)
		}
		out = append(out, u)
	}
	return out, nil
}

func addBranchFlags(cmd *cobra.Command, app, appKey, channel *string) {
	cmd.Flags().StringVar(app, "app", "", "the application's code (required)")
	cmd.Flags().StringVar(appKey, "app-key", "", "the phone app's key, as in its update URL (required)")
	cmd.Flags().StringVar(channel, "channel", "", "the channel the app sends in expo-channel-name (required)")
}

func checkBranchFlags(app, appKey, channel string) error {
	switch {
	case !appupdate.ValidApplicationCode(app):
		return errors.New("--app must be an application code")
	case !appupdate.ValidAppKey(appKey):
		return errors.New("--app-key must be 1-40 lower-case letters, digits and hyphens")
	case !appupdate.ValidChannel(channel):
		return errors.New("--channel must be 1-64 lower-case letters, digits and . _ -")
	}
	return nil
}

// uploadMissing asks which files the application lacks and uploads those.
func uploadMissing(ctx context.Context, a *fnAPI, app string, exp *exportBundle, log io.Writer) error {
	shas := make([]string, 0, len(exp.Files))
	for sha := range exp.Files {
		shas = append(shas, sha)
	}
	var missing struct {
		Missing []string `json:"missing"`
	}
	for start := 0; start < len(shas); start += appupdate.MaxMissingQuery {
		end := min(start+appupdate.MaxMissingQuery, len(shas))
		var page struct {
			Missing []string `json:"missing"`
		}
		if err := a.json(ctx, http.MethodPost, "/api/app-updates/files/"+url.PathEscape(app)+"/missing",
			map[string]any{"sha256": shas[start:end]}, &page); err != nil {
			return fmt.Errorf("ask which files are stored: %w", err)
		}
		missing.Missing = append(missing.Missing, page.Missing...)
	}
	fmt.Fprintf(log, "%d files, %d to upload\n", len(shas), len(missing.Missing))
	for i, sha := range missing.Missing {
		f, ok := exp.Files[sha]
		if !ok {
			return fmt.Errorf("the platform asked for %s, which this export does not have", sha)
		}
		if err := uploadFile(ctx, a, app, f); err != nil {
			return fmt.Errorf("upload %s: %w", filepath.Base(f.Path), err)
		}
		fmt.Fprintf(log, "  [%d/%d] %s (%d bytes)\n", i+1, len(missing.Missing), short(sha), f.Size)
	}
	return nil
}

func uploadFile(ctx context.Context, a *fnAPI, app string, f exportFile) error {
	fh, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	return a.do(ctx, http.MethodPut, "/api/app-updates/files/"+url.PathEscape(app)+"/"+f.SHA256, fh, "application/octet-stream", nil)
}

// ── reads ──────────────────────────────────────────────────────────────────

type updateRow struct {
	ID              string  `json:"id"`
	AppKey          string  `json:"appKey"`
	Platform        string  `json:"platform"`
	RuntimeVersion  string  `json:"runtimeVersion"`
	Channel         string  `json:"channel"`
	Kind            string  `json:"kind"`
	CreatedAt       string  `json:"createdAt"`
	RolloutPercent  int     `json:"rolloutPercent"`
	ServedPercent   int     `json:"servedPercent"`
	Message         *string `json:"message"`
	RepublishedFrom *string `json:"republishedFrom"`
	PublishedByName *string `json:"publishedByName"`
}

func (u updateRow) describe() string {
	what := u.Kind
	if u.RepublishedFrom != nil {
		what = "UPDATE (of " + *u.RepublishedFrom + ")"
	}
	return what
}

func newUpdatesListCmd() *cobra.Command {
	var f fnPlatformFlags
	var app, appKey, platform, rv, channel string
	var size int
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List an application's updates, newest first, with what is live",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !appupdate.ValidApplicationCode(app) {
				return errors.New("--app must be an application code")
			}
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			q := url.Values{"applicationCode": {app}, "size": {strconv.Itoa(size)}}
			for k, v := range map[string]string{"appKey": appKey, "platform": platform, "runtimeVersion": rv, "channel": channel} {
				if v != "" {
					q.Set(k, v)
				}
			}
			raw, err := a.doRaw(cmd.Context(), http.MethodGet, "/api/app-updates/updates?"+q.Encode(), nil, "")
			if err != nil {
				return err
			}
			if jsonOut {
				_, err := cmd.OutOrStdout().Write(raw)
				return err
			}
			var page struct {
				Data  []updateRow `json:"data"`
				Total int64       `json:"total"`
			}
			if err := json.Unmarshal(raw, &page); err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tAPP KEY\tPLATFORM\tRUNTIME\tCHANNEL\tKIND\tCREATED\tROLLOUT\tSERVED\tMESSAGE")
			for _, u := range page.Data {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d%%\t%d%%\t%s\n", u.ID, u.AppKey, u.Platform, u.RuntimeVersion,
					u.Channel, u.describe(), u.CreatedAt, u.RolloutPercent, u.ServedPercent, deref(u.Message))
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
	cmd.Flags().StringVar(&platform, "os", "", "only android or ios")
	cmd.Flags().StringVar(&rv, "runtime-version", "", "only this runtime version")
	cmd.Flags().StringVar(&channel, "channel", "", "only this channel")
	cmd.Flags().IntVar(&size, "size", 50, "how many (at most 1000)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the API's answer")
	return cmd
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.ReplaceAll(*s, "\n", " ")
}

func newUpdatesBranchesCmd() *cobra.Command {
	var f fnPlatformFlags
	var app, appKey string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "branches",
		Short: "List an application's branches (app key, platform, runtime version, channel) and what is live on each",
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
			raw, err := a.doRaw(cmd.Context(), http.MethodGet, "/api/app-updates/branches?"+q.Encode(), nil, "")
			if err != nil {
				return err
			}
			if jsonOut {
				_, err := cmd.OutOrStdout().Write(raw)
				return err
			}
			var resp struct {
				Branches []struct {
					AppKey, Platform, RuntimeVersion, Channel string
					Current                                   updateRow  `json:"current"`
					Previous                                  *updateRow `json:"previous"`
					UpdateCount                               int64      `json:"updateCount"`
				} `json:"branches"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "APP KEY\tPLATFORM\tRUNTIME\tCHANNEL\tCURRENT\tKIND\tROLLOUT\tREST GET\tUPDATES")
			for _, b := range resp.Branches {
				rest := "-"
				if b.Previous != nil {
					rest = b.Previous.ID
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d%%\t%s\t%d\n", b.AppKey, b.Platform, b.RuntimeVersion, b.Channel,
					b.Current.ID, b.Current.describe(), b.Current.RolloutPercent, rest, b.UpdateCount)
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

// ── rollout / republish / rollback ─────────────────────────────────────────

func newUpdatesRolloutCmd() *cobra.Command {
	var f fnPlatformFlags
	var yes bool
	cmd := &cobra.Command{
		Use:   "rollout <updateId> <percent>",
		Short: "Change the share of its branch's phones an update reaches (newest update only)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pct, err := strconv.Atoi(args[1])
			if err != nil || pct < 0 || pct > 100 {
				return errors.New("percent must be a whole number from 0 to 100")
			}
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			if err := a.confirm(fmt.Sprintf("set update %s's rollout to %d%%", args[0], pct), yes); err != nil {
				return err
			}
			var u updateRow
			if err := a.json(cmd.Context(), http.MethodPut, "/api/app-updates/updates/"+url.PathEscape(args[0])+"/rollout",
				map[string]int{"rolloutPercent": pct}, &u); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s now reaches %d%% of %s/%s/%s/%s\n", u.ID, u.RolloutPercent, u.AppKey, u.Platform, u.RuntimeVersion, u.Channel)
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
	return cmd
}

func newUpdatesRepublishCmd() *cobra.Command {
	var f fnPlatformFlags
	var message string
	var yes bool
	cmd := &cobra.Command{
		Use:   "republish <updateId>",
		Short: "Make an earlier update live again (published as a new update with its files)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			if err := a.confirm("republish update "+args[0], yes); err != nil {
				return err
			}
			body := map[string]any{}
			if message != "" {
				body["message"] = message
			}
			var u updateRow
			if err := a.json(cmd.Context(), http.MethodPost, "/api/app-updates/updates/"+url.PathEscape(args[0])+"/republish", body, &u); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "republished %s as %s on %s/%s/%s/%s\n", args[0], u.ID, u.AppKey, u.Platform, u.RuntimeVersion, u.Channel)
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().StringVar(&message, "message", "", "why")
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
	return cmd
}

func newUpdatesRollbackCmd() *cobra.Command {
	var f fnPlatformFlags
	var app, appKey, channel, platform, rv, message string
	var toEmbedded, yes bool
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Roll a branch back to its previous update, or (--to-embedded) to the bundle in the binary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkBranchFlags(app, appKey, channel); err != nil {
				return err
			}
			if !appupdate.ValidPlatform(platform) {
				return errors.New("--os must be android or ios")
			}
			if !appupdate.ValidRuntimeVersion(rv) {
				return errors.New("--runtime-version is required")
			}
			to := "PREVIOUS"
			if toEmbedded {
				to = "EMBEDDED"
			}
			a, err := newFnAPI(cmd, &f)
			if err != nil {
				return err
			}
			if err := a.confirm(fmt.Sprintf("roll %s/%s/%s/%s/%s back to %s", app, appKey, platform, rv, channel, strings.ToLower(to)), yes); err != nil {
				return err
			}
			body := map[string]any{
				"applicationCode": app, "appKey": appKey, "platform": platform,
				"runtimeVersion": rv, "channel": channel, "to": to,
			}
			if message != "" {
				body["message"] = message
			}
			var res struct {
				Update  updateRow `json:"update"`
				Created bool      `json:"created"`
			}
			if err := a.json(cmd.Context(), http.MethodPost, "/api/app-updates/rollback", body, &res); err != nil {
				return err
			}
			switch {
			case !res.Created:
				fmt.Fprintf(cmd.OutOrStdout(), "already rolled back to the embedded bundle (%s)\n", res.Update.ID)
			case toEmbedded:
				fmt.Fprintf(cmd.OutOrStdout(), "rolled back to the embedded bundle: %s\n", res.Update.ID)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "rolled back: %s republishes %s\n", res.Update.ID, deref(res.Update.RepublishedFrom))
			}
			return nil
		},
	}
	f.add(cmd)
	addBranchFlags(cmd, &app, &appKey, &channel)
	cmd.Flags().StringVar(&platform, "os", "", "android or ios (required)")
	cmd.Flags().StringVar(&rv, "runtime-version", "", "the runtime version (required)")
	cmd.Flags().BoolVar(&toEmbedded, "to-embedded", false, "tell phones to run the bundle embedded in their binary")
	cmd.Flags().StringVar(&message, "message", "", "why")
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: with a prod profile that sets allow_noninteractive = true, skip the typed confirmation")
	return cmd
}

// generateRSAPEMForUpdates makes a 2048-bit RSA key in PKCS#1 PEM.
func generateRSAPEMForUpdates() []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("rsa generate: " + err.Error())
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}
