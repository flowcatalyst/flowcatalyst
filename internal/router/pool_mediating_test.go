package router

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// blockingMediator parks every delivery until release is closed, so a test can
// observe the pool mid-flight.
type blockingMediator struct {
	entered chan struct{}
	release chan struct{}
}

func newBlockingMediator(n int) *blockingMediator {
	return &blockingMediator{entered: make(chan struct{}, n), release: make(chan struct{})}
}

func (b *blockingMediator) Mediate(context.Context, *common.Message) common.MediationOutcome {
	b.entered <- struct{}{}
	<-b.release
	return common.Success(http.StatusOK)
}

func (b *blockingMediator) awaitEntered(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-b.entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of %d workers reached the mediator", i, n)
		}
	}
}

// ActiveWorkers is the size of the mediating set rather than a counter kept
// alongside it, so the dashboard's "how many are being delivered" and its list
// of what they are can never disagree.
func TestActiveWorkersIsTheMediatingSetSize(t *testing.T) {
	const workers = 4
	med := newBlockingMediator(workers)
	c := &grConsumer{id: "q1"}
	p := grPool(med, c)

	assert.Equal(t, uint32(0), p.ActiveWorkers())

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p.processOne(context.Background(), grMsg(string(rune('a'+i)), "http://t/x"))
		}(i)
	}
	med.awaitEntered(t, workers)

	assert.Equal(t, uint32(workers), p.ActiveWorkers())
	assert.Len(t, p.MediatingSnapshot(), workers, "the count and the list are the same fact")

	close(med.release)
	wg.Wait()
	assert.Equal(t, uint32(0), p.ActiveWorkers(), "every worker must leave the set on exit")
	assert.Empty(t, p.MediatingSnapshot())
}

// The mediating set is keyed per worker, not per message id. Two copies of one
// message can briefly sit in two workers (that is what the process-time dedup
// backstop is for), and id-keying would then under-report the count and let the
// loser's exit delete the winner's entry.
func TestMediatingCountsBothCopiesOfOneMessage(t *testing.T) {
	med := newBlockingMediator(2)
	c := &grConsumer{id: "q1"}
	// tracker nil: this test is about the accounting, not the dedup that
	// normally prevents the overlap.
	p := NewPool(common.PoolConfig{Code: "TEST", Concurrency: 8}, med, nil,
		func(string) queue.Consumer { return c })

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.processOne(context.Background(), grMsg("evt_same", "http://t/x"))
		}()
	}
	med.awaitEntered(t, 2)

	assert.Equal(t, uint32(2), p.ActiveWorkers(), "both workers must be counted")
	require.Len(t, p.MediatingSnapshot(), 2)

	close(med.release)
	wg.Wait()
	assert.Equal(t, uint32(0), p.ActiveWorkers())
}
