//go:build integration

// Package appupdatetest is the integration harness of app updates: the real
// public routes and API over a real database and file store, and a client
// that asks and verifies exactly as expo-updates 58 does
// (docs/app-updates-design.md section 2). Used by the conformance test and the
// fcdev publish test.
package appupdatetest

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	appupdateapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/public"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// KeyID is the signing key id every world uses (the one `expo-updates
// codesigning:configure` defaults to).
const KeyID = "main"

// AllPerms are the three app-update permissions an application's service
// account holds (platform:application-service).
var AllPerms = []string{"platform:app-update:update:view", "platform:app-update:update:publish", "platform:app-update:rollout:manage"}

// LinkPerm is the install-link permission: operators only.
const LinkPerm = "platform:app-update:install-link:manage"

var (
	keyOnce sync.Once
	key     *rsa.PrivateKey
	certPEM []byte
)

// SigningKey is one RSA key and its code-signing certificate, shared by every
// world of a test binary (key generation is slow).
func SigningKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		now := time.Now().UTC()
		c, err := appupdate.NewCodeSigningCertificate(k, appupdate.CertificateOptions{
			CommonName: "acme driver-app updates", NotBefore: now.Add(-24 * time.Hour), NotAfter: now.AddDate(10, 0, 0),
		})
		if err != nil {
			panic(err)
		}
		key, certPEM = k, c
	})
	return key, certPEM
}

// World is the routes over one database, two applications (A and B) and a
// file store. Bearer tokens name principals: "operator" (anchor, every
// application), "sa-a" and "sa-b" (each application's service account),
// "client-admin" (client tier, every permission), "viewer" (view only),
// "nobody" (no permission).
type World struct {
	T         *testing.T
	Server    *httptest.Server
	Repo      *appupdate.Repository
	Binaries  *appupdate.BinaryRepository
	Links     *appupdate.LinkRepository
	Store     artifact.Store
	StoreDir  string
	Keys      *appupdate.Keyring
	CertPEM   []byte
	AppA      string // code
	AppAID    string
	AppB      string
	AppBID    string
	Principal map[string]*auth.AuthContext
	// InstallBase is the address install links are given under.
	InstallBase string
}

// Options adjust a world.
type Options struct {
	// NoKeys starts the public routes without a signing key.
	NoKeys bool
	// MaxBinaryBytes caps a binary upload (0: the default).
	MaxBinaryBytes int64
	// InstallBase is the address install links are given under (empty: the
	// test server's URL).
	InstallBase string
}

// NewWorld builds a world. The test binary's TestMain must call testpg.RunMain.
func NewWorld(t *testing.T, o Options) *World {
	t.Helper()
	httpcompat.Init()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	suffix := strings.ToLower(tsid.GenerateUntyped())
	w := &World{
		T: t, Repo: appupdate.NewRepository(pool), Binaries: appupdate.NewBinaryRepository(pool), Links: appupdate.NewLinkRepository(pool),
		AppA: "acme-" + suffix, AppB: "beta-" + suffix,
	}
	w.AppAID = testpg.EnsureApplication(t, w.AppA)
	w.AppBID = testpg.EnsureApplication(t, w.AppB)
	w.StoreDir = t.TempDir()
	store, err := artifact.NewFileStore(w.StoreDir)
	require.NoError(t, err)
	w.Store = store
	k, c := SigningKey(t)
	w.CertPEM = c
	if o.NoKeys {
		w.Keys, err = appupdate.ParseKeySpec("", "", nil)
		require.NoError(t, err)
	} else {
		w.Keys = appupdate.NewKeyringFromKeys(KeyID, map[string]*rsa.PrivateKey{KeyID: k})
	}
	w.Principal = map[string]*auth.AuthContext{
		"operator":     {PrincipalID: "prn_auoperator001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: append(slices.Clone(AllPerms), LinkPerm)},
		"sa-a":         {PrincipalID: "prn_ausaa00000001", Scope: auth.ScopeAnchor, Applications: []string{w.AppAID}, Permissions: AllPerms},
		"sa-b":         {PrincipalID: "prn_ausab00000001", Scope: auth.ScopeAnchor, Applications: []string{w.AppBID}, Permissions: AllPerms},
		"client-admin": {PrincipalID: "prn_auclient00001", Scope: auth.ScopeClient, Clients: []string{"clt_0000000000001"}, AllApplications: true, Permissions: AllPerms},
		"viewer":       {PrincipalID: "prn_auviewer00001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: AllPerms[:1]},
		"nobody":       {PrincipalID: "prn_aunobody00001", Scope: auth.ScopeAnchor, AllApplications: true},
	}

	st := &appupdateapi.State{
		Repo: w.Repo, Binaries: w.Binaries, Links: w.Links, Apps: application.NewRepository(pool), UoW: uow, Store: w.Store,
		MaxBinaryBytes: o.MaxBinaryBytes, SpoolDir: t.TempDir(), InstallBaseURL: o.InstallBase,
	}
	r := chi.NewMux()
	pub := &public.Server{Repo: w.Repo, Store: w.Store, Keys: w.Keys, Links: w.Links, InstallBase: o.InstallBase}
	pub.Mount(r, public.Middlewares{})
	r.Group(func(g chi.Router) {
		g.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				name := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
				if ac, ok := w.Principal[name]; ok {
					req = req.WithContext(auth.WithContext(req.Context(), ac))
				}
				next.ServeHTTP(rw, req)
			})
		})
		api := humachi.New(g, apiroute.PlatformAPIConfig("app-updates-test", "test"))
		appupdateapi.Register(api, st)
	})
	w.Server = httptest.NewServer(r)
	t.Cleanup(w.Server.Close)
	w.InstallBase = cmp.Or(o.InstallBase, w.Server.URL)
	pub.InstallBase, st.InstallBaseURL = w.InstallBase, w.InstallBase
	return w
}

