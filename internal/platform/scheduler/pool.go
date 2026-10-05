package scheduler

// PoolRuntimeParams are the session settings every connection of the scheduler's
// OWN database pool sets at connect time (pgx RuntimeParams) — and of no other
// pool:
//
//	plan_cache_mode = force_custom_plan   a prepared statement is re-planned with
//	                                      its actual arguments each time. pgx caches
//	                                      prepared statements per connection, and
//	                                      after five executions Postgres may switch
//	                                      to a cached generic plan; one made while
//	                                      the queue was empty is a sequential scan
//	                                      and was reused after a burst (850 ms per
//	                                      claim at 200,000 rows) until the next
//	                                      autoanalyze.
//	enable_sort = off                     the claim's ORDER BY is served by
//	                                      idx_dispatch_queue_order; without a statistics
//	                                      view that trusts the index the planner picks
//	                                      a sequential scan and a sort of the queue.
//	                                      (This only penalises sort nodes; statements
//	                                      that have no ordered path still sort.)
//
// Measured on Postgres (six statistics states x three queue sizes x three plan
// modes): the two-statement claim with these settings never took longer than
// 34 ms (200,000 rows, drained-then-analysed queue) and otherwise 0.1-0.3 ms per
// statement. The plan tests (poller_plan_pg_test.go) open their connections with
// the same settings; internal/server applies them to the scheduler's pool.
var PoolRuntimeParams = map[string]string{
	"plan_cache_mode": "force_custom_plan",
	"enable_sort":     "off",
}
