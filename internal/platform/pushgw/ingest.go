package pushgw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"github.com/flowcatalyst/framework/go/fw"
	"github.com/flowcatalyst/framework/go/metrics"
	"github.com/flowcatalyst/framework/go/push"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// PermIngest is the permission of POST /api/push/batch (spec 6.2): the counterpart of
// platform:messaging:batch:events-write. Four segments, like every platform permission.
const PermIngest = "platform:messaging:batch:push-write"

// IngestPath is the ingest endpoint. It is under the gateway's mount, so a load balancer that
// sends /api/push/* to a gateway service sends ingest there too.
const IngestPath = BasePath + "/push/batch"

// MaxBatchItems bounds one ingest request, as for the other batch endpoints.
const MaxBatchItems = 1000

// maxPushIDLength is the width of push_ingest_ids.push_id.
const maxPushIDLength = 64

// Item results of the batch answer, besides the outbox statuses.
const (
	statusSuccess    = "SUCCESS"
	statusBadRequest = "BAD_REQUEST"
	statusForbidden  = "FORBIDDEN"

	codeTopicNotOwned     = "TOPIC_NOT_OWNED"
	codeApplicationUnkown = "APPLICATION_UNKNOWN"
	codePushIDInvalid     = "PUSH_ID_INVALID"
	codeDuplicate         = "DUPLICATE"
)

// PushItem is one push of an ingest request: the payload the SDKs' PUSH outbox row carries
// (spec 6.1).
type PushItem struct {
	Topic      string          `json:"topic,omitempty" doc:"{application}:{segment}(:{segment})*; the first segment is the application, which the caller must be allowed to push for"`
	Type       string          `json:"type,omitempty" doc:"[a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*, at most 100 characters; reset, auth-expired, subscribed and fw.* are reserved"`
	Data       json.RawMessage `json:"data,omitempty" doc:"A JSON object, at most the application's data limit (16 KiB by default)"`
	ClientCode *string         `json:"clientCode,omitempty" doc:"The client the push is for, by identifier; the credential must reach it. Neither clientCode nor clientId: the credential's only client, or the platform client for an anchor."`
	ClientID   *string         `json:"clientId,omitempty" doc:"The client the push is for, by id; wins over clientCode"`
	PushID     string          `json:"pushId,omitempty" doc:"Idempotency key, at most 64 characters: a push whose pushId the application already sent is answered SUCCESS and written once"`
}

// PushBatchRequest is the wire body of POST /api/push/batch.
type PushBatchRequest struct {
	Items []PushItem `json:"items"`
}

// PushResult is one item's outcome: positional, one per item, the shape the outbox processor
// maps (status is SUCCESS, BAD_REQUEST, FORBIDDEN, ...).
type PushResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Code   string `json:"code,omitempty"`
}

// PushBatchResponse is the answer of POST /api/push/batch.
type PushBatchResponse struct {
	Results []PushResult `json:"results"`
}

