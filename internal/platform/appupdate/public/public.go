// Package public serves the Expo Updates protocol to phones: the manifest
// and asset routes under /app-updates (docs/app-updates-design.md sections
// 2, 6 and 8). The routes are unauthenticated (a phone asks before anyone
// signs in; integrity comes from the signature), so they validate every input
// before any work, answer every failure in one shape, and reveal nothing about
// other applications.
package public

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
)

// Request headers of the protocol (expo-updates 58: FileDownloader).
const (
	headerProtocolVersion = "Expo-Protocol-Version"
	headerPlatform        = "Expo-Platform"
	headerRuntimeVersion  = "Expo-Runtime-Version"
	headerExpectSignature = "Expo-Expect-Signature"
	headerClientID        = "EAS-Client-ID"
	// HeaderChannel is the request header that carries the channel: the app
	// config's updates.requestHeaders sets it (the convention expo-updates
	// documents for EAS).
	HeaderChannel = "Expo-Channel-Name"
)

// Store is what the asset route reads files from.
type Store interface {
	Open(ctx context.Context, digest string) (io.ReadCloser, error)
}

// Repository is what the routes read.
type Repository interface {
	PublicHead(ctx context.Context, applicationCode, appKey, platform, runtimeVersion, channel string) ([]appupdate.Update, error)
	PublicAsset(ctx context.Context, applicationCode, appKey, sha256Hex string) (contentType string, size int64, found bool, err error)
	AssetsOf(ctx context.Context, ids []string) (map[string][]appupdate.Asset, error)
}

// Server is the public half of app updates.
type Server struct {
	Repo  Repository
	Store Store
	Keys  *appupdate.Keyring
	// AssetBase, when set, makes asset URLs absolute (base +
	// /app-updates/...); empty makes them relative to the manifest URL.
	AssetBase string

	cacheOnce sync.Once
	cache     *partCache
}

// cacheSize bounds the signed-part cache (entries are one manifest or directive each).
const cacheSize = 512

func (s *Server) parts() *partCache {
	s.cacheOnce.Do(func() { s.cache = newPartCache(cacheSize) })
	return s.cache
}

// Mount mounts the routes under appupdate.PublicPrefix. manifestMW and
// assetMW wrap the two routes (the per-IP governors).
func (s *Server) Mount(r chi.Router, manifestMW, assetMW []func(http.Handler) http.Handler) {
	r.Route(appupdate.PublicPrefix, func(sub chi.Router) {
		sub.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		})
		sub.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only GET")
		})
		sub.With(manifestMW...).Get("/{applicationCode}/{appKey}/manifest", s.manifest)
		sub.With(assetMW...).Get("/{applicationCode}/{appKey}/assets/{sha256}", s.asset)
	})
}

// writeError is every failure's answer: {"error": CODE, "message": "..."}.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}

// acceptsMultipart reports whether an Accept header admits multipart/mixed (an
// absent header admits anything). The client sends
// "multipart/mixed,application/expo+json,application/json".
func acceptsMultipart(accept string) bool {
	if strings.TrimSpace(accept) == "" {
		return true
	}
	for part := range strings.SplitSeq(accept, ",") {
		mt, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		if q, ok := params["q"]; ok {
			if v, err := strconv.ParseFloat(q, 64); err == nil && v <= 0 {
				continue
			}
		}
		if mt == "multipart/mixed" || mt == "multipart/*" || mt == "*/*" {
			return true
		}
	}
	return false
}

