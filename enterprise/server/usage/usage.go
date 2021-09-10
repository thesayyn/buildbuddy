package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/buildbuddy-io/buildbuddy/server/environment"
	"github.com/buildbuddy-io/buildbuddy/server/tables"
	"github.com/buildbuddy-io/buildbuddy/server/util/perms"
	"github.com/buildbuddy-io/buildbuddy/server/util/status"
	"github.com/buildbuddy-io/buildbuddy/server/util/timeutil"
	"github.com/go-redis/redis"
)

const (
	// collectionPeriodDuration determines the granularity at which we flush data
	// to the DB.
	//
	// NOTE: if this value is changed, then the implementation of
	// collectionPeriodStartUsec needs to change as well. Also note that if this
	// value is increased, we may lose some usage data for a short period of time,
	// since keys at smaller granularity may not be properly flushed to the DB.
	collectionPeriodDuration = 1 * time.Minute

	// redisKeyTTL defines how long usage keys have to live before they are
	// deleted automatically by Redis.
	//
	// Keys should live for at least 2 collection periods since collection periods
	// aren't finalized until the period is past, plus some wiggle room for Redis
	// latency. We add a few more collection periods on top of that, in case
	// flushing fails due to transient errors.
	redisKeyTTL = 5 * collectionPeriodDuration

	redisUsageKeyPrefix       = "usage/"
	redisGroupsKeyPrefix      = redisUsageKeyPrefix + "groups/"
	redisCountsKeyPrefix      = redisUsageKeyPrefix + "counts/"
	redisInvocationsKeyPrefix = redisUsageKeyPrefix + "invocations/"
)

type TrackerOpts struct {
	Clock timeutil.Clock
}

type tracker struct {
	env   environment.Env
	rdb   *redis.Client
	clock timeutil.Clock
}

func NewTracker(env environment.Env, opts *TrackerOpts) (*tracker, error) {
	rdb := env.GetCacheRedisClient()
	if rdb == nil {
		return nil, status.UnimplementedError("Missing redis client for usage")
	}
	clock := opts.Clock
	if clock == nil {
		clock = timeutil.NewClock()
	}
	return &tracker{
		env:   env,
		rdb:   rdb,
		clock: clock,
	}, nil
}

func (ut *tracker) Increment(ctx context.Context, counts *tables.UsageCounts) error {
	groupID, err := perms.AuthenticatedGroupID(ctx, ut.env)
	if err != nil {
		return err
	}
	if groupID == "" {
		// Don't track anonymous usage for now.
		return nil
	}

	m, err := countsToMap(counts)
	if err != nil {
		return err
	}
	if len(m) == 0 {
		return nil
	}

	t := ut.clock.Now()

	if err := ut.observeGroup(ctx, groupID, t); err != nil {
		return err
	}

	uk := countsRedisKey(groupID, t)
	pipe := ut.rdb.TxPipeline()
	for k, c := range m {
		pipe.HIncrBy(ctx, uk, k, c)
	}
	pipe.Expire(ctx, uk, redisKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	return nil
}

func (ut *tracker) ObserveInvocation(ctx context.Context, invocationID string) error {
	groupID, err := perms.AuthenticatedGroupID(ctx, ut.env)
	if err != nil {
		return err
	}
	if groupID == "" {
		// Don't track anonymous usage for now.
		return nil
	}

	t := ut.clock.Now()

	if err := ut.observeGroup(ctx, groupID, t); err != nil {
		return err
	}

	ik := invocationsRedisKey(groupID, t)
	if _, err := ut.rdb.SAdd(ctx, ik, invocationID).Result(); err != nil {
		return err
	}

	return nil
}

// observeGroup records that the given group has usage data for the current
// collection period, so we know which groups to look for when flushing
// the data for a given period.
func (ut *tracker) observeGroup(ctx context.Context, groupID string, t time.Time) error {
	gk := groupsRedisKey(t)
	if _, err := ut.rdb.SAdd(ctx, gk, groupID).Result(); err != nil {
		return err
	}
	return nil
}

func collectionPeriodStartUsec(t time.Time) int64 {
	utc := t.UTC()
	// Start of minute
	start := time.Date(
		utc.Year(), utc.Month(), utc.Day(),
		utc.Hour(), utc.Minute(), 0, 0,
		utc.Location())
	return timeutil.ToUsec(start)
}

func groupsRedisKey(t time.Time) string {
	return fmt.Sprintf("%s%d", redisGroupsKeyPrefix, collectionPeriodStartUsec(t))
}

func countsRedisKey(groupID string, t time.Time) string {
	return fmt.Sprintf("%s%s/%d", redisCountsKeyPrefix, groupID, collectionPeriodStartUsec(t))
}

func invocationsRedisKey(groupID string, t time.Time) string {
	return fmt.Sprintf("%s%s/%d", redisInvocationsKeyPrefix, groupID, collectionPeriodStartUsec(t))
}

func countsToMap(tu *tables.UsageCounts) (map[string]int64, error) {
	b, err := json.Marshal(tu)
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	if err := json.Unmarshal(b, &counts); err != nil {
		return nil, err
	}
	for k, v := range counts {
		if v == 0 {
			delete(counts, k)
		}
	}
	return counts, nil
}