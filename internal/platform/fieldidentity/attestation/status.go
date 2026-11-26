package attestation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// The attestation key status list (docs/field-unlock-keys-spec.md section 6.4):
// Google publishes the serials of revoked and suspended attestation
// certificates at https://android.googleapis.com/attestation/status, as
// {"entries": {"<serial, lowercase hex, no leading zeros>": {"status":
// "REVOKED"|"SUSPENDED", "expires"?, "reason"?, "comment"?}}}. The platform
// fetches it in the background ([RunStatusFetcher]) and keeps the last good
// copy ([StatusCache]); no request waits on the network.

// DefaultStatusURL is Google's published status list.
const DefaultStatusURL = "https://android.googleapis.com/attestation/status"

// maxStatusBytes bounds a fetched list (the published one is a few hundred KiB).
const maxStatusBytes = 16 << 20

// maxStatusRedirects bounds the redirects a fetch follows (same host, https).
const maxStatusRedirects = 3

// StatusList is a parsed status list.
type StatusList struct {
	entries map[string]RevocationState
	skipped int
}

// Len is the number of listed certificates.
func (l *StatusList) Len() int {
	if l == nil {
		return 0
	}
	return len(l.entries)
}

// Skipped is how many entries were dropped because their key is not a
// hexadecimal serial (no certificate can match one).
func (l *StatusList) Skipped() int {
	if l == nil {
		return 0
	}
	return l.skipped
}

// Status says whether the serial is listed, and how. The serial is compared in
// the list's form (normalised: lower case, no leading zeros).
func (l *StatusList) Status(serial string) (RevocationState, bool) {
	if l == nil {
		return "", false
	}
	key, ok := normaliseSerial(serial)
	if !ok {
		return "", false
	}
	st, ok := l.entries[key]
	return st, ok
}

// normaliseSerial is a serial in the list's form: hexadecimal, lower case,
// without leading zeros ("0" for zero). It reports false for anything that is
// not hexadecimal.
func normaliseSerial(serial string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(serial))
	if s == "" {
		return "", false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", false
		}
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		s = "0"
	}
	return s, true
}

// ParseStatusList reads a status list. Only a document that is not the
// published shape at all is an error (not JSON, or no "entries" object), so a
// broken fetch never replaces a good copy. Inside it the reading leans to
// revoked: a serial is normalised (lower case, leading zeros dropped), an
// entry whose status is not SUSPENDED (REVOKED, a status this version does not
// know, a missing one, an entry that is not an object) counts as revoked, and
// a key that is not a hexadecimal serial is skipped ([StatusList.Skipped]),
// since no certificate can match it.
func ParseStatusList(data []byte) (*StatusList, error) {
	var doc struct {
		Entries map[string]json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("attestation status list: %w", err)
	}
	if doc.Entries == nil {
		return nil, errors.New("attestation status list: no entries object")
	}
	out := &StatusList{entries: make(map[string]RevocationState, len(doc.Entries))}
	for serial, raw := range doc.Entries {
		key, ok := normaliseSerial(serial)
		if !ok {
			out.skipped++
			continue
		}
		var e struct {
			Status string `json:"status"`
		}
		state := RevocationRevoked
		if json.Unmarshal(raw, &e) == nil && e.Status == "SUSPENDED" {
			state = RevocationSuspended
		}
		// Two spellings of one serial: revoked wins over suspended.
		if prev, dup := out.entries[key]; dup && prev == RevocationRevoked {
			continue
		}
		out.entries[key] = state
	}
	return out, nil
}

// ValidateStatusURL refuses a status list address that is not https with a
// host: the list decides which keys are revoked, so it is never fetched in the
// clear.
func ValidateStatusURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("attestation status list address: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("attestation status list address %q: must be an https URL", raw)
	}
	return nil
}

// StatusCache holds the last good list and when it was fetched. A copy older
// than MaxAge is not usable. Safe for concurrent use.
type StatusCache struct {
	MaxAge time.Duration

	mu        sync.RWMutex
	list      *StatusList
	fetchedAt time.Time
}

// NewStatusCache is an empty cache whose copies are usable for maxAge.
func NewStatusCache(maxAge time.Duration) *StatusCache { return &StatusCache{MaxAge: maxAge} }

// Set stores a freshly fetched list.
func (c *StatusCache) Set(l *StatusList, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list, c.fetchedAt = l, at
}

// Usable returns the list when there is one no older than MaxAge at now.
func (c *StatusCache) Usable(now time.Time) (*StatusList, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.list == nil || now.Sub(c.fetchedAt) > c.MaxAge {
		return nil, false
	}
	return c.list, true
}

// Age is when the last good copy was fetched (zero when never).
func (c *StatusCache) Age() time.Time {
	if c == nil {
		return time.Time{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fetchedAt
}

// FetchStatus fetches and parses the list at rawURL, which must be https
// ([ValidateStatusURL]). It follows at most a few redirects, each to https on
// the same host; one elsewhere or to plain http fails the fetch. The body is
// read up to 16 MiB; the client's timeout and ctx bound the time.
func FetchStatus(ctx context.Context, client *http.Client, rawURL string) (*StatusList, error) {
	if err := ValidateStatusURL(rawURL); err != nil {
		return nil, err
	}
	c := *client
	c.CheckRedirect = sameHostHTTPS
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("attestation status list: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxStatusBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxStatusBytes {
		return nil, errors.New("attestation status list: too large")
	}
	return ParseStatusList(data)
}

// sameHostHTTPS is the fetch's redirect policy.
func sameHostHTTPS(req *http.Request, via []*http.Request) error {
	if len(via) > maxStatusRedirects {
		return errors.New("attestation status list: too many redirects")
	}
	if req.URL.Scheme != "https" || len(via) == 0 || req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("attestation status list: refusing a redirect to %s://%s", req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// RunStatusFetcher fetches the list at once and then every interval until ctx
// is done, keeping the last good copy in cache and handing each good copy to
// fetched (nil for none), which checks the registered keys against it. A
// failed fetch keeps the old copy; it is logged as a warning while the copy is
// still usable and as an error at every attempt once it is not (stale, or
// never fetched), because then every unlock under REQUIRED is refused.
func RunStatusFetcher(ctx context.Context, client *http.Client, rawURL string, cache *StatusCache, interval time.Duration,
	fetched func(context.Context, *StatusList),
) {
	fetch := func() {
		fctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		l, err := FetchStatus(fctx, client, rawURL)
		if err != nil {
			if _, ok := cache.Usable(time.Now().UTC()); ok {
				slog.Warn("field attestation status list fetch failed; keeping the last good copy",
					"url", rawURL, "lastGood", cache.Age(), "err", err)
			} else {
				slog.Error("field attestation status list unavailable: no usable copy, so unlock keys under REQUIRED are refused until a fetch succeeds",
					"url", rawURL, "lastGood", cache.Age(), "maxAge", cache.MaxAge, "err", err)
			}
			return
		}
		cache.Set(l, time.Now().UTC())
		if l.Skipped() > 0 {
			slog.Warn("field attestation status list: entries skipped, their keys are not serials", "skipped", l.Skipped())
		}
		slog.Debug("field attestation status list fetched", "entries", l.Len())
		if fetched != nil {
			fetched(ctx, l)
		}
	}
	fetch()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done(): // shutdown
			return
		case <-tick.C: // refresh due
			fetch()
		}
	}
}