// Ingestor serves POST /api/push/batch.
type Ingestor struct {
	// Pool is the platform pool: the push_ingest_ids rows and the pushes are written in one
	// transaction on it (the module's tables are in the same database).
	Pool    *pgxpool.Pool
	Apps    *application.Repository
	Clients *client.Repository
	Store   *Store
	// Pushes returns the module the pushes are written with.
	Pushes func() *push.Pushes
	// DefaultMaxData is push.max-data-bytes, the data limit of an application without its own.
	DefaultMaxData int
	// Items and Rejected count the outcome per application (spec 11); either may be nil.
	Items, Rejected *metrics.Counter
	Log             *slog.Logger
	Now             func() time.Time

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func (in *Ingestor) now() time.Time {
	if in.Now != nil {
		return in.Now()
	}
	return time.Now()
}

// Register mounts POST /api/push/batch on the huma API.
func (in *Ingestor) Register(api huma.API) {
	g := apiroute.New(api, "push")
	apiroute.Post(g, "pushBatch", IngestPath,
		"Publish pushes to clients (push gateway ingest; the outbox processor's PUSH rows)", http.StatusOK, in.batch)
}

// rateLimited is the whole-batch refusal of an application that is paused or over its publish
// rate: 429 with Retry-After, so the outbox keeps the rows and sends them again.
type rateLimited struct {
	retryAfter time.Duration
	Code       string `json:"error"`
	Message    string `json:"message"`
}

func (e *rateLimited) Error() string  { return e.Message }
func (e *rateLimited) GetStatus() int { return http.StatusTooManyRequests }
func (e *rateLimited) GetHeaders() http.Header {
	secs := max(1, int((e.retryAfter+time.Second-1)/time.Second))
	return http.Header{"Retry-After": []string{strconv.Itoa(secs)}}
}

// pausedRetryAfter is the Retry-After of a paused application: how long its outbox waits before it
// asks again (an unpause is felt within this).
const pausedRetryAfter = 5 * time.Second

// admit decides whether an application may publish n pushes now: false with the time to wait
// when it is paused or over its rate (a token bucket per application and per gateway task, rate
// and burst from push_app_settings, default 500/s and 2,000).
func (in *Ingestor) admit(app string, n int) (wait time.Duration, code string, ok bool) {
	ps := in.Store.Publish(app)
	if ps.Paused {
		return pausedRetryAfter, "PUSH_PAUSED", false
	}
	in.mu.Lock()
	if in.limiters == nil {
		in.limiters = map[string]*rate.Limiter{}
	}
	lim := in.limiters[app]
	if lim == nil {
		lim = rate.NewLimiter(rate.Limit(ps.Rate), ps.Burst)
		in.limiters[app] = lim
	} else {
		if lim.Limit() != rate.Limit(ps.Rate) {
			lim.SetLimit(rate.Limit(ps.Rate))
		}
		if lim.Burst() != ps.Burst {
			lim.SetBurst(ps.Burst)
		}
	}
	in.mu.Unlock()
	n = min(n, ps.Burst) // a batch larger than the burst would never pass
	now := in.now()
	r := lim.ReserveN(now, n)
	if !r.OK() {
		return time.Second, "PUSH_RATE", false
	}
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now)
		return d, "PUSH_RATE", false
	}
	return 0, "", true
}

type pending struct {
	idx    int
	app    string
	appID  string
	push   push.Push
	cx     fw.Context
	pushID string
}

func (in *Ingestor) batch(ctx context.Context, req *apicommon.In[PushBatchRequest]) (*apicommon.Out[PushBatchResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, PermIngest); err != nil {
		return nil, err
	}
	items := req.Body.Items
	if len(items) > MaxBatchItems {
		return nil, httperror.BadRequest("BATCH_TOO_LARGE", fmt.Sprintf("max %d items per batch", MaxBatchItems))
	}
	results := make([]PushResult, len(items))
	scope := sdk.NewAppScope(in.Apps)
	codes := sdk.NewClientCodes(in.Clients)
	var ok []pending
	for i := range items {
		p, res, err := in.prepare(ctx, ac, scope, codes, i, &items[i])
		if err != nil {
			return nil, err
		}
		if res != nil {
			results[i] = *res
			in.count(p.app, res)
			continue
		}
		ok = append(ok, p)
	}

	// An application that is paused or over its rate answers the whole batch 429, before anything
	// is written: the outbox keeps its rows and sends them again.
	perApp := map[string]int{}
	var order []string
	for _, p := range ok {
		if perApp[p.app] == 0 {
			order = append(order, p.app)
		}
		perApp[p.app]++
	}
	for _, app := range order {
		if wait, code, allowed := in.admit(app, perApp[app]); !allowed {
			in.countRejected(app, code)
			return nil, &rateLimited{
				retryAfter: wait, Code: code,
				Message: "the application " + app + " is paused or over its push publish rate; send the batch again after the Retry-After",
			}
		}
	}

	written, err := in.write(ctx, ok)
	if err != nil {
		return nil, err
	}
	for j, p := range ok {
		res := PushResult{ID: p.pushID, Status: statusSuccess}
		if !written[j] {
			res.Code = codeDuplicate
		}
		results[p.idx] = res
		if written[j] {
			in.count(p.app, &res)
		} else {
			in.countItem(p.app, "duplicate")
		}
	}
	return &apicommon.Out[PushBatchResponse]{Body: PushBatchResponse{Results: results}}, nil
}

