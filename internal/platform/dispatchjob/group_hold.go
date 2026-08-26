package dispatchjob

// GroupHoldingStatusSQL matches a job that is HOLDING its message group: one
// that has not got through and is not about to on its own.
//
//   - FAILED / ERROR — terminal failure, held until an operator resolves it.
//     ('ERROR' is a legacy value that predates the current status set; it is
//     matched so old rows keep blocking as they always did.)
//   - PENDING with a future scheduled_for — sitting out a retry backoff. This
//     one is easy to miss, because such a job looks idle from every angle: it
//     is excluded from the claim query by its own scheduled_for and it is not
//     FAILED, so nothing used to treat it as holding anything. Its successors
//     were therefore claimed and delivered while it waited, which is exactly
//     the reordering BLOCK_ON_ERROR exists to prevent. A backed-off job still
//     owns its place at the front of the group.
//
// Deliberately NOT holding: QUEUED and PROCESSING. Those are the normal flow —
// the poller hands a group's whole eligible run to the router in one batch and
// the router's per-group FIFO delivers them in order. Treating them as holders
// would reduce every ordered group to one job per poll cycle.
//
// Used by the scheduler's claim-time hold-back and by the delivery-time check
// in the processing endpoint; both must agree, or a job held at one gate and
// waved through at the other would loop.
const GroupHoldingStatusSQL = `status IN ('FAILED', 'ERROR') ` +
	`OR (status = 'PENDING' AND scheduled_for IS NOT NULL AND scheduled_for > NOW())`
