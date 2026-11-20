package payloadstore

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_DisabledAtZero(t *testing.T) {
	c := newCache(0)
	assert.Nil(t, c)
	c.put("a", []byte("x"))
	_, ok := c.get("a")
	assert.False(t, ok)
}

// The cache holds at most its bound in payload bytes, evicting the least
// recently used.
func TestCache_ByteBoundAndLRU(t *testing.T) {
	c := newCache(800) // entries up to 100 bytes
	for i := range 8 {
		c.put(fmt.Sprintf("p%d", i), bytes.Repeat([]byte{'a'}, 100))
	}
	used, n := c.size()
	assert.Equal(t, int64(800), used)
	assert.Equal(t, 8, n)

	_, ok := c.get("p0") // p0 is now the most recently used; p1 the least
	require.True(t, ok)
	c.put("p8", bytes.Repeat([]byte{'b'}, 100))
	used, n = c.size()
	assert.Equal(t, int64(800), used)
	assert.Equal(t, 8, n)
	_, ok = c.get("p1")
	assert.False(t, ok, "the least recently used went")
	_, ok = c.get("p0")
	assert.True(t, ok)

	// A smaller entry evicts only as much as it needs.
	c.put("small", []byte("12345"))
	used, _ = c.size()
	assert.LessOrEqual(t, used, int64(800))
}

func TestCache_LargerThanAnEighthIsNotCached(t *testing.T) {
	c := newCache(800)
	c.put("exact", bytes.Repeat([]byte{'a'}, 100))
	c.put("over", bytes.Repeat([]byte{'a'}, 101))
	_, ok := c.get("exact")
	assert.True(t, ok, "an eighth exactly is cached")
	_, ok = c.get("over")
	assert.False(t, ok, "one byte more is not")
}

func TestCache_KeepsItsOwnCopies(t *testing.T) {
	c := newCache(1 << 10)
	in := []byte("payload")
	c.put("p", in)
	in[0] = 'X'
	out, ok := c.get("p")
	require.True(t, ok)
	assert.Equal(t, []byte("payload"), out)
	out[0] = 'Y'
	again, _ := c.get("p")
	assert.Equal(t, []byte("payload"), again)
}

// Run with -race.
func TestCache_Concurrent(t *testing.T) {
	c := newCache(16 << 10)
	var wg sync.WaitGroup
	for g := range 16 {
		wg.Go(func() {
			for i := range 500 {
				id := fmt.Sprintf("p%d", (g*31+i)%97)
				want := bytes.Repeat([]byte{byte('a' + (g*31+i)%97%26)}, 100+(g*31+i)%97)
				c.put(id, want)
				if got, ok := c.get(id); ok {
					assert.Equal(t, len(want), len(got))
				}
			}
		})
	}
	wg.Wait()
	used, _ := c.size()
	assert.LessOrEqual(t, used, int64(16<<10))
}
