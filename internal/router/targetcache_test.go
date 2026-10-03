package router

import (
	"fmt"
	"sync"
	"testing"
)

func TestCachedTargetKeysMatchUncached(t *testing.T) {
	for _, target := range []string{
		"https://api.example.com/hook?tenant=1#frag",
		"http://host:8080/a",
		"http:///nohost",
		"ftp://example.com/x",
		"http://example.com:99999/x",
	} {
		for i := 0; i < 2; i++ { // second pass hits the cache
			if got, want := cachedBreakerKey(target), breakerKey(target); got != want {
				t.Fatalf("breakerKey(%q) = %q, want %q", target, got, want)
			}
			gk, gerr := cachedHostKey(target)
			wk, werr := HostKeyFromURL(target)
			if gk != wk || (gerr == nil) != (werr == nil) || (gerr != nil && gerr.Error() != werr.Error()) {
				t.Fatalf("hostKey(%q) = %v,%v want %v,%v", target, gk, gerr, wk, werr)
			}
		}
	}
}

func TestTargetCacheBoundedAndConcurrentSafe(t *testing.T) {
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < maxTargetCacheEntries; i++ {
				target := fmt.Sprintf("http://h%d.example.com/p", i)
				if got := cachedBreakerKey(target); got != target {
					t.Errorf("breakerKey mismatch %q", got)
				}
				if k, err := cachedHostKey(target); err != nil || k.Port != 80 {
					t.Errorf("hostKey wrong: %v %v", k, err)
				}
			}
		}()
	}
	wg.Wait()
	for _, c := range []*counterBox{&breakerKeyCount, &hostKeyCount} {
		if c.n > maxTargetCacheEntries {
			t.Fatalf("cache grew past cap: %d", c.n)
		}
	}
	// Past the cap: still correct, not stored.
	extra := "http://overflow.example.com/x"
	if cachedBreakerKey(extra) != extra {
		t.Fatal("overflow target wrong")
	}
	if _, ok := breakerKeyCache.Load(extra); ok {
		t.Fatal("overflow target should not be stored once cap reached")
	}
}