func (s *Server) manifest(w http.ResponseWriter, r *http.Request) {
	code, appKey := chi.URLParam(r, "applicationCode"), chi.URLParam(r, "appKey")
	if !appupdate.ValidApplicationCode(code) || !appupdate.ValidAppKey(appKey) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "the address does not name a phone app")
		return
	}
	if !s.Keys.Configured() {
		writeError(w, http.StatusServiceUnavailable, "APP_UPDATES_UNCONFIGURED", "updates are not available")
		return
	}
	if r.Header.Get(headerProtocolVersion) != "1" {
		writeError(w, http.StatusNotAcceptable, "UNSUPPORTED_PROTOCOL_VERSION", "only expo-protocol-version 1 is served")
		return
	}
	if !acceptsMultipart(r.Header.Get("Accept")) {
		writeError(w, http.StatusNotAcceptable, "NOT_ACCEPTABLE", "only multipart/mixed is served")
		return
	}
	platform, runtimeVersion, channel := r.Header.Get(headerPlatform), r.Header.Get(headerRuntimeVersion), r.Header.Get(HeaderChannel)
	switch {
	case !appupdate.ValidPlatform(platform):
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "expo-platform must be android or ios")
		return
	case !appupdate.ValidRuntimeVersion(runtimeVersion):
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "expo-runtime-version is missing or malformed")
		return
	case !appupdate.ValidChannel(channel):
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "expo-channel-name is missing or malformed")
		return
	}
	keyID := s.Keys.DefaultKeyID()
	expect, asked, err := appupdate.ParseExpectSignature(r.Header.Get(headerExpectSignature))
	switch {
	case errors.Is(err, appupdate.ErrUnsupportedAlg):
		writeError(w, http.StatusBadRequest, "UNSUPPORTED_SIGNATURE_ALGORITHM", "only rsa-v1_5-sha256 signatures are served")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "expo-expect-signature is malformed")
		return
	case asked && expect.KeyID != "":
		if !s.Keys.Has(expect.KeyID) {
			writeError(w, http.StatusBadRequest, "UNKNOWN_KEY_ID", "the key id in expo-expect-signature is not a signing key of this server")
			return
		}
		keyID = expect.KeyID
	}
	clientID := appupdate.NormalizeClientID(r.Header.Get(headerClientID))

	rows, err := s.Repo.PublicHead(r.Context(), code, appKey, platform, runtimeVersion, channel)
	if err != nil {
		slog.Error("app updates: manifest read failed", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "try again later")
		return
	}
	parts, err := s.answer(r.Context(), appupdate.Select(rows, clientID), keyID)
	if err != nil {
		slog.Error("app updates: manifest build failed", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "try again later")
		return
	}
	var body bytes.Buffer
	ct, err := appupdate.WriteMultipart(&body, parts)
	if err != nil {
		slog.Error("app updates: multipart write failed", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "try again later")
		return
	}
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("Expo-Protocol-Version", "1")
	h.Set("Expo-Sfv-Version", "0")
	h.Set("Cache-Control", "private, max-age=0")
	h.Set("Content-Length", strconv.Itoa(body.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

// answer is the signed parts for the selected row (nil: no update available).
// Built and signed once per row and key, then served from the cache: rows are
// immutable but for their rollout, which does not change their parts.
func (s *Server) answer(ctx context.Context, sel *appupdate.Update, keyID string) ([]appupdate.Part, error) {
	cacheKey := "none"
	if sel != nil {
		cacheKey = sel.ID
	}
	cacheKey += "|" + keyID + "|" + s.AssetBase
	if p, ok := s.parts().get(cacheKey); ok {
		return p, nil
	}
	var parts []appupdate.Part
	switch {
	case sel == nil:
		parts = []appupdate.Part{{Name: "directive", Body: appupdate.NoUpdateAvailableJSON()}}
	case sel.Kind == appupdate.KindRollbackToEmbedded:
		parts = []appupdate.Part{{Name: "directive", Body: appupdate.RollBackToEmbeddedJSON(sel)}}
	default:
		assets, err := s.Repo.AssetsOf(ctx, []string{sel.ID})
		if err != nil {
			return nil, err
		}
		u := *sel
		u.Assets = assets[sel.ID]
		body, err := appupdate.ManifestJSON(&u, s.AssetBase)
		if err != nil {
			return nil, err
		}
		parts = []appupdate.Part{
			{Name: "manifest", Body: body},
			{Name: "extensions", Body: appupdate.ExtensionsJSON()},
		}
	}
	// The manifest or directive part is signed; extensions are not (no client
	// verifies them).
	sig, err := s.Keys.Sign(ctx, keyID, parts[0].Body)
	if err != nil {
		return nil, err
	}
	parts[0].Signature = sig
	s.parts().put(cacheKey, parts)
	return parts, nil
}

func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	code, appKey, sha := chi.URLParam(r, "applicationCode"), chi.URLParam(r, "appKey"), chi.URLParam(r, "sha256")
	if !appupdate.ValidApplicationCode(code) || !appupdate.ValidAppKey(appKey) || !appupdate.ValidSHA256(sha) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	ct, size, found, err := s.Repo.PublicAsset(r.Context(), code, appKey, sha)
	if err != nil {
		slog.Error("app updates: asset read failed", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "try again later")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	etag := `"` + sha + `"`
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	rc, err := s.Store.Open(r.Context(), sha)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			// The registry names a file the store does not hold: an operator
			// problem, not something to tell the phone about.
			slog.Error("app updates: a published file is missing from the store", "sha256", sha)
			writeError(w, http.StatusNotFound, "NOT_FOUND", "not found")
			return
		}
		slog.Error("app updates: store open failed", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "try again later")
		return
	}
	defer rc.Close()
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("ETag", etag)
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, io.LimitReader(rc, size)); err != nil {
		slog.Warn("app updates: asset stream ended early", "sha256", sha, "err", err)
	}
}

// partCache is a bounded least-recently-used cache of signed parts.
type partCache struct {
	mu    sync.Mutex
	max   int
	ll    *list.List
	items map[string]*list.Element
}

type cacheEntry struct {
	key   string
	parts []appupdate.Part
}

func newPartCache(maxEntries int) *partCache {
	return &partCache{max: maxEntries, ll: list.New(), items: map[string]*list.Element{}}
}

func (c *partCache) get(k string) ([]appupdate.Part, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		c.ll.MoveToFront(e)
		return e.Value.(*cacheEntry).parts, true
	}
	return nil, false
}

func (c *partCache) put(k string, p []appupdate.Part) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		c.ll.MoveToFront(e)
		e.Value.(*cacheEntry).parts = p
		return
	}
	c.items[k] = c.ll.PushFront(&cacheEntry{key: k, parts: p})
	for c.ll.Len() > c.max {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.items, last.Value.(*cacheEntry).key)
	}
}
