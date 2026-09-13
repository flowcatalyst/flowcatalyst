package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
)

// fakeSQS records every send and can be told to fail specific entries.
type fakeSQS struct {
	sends   []*sqs.SendMessageBatchInput
	created []string
	// failIDs are entry ids SQS reports as failed, per send.
	failIDs map[string]bool
	// missingUntilCreated makes the first send to each queue fail with
	// QueueDoesNotExist until CreateQueue is called for it.
	missingUntilCreated map[string]bool
	// sendErr, when set, fails every send outright.
	sendErr error
}

func (f *fakeSQS) SendMessageBatch(_ context.Context, in *sqs.SendMessageBatchInput, _ ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error) {
	f.sends = append(f.sends, in)
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	if f.missingUntilCreated[*in.QueueUrl] {
		return nil, &sqstypes.QueueDoesNotExist{}
	}
	out := &sqs.SendMessageBatchOutput{}
	for _, e := range in.Entries {
		if f.failIDs[*e.Id] {
			out.Failed = append(out.Failed, sqstypes.BatchResultErrorEntry{Id: e.Id})
			continue
		}
		out.Successful = append(out.Successful, sqstypes.SendMessageBatchResultEntry{Id: e.Id})
	}
	return out, nil
}

func (f *fakeSQS) CreateQueue(_ context.Context, in *sqs.CreateQueueInput, _ ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error) {
	f.created = append(f.created, *in.QueueName)
	for url := range f.missingUntilCreated {
		if strings.HasSuffix(url, "/"+*in.QueueName) {
			f.missingUntilCreated[url] = false
		}
	}
	return &sqs.CreateQueueOutput{}, nil
}

// stubDestinations routes by the item's client id, so a test can put jobs on
// different queues without a database behind the real resolver.
type stubDestinations struct {
	byClient map[string]string
	err      map[string]error
}

func (s stubDestinations) Destination(_ context.Context, item PublishItem) (string, error) {
	if err := s.err[item.JobID]; err != nil {
		return "", err
	}
	if name, ok := s.byClient[item.ClientID]; ok {
		return name, nil
	}
	return "FC-test-platform-DEFAULT.fifo", nil
}

func testSQSPublisher(client sqsSendClient, dest destinationResolver) *SQSDispatchPublisher {
	return &SQSDispatchPublisher{
		client: client,
		settings: dispatch.Settings{
			SQS: true, Prefix: "FC-test", SQSAccountID: "123456789012", SQSRegion: "eu-west-1",
		},
		destinations: dest,
	}
}

// item builds a claimed job bound for group (empty = group-less).
func item(jobID, clientID, group string) PublishItem {
	m := common.Message{ID: jobID}
	if group != "" {
		g := group
		m.MessageGroupID = &g
	}
	return PublishItem{JobID: jobID, ClientID: clientID, Message: m}
}

func entryIDs(in *sqs.SendMessageBatchInput) []string {
	ids := make([]string, 0, len(in.Entries))
	for _, e := range in.Entries {
		ids = append(ids, *e.Id)
	}
	return ids
}

func TestSQSPublish_ChunksAtTenAndPartitionsByQueue(t *testing.T) {
	f := &fakeSQS{}
	dest := stubDestinations{byClient: map[string]string{
		"acme":   "FC-test-acme-DEFAULT.fifo",
		"globex": "FC-test-globex-DEFAULT.fifo",
	}}
	p := testSQSPublisher(f, dest)

	// 12 group-less acme jobs (so they pack fully) + 1 globex job.
	var items []PublishItem
	for i := range 12 {
		items = append(items, item(string(rune('a'+i)), "acme", ""))
	}
	items = append(items, item("z", "globex", ""))

	unpublished, err := p.Publish(context.Background(), items)
	if err != nil || unpublished != nil {
		t.Fatalf("Publish = %v, %v; want a clean run", unpublished, err)
	}
	if len(f.sends) != 3 {
		t.Fatalf("sends = %d, want 3 (10 + 2 for acme, 1 for globex)", len(f.sends))
	}
	if got := len(f.sends[0].Entries); got != 10 {
		t.Errorf("first chunk = %d entries, want SQS's cap of 10", got)
	}
	if got := len(f.sends[1].Entries); got != 2 {
		t.Errorf("second chunk = %d entries, want the remainder", got)
	}
	// One call addresses one queue.
	if *f.sends[2].QueueUrl != "https://sqs.eu-west-1.amazonaws.com/123456789012/FC-test-globex-DEFAULT.fifo" {
		t.Errorf("third chunk went to %q", *f.sends[2].QueueUrl)
	}
	// Claim order is preserved within a destination.
	if got := entryIDs(f.sends[0]); got[0] != "a" || got[9] != "j" {
		t.Errorf("first chunk = %v, want claim order a..j", got)
	}
}

