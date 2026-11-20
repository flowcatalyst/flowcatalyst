package payloadstore

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"log/slog"
	"time"
	"unicode/utf8"
)

// castagnoli is the CRC-32C table a reference's checksum uses.
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// Item is one payload to store: the bytes as the producer sent them, the
// client of the event or job it is stored for ("" for none), and that row's
// created_at, under which the payload row is stored.
type Item struct {
	ClientID  string
	CreatedAt time.Time
	Data      []byte
}

// Prepared is the outcome of one Item: the row to insert, or the reason it
// cannot be stored (Err, with Row zero). Err wraps ErrTooLarge or ErrNotText
// (permanent: refuse the item) or ErrStoreUnavailable (retryable: the store
// could not be written and the payload is too large for the database).
type Prepared struct {
	Row Row
	Err error
}

// Prepare decides, for each payload of one batch, whether it is kept in the
// database or offloaded, writes the offloaded ones to the store, and returns
// in the order of items the rows to insert (docs/payload-offload-design.md,
// sections 5 to 7). It never inserts anything.
//
//   - A payload over MaxPayloadBytes (with the payload table written, as it
//     is wherever Prepare is called) is refused (ErrTooLarge), and so is one
//     the payload table cannot hold as text (ErrNotText: not valid UTF-8, or
//     a NUL byte).
//   - While offloading, a payload larger than OffloadMinBytes is compressed
//     on its own with the configured codec, and the offloaded payloads of one
//     client are written as one object, a segment, keyed
//     v1/<client id or _>/<yyyy-mm-dd UTC>/<segment id>. Its row holds the
//     reference.
//   - Every other payload's row holds its bytes.
//   - When a segment cannot be written, its payloads up to InlineMaxBytes are
//     kept in their rows (counted, and logged once per call at warn); larger
//     ones are refused with ErrStoreUnavailable.
//
// Every store write is finished before Prepare returns, so a reference is
// never inserted before its object exists. A row's id is a new untyped TSID.
func (s *Service) Prepare(ctx context.Context, items []Item) []Prepared {
	out := make([]Prepared, len(items))
	maxBytes := s.storableBytes()
	// Offloaded items by client, in the order clients first appear.
	groups := map[string][]int{}
	var order []string
	for i, it := range items {
		n := len(it.Data)
		if n > maxBytes {
			out[i].Err = fmt.Errorf("%w: %d bytes, the largest accepted is %d", ErrTooLarge, n, maxBytes)
			continue
		}
		if err := checkText(it.Data); err != nil {
			out[i].Err = err
			continue
		}
		out[i].Row = Row{ID: s.newID(), CreatedAt: it.CreatedAt, Size: n}
		if s.offloading() && n > s.cfg.OffloadMinBytes {
			if _, ok := groups[it.ClientID]; !ok {
				order = append(order, it.ClientID)
			}
			groups[it.ClientID] = append(groups[it.ClientID], i)
			continue
		}
		out[i].Row.Data = it.Data
	}
	if len(order) == 0 {
		return out
	}

	date := s.now().UTC().Format("2006-01-02")
	var failures storeFailures
	for _, client := range order {
		idx := groups[client]
		key := "v1/" + clientSegment(client) + "/" + date + "/" + s.newID()
		refs, err := s.writeSegment(ctx, key, items, idx)
		if err != nil {
			failures.add(err)
			for _, i := range idx {
				if len(items[i].Data) <= s.cfg.InlineMaxBytes {
					out[i].Row.Data = items[i].Data
					inlineFallbackTotal.Inc()
					failures.keptInline++
					continue
				}
				out[i] = Prepared{Err: fmt.Errorf("%w: %d bytes is over the %d the database takes: %w",
					ErrStoreUnavailable, len(items[i].Data), s.cfg.InlineMaxBytes, err)}
				failures.refused++
			}
			continue
		}
		for j, i := range idx {
			ref := refs[j]
			out[i].Row.Ref = &ref
			offloadedTotal.Inc()
			offloadedBytes.WithLabelValues(formRaw).Add(float64(len(items[i].Data)))
			offloadedBytes.WithLabelValues(formStored).Add(float64(ref.Length))
		}
	}
	failures.log()
	return out
}

// PrepareInline is Prepare for a writer that may not call the payload store:
// one inside a database transaction (the platform's own events, written in
// the business transaction of the use case that raised them). Every row
// holds its bytes, whatever the threshold; a payload over InlineMaxBytes is
// refused (ErrTooLarge), as is one that is not text (ErrNotText). Nothing is
// written anywhere.
func (s *Service) PrepareInline(items []Item) []Prepared {
	out := make([]Prepared, len(items))
	for i, it := range items {
		n := len(it.Data)
		if n > s.cfg.InlineMaxBytes {
			out[i].Err = fmt.Errorf("%w: %d bytes, the most kept in the database is %d", ErrTooLarge, n, s.cfg.InlineMaxBytes)
			continue
		}
		if err := checkText(it.Data); err != nil {
			out[i].Err = err
			continue
		}
		out[i].Row = Row{ID: s.newID(), CreatedAt: it.CreatedAt, Data: it.Data, Size: n}
	}
	return out
}

// checkText refuses a payload msg_payloads.data (TEXT) cannot hold: bytes
// that are not valid UTF-8, or a NUL byte. Checked for an offloaded payload
// too, so whether a payload is accepted never depends on its size.
func checkText(b []byte) error {
	if !utf8.Valid(b) {
		return fmt.Errorf("%w: not valid UTF-8", ErrNotText)
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return fmt.Errorf("%w: contains a NUL byte", ErrNotText)
	}
	return nil
}

// writeSegment compresses the payloads items[idx...] one after the other into
// one segment, writes it under key and returns each payload's reference.
func (s *Service) writeSegment(ctx context.Context, key string, items []Item, idx []int) ([]Ref, error) {
	total := 0
	for _, i := range idx {
		total += len(items[i].Data)
	}
	// Compressed JSON is smaller than its source; the buffer grows if not.
	seg := make([]byte, 0, total/2+64)
	refs := make([]Ref, len(idx))
	for j, i := range idx {
		off := len(seg)
		var err error
		seg, err = encodeAppend(s.cfg.Codec, seg, items[i].Data)
		if err != nil {
			return nil, err
		}
		refs[j] = Ref{
			Key:    key,
			Offset: int64(off),
			Length: int64(len(seg) - off),
			Codec:  s.cfg.Codec,
			CRC32C: crc32.Checksum(seg[off:], castagnoli),
		}
	}
	start := time.Now()
	err := s.store.Put(ctx, key, seg)
	storePutSeconds.Observe(time.Since(start).Seconds())
	if err != nil {
		storeErrors.WithLabelValues(opPut).Inc()
		s.health.failure(s.now(), HealthOpPut)
		return nil, err
	}
	s.health.success(s.now())
	return refs, nil
}

// storeFailures collects the failed segment writes of one Prepare, so they
// are logged once.
type storeFailures struct {
	segments   int
	keptInline int
	refused    int
	first      error
}

func (f *storeFailures) add(err error) {
	f.segments++
	if f.first == nil {
		f.first = err
	}
}

func (f *storeFailures) log() {
	if f.segments == 0 {
		return
	}
	slog.Warn("payload store: segment write failed; payloads kept in the database where they fit",
		"segments_failed", f.segments, "kept_inline", f.keptInline, "refused", f.refused, "err", f.first)
}
