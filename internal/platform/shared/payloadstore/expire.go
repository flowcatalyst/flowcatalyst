package payloadstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Expirer is implemented by an ObjectStore whose expired payloads the
// platform removes itself. Only the file store implements it: the S3 store is
// never cleaned by the platform, whose role there has no delete permission;
// its objects expire by a lifecycle rule on the bucket's prefix, which the
// infrastructure code sets (section 7).
type Expirer interface {
	// RemoveExpired removes the payloads written on a UTC date before the date
	// of cutoff, and reports how many date directories it removed. It removes
	// nothing that is not in the layout the store writes. The partition
	// manager calls it daily with the retention of msg_payloads plus 35 days
	// (stream.PayloadExpirySlackDays), the figure of the S3 lifecycle rule.
	RemoveExpired(ctx context.Context, cutoff time.Time) (removed int, err error)
}

// OpenExpirer returns the Expirer of the store rawURL names (FC_PAYLOAD_STORE)
// when the platform cleans that kind of store: a file store. Nil, with no
// error, for no store and for an S3 store (cleaned by its bucket's lifecycle
// rule), without building an S3 client.
func OpenExpirer(ctx context.Context, rawURL string) (Expirer, error) {
	if !strings.HasPrefix(rawURL, "file://") {
		return nil, nil
	}
	st, err := OpenStore(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	e, ok := st.(Expirer)
	if !ok {
		return nil, nil
	}
	return e, nil
}

// dateDirLayout is a date directory's name: the UTC date its segments were
// written on.
const dateDirLayout = "2006-01-02"

// RemoveExpired removes the date directories v1/<client>/<yyyy-mm-dd> whose
// date is before cutoff's UTC date, then the client directories left empty.
// It touches nothing else: an entry under v1 that is not a directory named
// as clientSegment names one, a date directory not named exactly yyyy-mm-dd,
// a symbolic link, and anything outside v1 are left alone. A directory that
// cannot be removed is skipped and reported in the error; the others are
// still removed.
func (s *FileStore) RemoveExpired(ctx context.Context, cutoff time.Time) (int, error) {
	root := filepath.Join(s.dir, refVersion)
	clients, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil // nothing written yet
	}
	if err != nil {
		return 0, fmt.Errorf("payloadstore: file expiry: read %s: %w", root, err)
	}
	before := cutoff.UTC().Format(dateDirLayout)
	removed := 0
	var errs []error
	for _, c := range clients {
		if ctx.Err() != nil {
			return removed, ctx.Err()
		}
		if !c.IsDir() || clientSegment(c.Name()) != c.Name() {
			continue
		}
		clientDir := filepath.Join(root, c.Name())
		dates, err := os.ReadDir(clientDir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		left := len(dates)
		for _, d := range dates {
			// The layout's dates compare as strings in date order.
			if !d.IsDir() || !isDateDir(d.Name()) || d.Name() >= before {
				continue
			}
			if err := os.RemoveAll(filepath.Join(clientDir, d.Name())); err != nil {
				errs = append(errs, err)
				continue
			}
			removed++
			left--
		}
		if left == 0 {
			// os.Remove refuses a directory that is not empty, so one that
			// gained an entry since it was read is kept.
			if err := os.Remove(clientDir); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return removed, fmt.Errorf("payloadstore: file expiry: %w", errors.Join(errs...))
	}
	return removed, nil
}

// isDateDir reports whether name is exactly a yyyy-mm-dd date.
func isDateDir(name string) bool {
	t, err := time.Parse(dateDirLayout, name)
	return err == nil && t.Format(dateDirLayout) == name
}
