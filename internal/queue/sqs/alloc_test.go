package sqs

import (
	"context"
	"sync"
	"testing"
	"time"

	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// TestDeleteAllocs measures the per-ack cost of deleteBatcher.delete with a
// stub drainer (no AWS client), so only the enqueue/wait overhead is counted.
func TestDeleteAllocs(t *testing.T) {
	q := newGuardQueue()
	b := &q.del
	b.init()
	b.startOnce.Do(func() {})
	b.helpers.Store(deleteMaxHelpers) // no real helpers either // no real drainer
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case it := <-b.items:
				it.finish(q, nil)
			case <-stop:
				return
			}
		}
	}()
	defer func() { close(stop); <-done }()

	ctx := context.Background()
	run := func() {
		if err := b.delete(ctx, q, "receipt-handle", nil); err != nil {
			t.Fatal(err)
		}
	}
	run()
	got := testing.AllocsPerRun(2000, run)
	t.Logf("deleteBatcher.delete allocs/op = %v", got)
	if got > 1 && !raceEnabled {
		t.Fatalf("allocs/op = %v", got)
	}
}

func TestParseMessageAllocs(t *testing.T) {
	q := newGuardQueue()
	body := `{"id":"0HZXY1234ABCD","poolCode":"P","mediationType":"HTTP","mediationTarget":"http://example.com/hook","messageGroupId":"g1","dispatchMode":"IMMEDIATE"}`
	id, rh := "mid-1", "receipt-handle-1"
	sm := sqstypes.Message{Body: &body, MessageId: &id, ReceiptHandle: &rh}
	if _, _, _, err := q.parseMessage(sm); err != nil {
		t.Fatal(err)
	}
	got := testing.AllocsPerRun(2000, func() { _, _, _, _ = q.parseMessage(sm) })
	t.Logf("parseMessage allocs/op = %v", got)
	if got > 2 && !raceEnabled {
		t.Fatalf("parseMessage allocs/op = %v", got)
	}
}

// TestDeleteItemReuseUnderCancellation hammers delete with callers that give
// up at random while a slow drainer answers; run with -race. A recycled item
// that a drainer still held would answer the wrong caller or race.
func TestDeleteItemReuseUnderCancellation(t *testing.T) {
	q := newGuardQueue()
	b := &q.del
	b.init()
	b.startOnce.Do(func() {})
	b.helpers.Store(deleteMaxHelpers) // no real helpers either
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case it := <-b.items:
				time.Sleep(50 * time.Microsecond)
				if it.receipt == "" {
					panic("answered a reset item")
				}
				it.finish(q, nil)
			case <-stop:
				return
			}
		}
	}()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i%7)*30*time.Microsecond)
				_ = b.delete(ctx, q, "r", nil)
				cancel()
			}
		}()
	}
	wg.Wait()
	// Let the drainer finish what is queued before it is stopped.
	for len(b.items) > 0 {
		time.Sleep(time.Millisecond)
	}
	close(stop)
	<-done
}
