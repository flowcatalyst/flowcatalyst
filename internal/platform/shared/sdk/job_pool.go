package sdk

import (
	"context"
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// jobPools refuses a job that names an ARCHIVED dispatch pool (owner ruling
// 2026-11-09: an archived pool takes no new jobs). A SUSPENDED pool is
// accepted — its jobs wait until it is resumed — and an id that names no pool
// is passed through as before. One lookup per distinct pool id per request.
type jobPools struct {
	repo  *dispatchpool.Repository
	cache map[string]*dispatchpool.DispatchPool
}

func (s *DispatchJobsBatchState) newJobPools() *jobPools {
	return &jobPools{repo: s.Pools, cache: map[string]*dispatchpool.DispatchPool{}}
}

// check returns 409 POOL_ARCHIVED when j names an archived pool. A nil
// repository checks nothing.
func (p *jobPools) check(ctx context.Context, j *dispatchjob.DispatchJob) error {
	if p.repo == nil || j.DispatchPoolID == nil || *j.DispatchPoolID == "" {
		return nil
	}
	id := *j.DispatchPoolID
	pool, seen := p.cache[id]
	if !seen {
		var err error
		if pool, err = p.repo.FindByID(ctx, id); err != nil {
			return usecase.Internal("REPO", "dispatch pool lookup failed", err)
		}
		p.cache[id] = pool
	}
	if pool != nil && pool.Status == dispatchpool.StatusArchived {
		return usecase.Conflict("POOL_ARCHIVED",
			fmt.Sprintf("dispatch pool %s (%s) is archived and takes no new jobs", pool.Code, id))
	}
	return nil
}
