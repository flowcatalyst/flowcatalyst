package tsid

import (
	"regexp"
	"sync"
	"testing"
	"time"
)

func TestGenerate(t *testing.T) {
	id := Generate()

	if id == "" {
		t.Error("Generate() returned empty string")
	}

	// TSID should be 13 characters in Crockford Base32
	if len(id) != 13 {
		t.Errorf("Generate() returned ID of length %d, expected 13", len(id))
	}

	// Should only contain valid Crockford Base32 characters (uppercase)
	valid := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]+$`)
	if !valid.MatchString(id) {
		t.Errorf("Generate() returned invalid Crockford Base32: %s", id)
	}
}

func TestGenerateUniqueness(t *testing.T) {
	ids := make(map[string]bool)
	count := 10000

	for i := 0; i < count; i++ {
		id := Generate()
		if ids[id] {
			t.Errorf("Generate() produced duplicate ID: %s", id)
		}
		ids[id] = true
	}
}

func TestGenerateConcurrent(t *testing.T) {
	ids := sync.Map{}
	var wg sync.WaitGroup
	goroutines := 10
	idsPerGoroutine := 1000

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < idsPerGoroutine; i++ {
				id := Generate()
				if _, loaded := ids.LoadOrStore(id, true); loaded {
					t.Errorf("Generate() produced duplicate ID in concurrent test: %s", id)
				}
			}
		}()
	}

	wg.Wait()

	// Count total unique IDs
	count := 0
	ids.Range(func(_, _ interface{}) bool {
		count++
		return true
	})

	expected := goroutines * idsPerGoroutine
	if count != expected {
		t.Errorf("Expected %d unique IDs, got %d", expected, count)
	}
}

func TestGenerateSortable(t *testing.T) {
	// Generate IDs with time gaps to verify they sort correctly
	// TSIDs are sortable at the millisecond granularity, not sub-millisecond
	ids := make([]string, 10)
	for i := 0; i < 10; i++ {
		ids[i] = Generate()
		time.Sleep(2 * time.Millisecond) // Ensure different timestamps
	}

	// Each subsequent ID should be > previous (lexicographically)
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("IDs not sortable: %s came after %s", ids[i], ids[i-1])
		}
	}
}

func BenchmarkGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Generate()
	}
}

func BenchmarkGenerateParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Generate()
		}
	})
}