// Two jobs of one message group must never share a SendMessageBatch call: SQS
// reports failures per entry, so an earlier failure with a later success would
// deliver the group out of order.
func TestSQSPublish_NeverTwoOfOneGroupInAChunk(t *testing.T) {
	f := &fakeSQS{}
	p := testSQSPublisher(f, stubDestinations{})

	items := []PublishItem{
		item("j1", "acme", "g1"),
		item("j2", "acme", "g1"),
		item("j3", "acme", "g1"),
	}
	if _, err := p.Publish(context.Background(), items); err != nil {
		t.Fatalf("Publish errored: %v", err)
	}
	if len(f.sends) != 3 {
		t.Fatalf("sends = %d, want one per group member", len(f.sends))
	}
	for i, s := range f.sends {
		if len(s.Entries) != 1 {
			t.Errorf("send %d carried %d entries, want 1", i, len(s.Entries))
		}
	}
}

// A group-less job is its own group, so group-less jobs still pack fully — and
// each gets a MessageGroupId, which FIFO requires.
func TestSQSPublish_GrouplessJobsUseTheirOwnJobIDAsGroup(t *testing.T) {
	f := &fakeSQS{}
	p := testSQSPublisher(f, stubDestinations{})

	items := []PublishItem{item("j1", "acme", ""), item("j2", "acme", "")}
	if _, err := p.Publish(context.Background(), items); err != nil {
		t.Fatalf("Publish errored: %v", err)
	}
	if len(f.sends) != 1 {
		t.Fatalf("sends = %d, want group-less jobs to pack into one chunk", len(f.sends))
	}
	for _, e := range f.sends[0].Entries {
		if e.MessageGroupId == nil || *e.MessageGroupId != *e.Id {
			t.Errorf("entry %q group = %v, want its own job id", *e.Id, e.MessageGroupId)
		}
	}
}

// Ruling R2: the dedup id is never the bare job id, and never repeats across
// calls — otherwise SQS silently drops a stale-recovery re-publish inside its
// five-minute dedup window, stranding the job for good.
func TestSQSPublish_DedupIDIsUniquePerPublishCall(t *testing.T) {
	f := &fakeSQS{}
	p := testSQSPublisher(f, stubDestinations{})

	items := []PublishItem{item("j1", "acme", "")}
	if _, err := p.Publish(context.Background(), items); err != nil {
		t.Fatalf("first Publish errored: %v", err)
	}
	if _, err := p.Publish(context.Background(), items); err != nil {
		t.Fatalf("second Publish errored: %v", err)
	}
	first := *f.sends[0].Entries[0].MessageDeduplicationId
	second := *f.sends[1].Entries[0].MessageDeduplicationId
	if first == "j1" || second == "j1" {
		t.Fatal("dedup id is the bare job id; a re-publish would be silently dropped")
	}
	if !strings.HasPrefix(first, "j1:") {
		t.Errorf("dedup id = %q, want it to start with the job id", first)
	}
	if first == second {
		t.Error("the same job got the same dedup id on two publish calls")
	}
}

// The queue is created on first use, and the SAME request is retried with the
// SAME dedup ids: the first attempt never reached a queue that could have
// deduplicated anything, so it is still one publish attempt.
func TestSQSPublish_CreatesQueueLazilyAndRetriesOnce(t *testing.T) {
	url := "https://sqs.eu-west-1.amazonaws.com/123456789012/FC-test-platform-DEFAULT.fifo"
	f := &fakeSQS{missingUntilCreated: map[string]bool{url: true}}
	p := testSQSPublisher(f, stubDestinations{})

	unpublished, err := p.Publish(context.Background(), []PublishItem{item("j1", "", "")})
	if err != nil || unpublished != nil {
		t.Fatalf("Publish = %v, %v; want success after the lazy create", unpublished, err)
	}
	if len(f.created) != 1 || f.created[0] != "FC-test-platform-DEFAULT.fifo" {
		t.Fatalf("created = %v, want the missing queue created once", f.created)
	}
	if len(f.sends) != 2 {
		t.Fatalf("sends = %d, want the original send plus one retry", len(f.sends))
	}
	if *f.sends[0].Entries[0].MessageDeduplicationId != *f.sends[1].Entries[0].MessageDeduplicationId {
		t.Error("the retry used a fresh dedup id; it is the same attempt and must reuse it")
	}
}