// Do sends an API request as principal and returns the status and body.
func (w *World) Do(principal, method, path string, body any) (int, []byte) {
	w.T.Helper()
	var rd io.Reader
	ct := ""
	switch b := body.(type) {
	case nil:
	case []byte:
		rd, ct = bytes.NewReader(b), "application/octet-stream"
	default:
		raw, err := json.Marshal(b)
		require.NoError(w.T, err)
		rd, ct = bytes.NewReader(raw), "application/json"
	}
	req, err := http.NewRequestWithContext(context.Background(), method, w.Server.URL+path, rd)
	require.NoError(w.T, err)
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Authorization", "Bearer "+principal)
	resp, err := w.Server.Client().Do(req)
	require.NoError(w.T, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(w.T, err)
	return resp.StatusCode, out
}

// Export is a fake `expo export` of one platform: a bundle and assets.
type Export struct {
	Bundle []byte
	Assets []ExportAsset
}

// ExportAsset is one asset file.
type ExportAsset struct {
	Body []byte
	Ext  string
}

// NewExport makes an export whose files are unique to label.
func NewExport(label string, nAssets int) Export {
	e := Export{Bundle: []byte("// bundle " + label + "\n" + strings.Repeat("x", 4096))}
	exts := []string{"png", "ttf", "jpg", "json"}
	for i := range nAssets {
		e.Assets = append(e.Assets, ExportAsset{Body: fmt.Appendf(nil, "asset %d of %s %s", i, label, strings.Repeat("y", 100*i)), Ext: exts[i%len(exts)]})
	}
	return e
}

// SHA256Hex is a file's address.
func SHA256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Branch names where an update goes.
type Branch struct {
	App, AppKey, Platform, RuntimeVersion, Channel string
}

// Upload stores every file of e for app as principal (missing ones only, as
// fcdev does) and returns how many were uploaded.
func (w *World) Upload(principal, app string, e Export) int {
	w.T.Helper()
	files := map[string][]byte{SHA256Hex(e.Bundle): e.Bundle}
	for _, a := range e.Assets {
		files[SHA256Hex(a.Body)] = a.Body
	}
	shas := make([]string, 0, len(files))
	for s := range files {
		shas = append(shas, s)
	}
	st, body := w.Do(principal, http.MethodPost, "/api/app-updates/files/"+app+"/missing", map[string]any{"sha256": shas})
	require.Equal(w.T, http.StatusOK, st, string(body))
	var missing struct {
		Missing []string `json:"missing"`
	}
	require.NoError(w.T, json.Unmarshal(body, &missing))
	for _, s := range missing.Missing {
		st, body := w.Do(principal, http.MethodPut, "/api/app-updates/files/"+app+"/"+s, files[s])
		require.Equal(w.T, http.StatusOK, st, string(body))
	}
	return len(missing.Missing)
}

// PublishBody is the publish request of e on b.
func PublishBody(b Branch, e Export, rollout int, expoClient json.RawMessage) map[string]any {
	assets := []map[string]string{}
	for _, a := range e.Assets {
		assets = append(assets, map[string]string{"sha256": SHA256Hex(a.Body), "fileExtension": a.Ext})
	}
	body := map[string]any{
		"applicationCode": b.App, "appKey": b.AppKey, "platform": b.Platform, "runtimeVersion": b.RuntimeVersion,
		"channel": b.Channel, "rolloutPercent": rollout, "launchAsset": map[string]string{"sha256": SHA256Hex(e.Bundle)},
		"assets": assets, "message": "test update", "sourceCommit": "abc123",
	}
	if expoClient != nil {
		body["expoClient"] = expoClient
	}
	return body
}

// UpdateRow is the API's update answer.
type UpdateRow struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	ManifestID      string  `json:"manifestId"`
	CreatedAt       string  `json:"createdAt"`
	RolloutPercent  int     `json:"rolloutPercent"`
	ServedPercent   int     `json:"servedPercent"`
	Live            bool    `json:"live"`
	RepublishedFrom *string `json:"republishedFrom"`
}

// Publish uploads and publishes e on b as principal and returns the update.
func (w *World) Publish(principal string, b Branch, e Export, rollout int) UpdateRow {
	w.T.Helper()
	w.Upload(principal, b.App, e)
	st, body := w.Do(principal, http.MethodPost, "/api/app-updates/updates", PublishBody(b, e, rollout, json.RawMessage(`{"name":"Driver","slug":"driver-app"}`)))
	require.Equal(w.T, http.StatusCreated, st, string(body))
	var u UpdateRow
	require.NoError(w.T, json.Unmarshal(body, &u))
	return u
}