// prepare checks one item: the result is non-nil when the item is refused (never an error for the
// batch), or the pending push when it passes. The checks run in this order: topic, ownership of
// the application (spec 6.2.1), type and data, push id, client.
func (in *Ingestor) prepare(ctx context.Context, ac *auth.AuthContext, scope *sdk.AppScope, codes *sdk.ClientCodes, i int, it *PushItem) (pending, *PushResult, error) {
	p := pending{idx: i, pushID: it.PushID}
	bad := func(code, msg string) (pending, *PushResult, error) {
		return p, &PushResult{ID: it.PushID, Status: statusBadRequest, Error: msg, Code: code}, nil
	}
	if err := push.CheckTopic(it.Topic); err != nil {
		code, msg := pushRefusal(err)
		return bad(code, msg)
	}
	p.app = sdk.FirstSegment(it.Topic)

	// ownership: an application-scoped caller pushes for its own applications only; unknown and
	// inaccessible are indistinguishable to it
	canAccess, err := scope.CanAccessCode(ctx, ac, p.app)
	if err != nil {
		return p, nil, err
	}
	if !canAccess {
		return p, &PushResult{
			ID: it.PushID, Status: statusForbidden, Code: codeTopicNotOwned,
			Error: "the topic belongs to an application this credential cannot push for",
		}, nil
	}
	appID, err := scope.AppIDByCode(ctx, p.app)
	if err != nil {
		return p, nil, err
	}
	if appID == nil {
		return bad(codeApplicationUnkown, "no application has the code "+p.app)
	}
	p.appID = *appID

	pv, err := push.New(it.Topic, it.Type, []byte(it.Data))
	if err != nil {
		code, msg := pushRefusal(err)
		return bad(code, msg)
	}
	p.push = pv
	if limit := in.maxData(p.app); len(pv.Data()) > limit {
		return bad("DATA_TOO_LARGE", fmt.Sprintf("the data is %d bytes, more than %d (the limit of application %s)", len(pv.Data()), limit, p.app))
	}
	if len(it.PushID) > maxPushIDLength || (it.PushID != "" && strings.TrimSpace(it.PushID) == "") {
		return bad(codePushIDInvalid, fmt.Sprintf("pushId must not be blank and is at most %d characters", maxPushIDLength))
	}

	clientID, err := sdk.ResolveEventClient(ctx, ac, codes, it.ClientID, it.ClientCode)
	if err != nil {
		if httperror.Status(err) == http.StatusForbidden {
			return p, &PushResult{ID: it.PushID, Status: statusForbidden, Code: codeClientForbidden, Error: err.Error()}, nil
		}
		return p, nil, err
	}
	p.cx = fw.ContextFor(fw.PlatformClient)
	if clientID != nil {
		cid, perr := fw.ParseClientID(*clientID)
		if perr != nil {
			return p, &PushResult{ID: it.PushID, Status: statusForbidden, Code: codeClientForbidden, Error: "the client id is not usable for a push"}, nil
		}
		p.cx = fw.ContextFor(cid)
	}
	return p, nil, nil
}

// maxData is the data limit of an application: its own override, else push.max-data-bytes.
func (in *Ingestor) maxData(app string) int {
	if n := in.Store.AppLimits(app).MaxDataBytes; n > 0 {
		return min(n, push.MaxDataBytes)
	}
	if in.DefaultMaxData > 0 {
		return in.DefaultMaxData
	}
	return DefaultMaxData
}