// Ruling O2: only the jobs SQS actually rejected revert. Reverting the whole
// batch would republish jobs the broker accepted, delivering them twice.
func TestSQSPublish_RevertsOnlyTheFailedEntries(t *testing.T) {
	f := &fakeSQS{failIDs: map[string]bool{"j2": true}}
	p := testSQSPublisher(f, stubDestinations{})

	items := []PublishItem{item("j1", "", ""), item("j2", "", ""), item("j3", "", "")}
	unpublished, err := p.Publish(context.Background(), items)
	if err == nil {
		t.Fatal("Publish succeeded, want an error reporting the partial failure")
	}
	if len(unpublished) != 1 || unpublished[0] != "j2" {
		t.Errorf("unpublished = %v, want exactly the rejected job", unpublished)
	}
}

// A failed group poisons its own later jobs: they are reported unpublished
// WITHOUT being sent, so the group reverts together and keeps its order.
func TestSQSPublish_FailedGroupPoisonsItsLaterJobs(t *testing.T) {
	f := &fakeSQS{failIDs: map[string]bool{"j1": true}}
	p := testSQSPublisher(f, stubDestinations{})

	// Same group: rule 1 puts them in separate chunks, so j2 is only
	// considered after j1's failure is known.
	items := []PublishItem{item("j1", "", "g1"), item("j2", "", "g1"), item("other", "", "g2")}
	unpublished, err := p.Publish(context.Background(), items)
	if err == nil {
		t.Fatal("Publish succeeded, want an error")
	}
	want := map[string]bool{"j1": true, "j2": true}
	if len(unpublished) != 2 || !want[unpublished[0]] || !want[unpublished[1]] {
		t.Fatalf("unpublished = %v, want the failed job and its poisoned sibling", unpublished)
	}
	// j2 must never have reached SQS.
	for _, s := range f.sends {
		for _, id := range entryIDs(s) {
			if id == "j2" {
				t.Error("j2 was sent after its group had already failed; the group can now arrive out of order")
			}
		}
	}
	// An unrelated group is unaffected.
	var sentOther bool
	for _, s := range f.sends {
		for _, id := range entryIDs(s) {
			if id == "other" {
				sentOther = true
			}
		}
	}
	if !sentOther {
		t.Error("an unrelated group was stranded by another group's failure")
	}
}

// A chunk that throws marks its own jobs unpublished, and every other
// destination is still attempted: one client's broken queue must not strand
// every other client's jobs in the same claimed batch.
func TestSQSPublish_ChunkErrorDoesNotAbandonOtherQueues(t *testing.T) {
	f := &fakeSQS{sendErr: errors.New("throttled")}
	p := testSQSPublisher(f, stubDestinations{byClient: map[string]string{
		"acme":   "FC-test-acme-DEFAULT.fifo",
		"globex": "FC-test-globex-DEFAULT.fifo",
	}})

	items := []PublishItem{item("j1", "acme", ""), item("j2", "globex", "")}
	unpublished, err := p.Publish(context.Background(), items)
	if err == nil {
		t.Fatal("Publish succeeded, want the send failure reported")
	}
	if len(unpublished) != 2 {
		t.Errorf("unpublished = %v, want both jobs", unpublished)
	}
	if len(f.sends) != 2 {
		t.Errorf("sends = %d, want the second queue attempted despite the first failing", len(f.sends))
	}
}

// A destination that cannot be composed (an over-long tenant, say) costs that
// job alone, not the batch.
func TestSQSPublish_UnresolvableDestinationCostsOnlyThatJob(t *testing.T) {
	f := &fakeSQS{}
	p := testSQSPublisher(f, stubDestinations{err: map[string]error{"bad": errors.New("name too long")}})

	items := []PublishItem{item("good", "", ""), item("bad", "", "")}
	unpublished, err := p.Publish(context.Background(), items)
	if err == nil {
		t.Fatal("Publish succeeded, want the unresolvable job reported")
	}
	if len(unpublished) != 1 || unpublished[0] != "bad" {
		t.Errorf("unpublished = %v, want only the unresolvable job", unpublished)
	}
	if len(f.sends) != 1 || len(f.sends[0].Entries) != 1 {
		t.Errorf("the resolvable job was not published on its own")
	}
}