// pushRefusal reads the code and message of a refused push (*push.InvalidPushError).
func pushRefusal(err error) (code, msg string) {
	if ipe, ok := errors.AsType[*push.InvalidPushError](err); ok {
		return ipe.Reason, ipe.Message
	}
	return "PUSH_INVALID", err.Error()
}

// write records the push ids and writes the pushes of one request in one transaction: either every
// new push and its id exist when it commits, or none. written[j] is false for a push whose id the
// application already sent (an earlier request, or earlier in this one): nothing is written for it.
func (in *Ingestor) write(ctx context.Context, ok []pending) ([]bool, error) {
	written := make([]bool, len(ok))
	if len(ok) == 0 {
		return written, nil
	}
	err := pgx.BeginTxFunc(ctx, in.Pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		fresh, err := recordPushIDs(ctx, tx, ok)
		if err != nil {
			return err
		}
		var pubs []push.Publication
		for j, p := range ok {
			written[j] = fresh[j]
			if fresh[j] {
				pubs = append(pubs, push.Publication{Context: p.cx, Push: p.push})
			}
		}
		return in.Pushes().PublishBatch(ctx, tx, pubs...)
	})
	if err != nil {
		if ipe, isInvalid := errors.AsType[*push.InvalidPushError](err); isInvalid {
			return nil, httperror.BadRequest(ipe.Reason, ipe.Message)
		}
		return nil, usecase.Internal("PUSH", "writing the pushes failed", err)
	}
	return written, nil
}

// recordPushIDs inserts the push ids of the items that carry one into push_ingest_ids and reports
// per item whether it is new. An item without an id is always new. The first of two items with
// the same id in one request is the new one.
func recordPushIDs(ctx context.Context, tx pgx.Tx, ok []pending) ([]bool, error) {
	fresh := make([]bool, len(ok))
	var apps, ids []string
	var slot []int
	seen := map[[2]string]bool{}
	for j, p := range ok {
		if p.pushID == "" {
			fresh[j] = true
			continue
		}
		key := [2]string{p.appID, p.pushID}
		if seen[key] {
			continue
		}
		seen[key] = true
		apps, ids, slot = append(apps, p.appID), append(ids, p.pushID), append(slot, j)
	}
	if len(ids) == 0 {
		return fresh, nil
	}
	rows, err := tx.Query(ctx, `
		INSERT INTO push_ingest_ids (application_id, push_id)
		SELECT u.application_id, u.push_id FROM unnest($1::text[], $2::text[]) AS u(application_id, push_id)
		ON CONFLICT DO NOTHING
		RETURNING application_id, push_id`, apps, ids)
	if err != nil {
		return nil, fmt.Errorf("record push ids: %w", err)
	}
	defer rows.Close()
	inserted := map[[2]string]bool{}
	for rows.Next() {
		var a, id string
		if err := rows.Scan(&a, &id); err != nil {
			return nil, err
		}
		inserted[[2]string{a, id}] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for k, j := range slot {
		fresh[j] = inserted[[2]string{apps[k], ids[k]}]
	}
	return fresh, nil
}

func (in *Ingestor) count(app string, res *PushResult) {
	switch res.Status {
	case statusSuccess:
		in.countItem(app, "accepted")
	default:
		in.countItem(app, "refused")
		in.countRejected(app, res.Code)
	}
}

func (in *Ingestor) countItem(app, result string) {
	if in.Items != nil && app != "" {
		in.Items.With(app, result).Inc()
	}
}

func (in *Ingestor) countRejected(app, code string) {
	if in.Rejected != nil && app != "" {
		in.Rejected.With(app, code).Inc()
	}
}

// PruneIngestIDs deletes the push ids older than retention (spec 6.2.4) and returns how many.
func PruneIngestIDs(ctx context.Context, pool *pgxpool.Pool, retention time.Duration, now time.Time) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM push_ingest_ids WHERE created_at < $1`, now.Add(-retention))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
