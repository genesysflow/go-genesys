package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfig configures a Redis queue connection.
type RedisConfig struct {
	// Addr is the host:port of the Redis server.
	Addr string
	// Password is the optional AUTH password.
	Password string
	// DB is the Redis database number.
	DB int
	// Prefix namespaces every queue key (default "queues:").
	Prefix string
	// RetryAfter is how long a reserved job may be held by a worker
	// before it is assumed crashed and made available again
	// (default 90s), Laravel's retry_after.
	RetryAfter time.Duration
}

// RedisQueue is a durable queue driver backed by Redis lists, with
// sorted sets holding delayed jobs until they come due and reserved
// jobs while a worker processes them - a worker crash leaves the
// reserved copy behind, and it is reclaimed after RetryAfter.
type RedisQueue struct {
	client     *redis.Client
	prefix     string
	retryAfter time.Duration
	ctx        context.Context
}

// redisJob is the envelope stored on the list.
type redisJob struct {
	ID       int64  `json:"id"`
	Queue    string `json:"queue"`
	Name     string `json:"name"`
	Payload  []byte `json:"payload"`
	Attempts int    `json:"attempts"`
}

// NewRedisQueue creates a Redis queue from config.
func NewRedisQueue(config RedisConfig) *RedisQueue {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})
	q := NewRedisQueueWithClient(client, config.Prefix)
	if config.RetryAfter > 0 {
		q.retryAfter = config.RetryAfter
	}
	return q
}

// NewRedisQueueWithClient wraps an existing Redis client.
func NewRedisQueueWithClient(client *redis.Client, prefix string) *RedisQueue {
	if prefix == "" {
		prefix = "queues:"
	}
	return &RedisQueue{
		client:     client,
		prefix:     prefix,
		retryAfter: 90 * time.Second,
		ctx:        context.Background(),
	}
}

// WithRetryAfter sets how long a reserved job may be held before it is
// assumed crashed and reclaimed.
func (q *RedisQueue) WithRetryAfter(d time.Duration) *RedisQueue {
	q.retryAfter = d
	return q
}

// Client exposes the underlying Redis client.
func (q *RedisQueue) Client() *redis.Client { return q.client }

func normalizeQueue(queue string) string {
	if queue == "" {
		return "default"
	}
	return queue
}

// Queue keys live under a "queue:" segment so user queue names cannot
// collide with the failed list or the id counter.
func (q *RedisQueue) listKey(queue string) string {
	return q.prefix + "queue:" + normalizeQueue(queue)
}

func (q *RedisQueue) delayedKey(queue string) string {
	return q.listKey(queue) + ":delayed"
}

func (q *RedisQueue) reservedKey(queue string) string {
	return q.listKey(queue) + ":reserved"
}

func (q *RedisQueue) failedKey() string { return q.prefix + "failed" }
func (q *RedisQueue) idKey() string     { return q.prefix + "next_id" }

// Push dispatches a job onto the default queue.
func (q *RedisQueue) Push(job Job) error {
	return q.Later(0, job)
}

// Later dispatches a job to become available after the given delay.
func (q *RedisQueue) Later(delay time.Duration, job Job) error {
	return q.PushOn("", delay, job)
}

// PushOn dispatches a job onto a named queue after the given delay.
func (q *RedisQueue) PushOn(queueName string, delay time.Duration, job Job) error {
	name, body, err := marshalJob(job)
	if err != nil {
		return err
	}
	id, err := q.client.Incr(q.ctx, q.idKey()).Result()
	if err != nil {
		return err
	}
	envelope := redisJob{ID: id, Queue: normalizeQueue(queueName), Name: name, Payload: body, Attempts: 0}
	return q.enqueue(envelope, delay)
}

func (q *RedisQueue) enqueue(envelope redisJob, delay time.Duration) error {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if delay > 0 {
		return q.client.ZAdd(q.ctx, q.delayedKey(envelope.Queue), redis.Z{
			Score:  float64(time.Now().Add(delay).UnixMilli()),
			Member: encoded,
		}).Err()
	}
	return q.client.RPush(q.ctx, q.listKey(envelope.Queue), encoded).Err()
}

// migrateScript atomically moves every due member of a sorted set onto
// the ready list, so a crash can never lose a job between the ZREM and
// the RPUSH.
var migrateScript = redis.NewScript(`
local due = redis.call('zrangebyscore', KEYS[1], '-inf', ARGV[1])
for i = 1, #due do
	redis.call('zrem', KEYS[1], due[i])
	redis.call('rpush', KEYS[2], due[i])
end
return #due
`)

// popScript atomically pops the next ready job, increments its attempt
// count, and parks it on the reserved set until the worker settles it -
// Laravel's retrieveNextJob. A worker crash leaves the reserved copy to
// be reclaimed by migrateDue once its score (the reservation expiry)
// passes.
var popScript = redis.NewScript(`
local job = redis.call('lpop', KEYS[1])
if job == false then
	return false
end
local decoded = cjson.decode(job)
decoded.attempts = decoded.attempts + 1
local reserved = cjson.encode(decoded)
redis.call('zadd', KEYS[2], ARGV[1], reserved)
return reserved
`)

// migrateDue moves due delayed jobs and expired reservations (crashed
// workers) onto the ready list.
func (q *RedisQueue) migrateDue(queue string) error {
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	for _, source := range []string{q.delayedKey(queue), q.reservedKey(queue)} {
		keys := []string{source, q.listKey(queue)}
		if err := migrateScript.Run(q.ctx, q.client, keys, now).Err(); err != nil {
			return err
		}
	}
	return nil
}

// Pop reserves the next available job. Returns (nil, nil) when empty.
func (q *RedisQueue) Pop(queue string) (*ReservedJob, error) {
	if err := q.migrateDue(queue); err != nil {
		return nil, err
	}
	expiry := strconv.FormatInt(time.Now().Add(q.retryAfter).UnixMilli(), 10)
	keys := []string{q.listKey(queue), q.reservedKey(queue)}
	raw, err := popScript.Run(q.ctx, q.client, keys, expiry).Text()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var envelope redisJob
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, fmt.Errorf("queue: corrupt redis job: %w", err)
	}
	return &ReservedJob{
		ID:       envelope.ID,
		Queue:    normalizeQueue(envelope.Queue),
		Name:     envelope.Name,
		Payload:  envelope.Payload,
		Attempts: envelope.Attempts, // popScript already counted this attempt
		reserved: raw,
	}, nil
}

// Delete removes a completed job's reserved copy.
func (q *RedisQueue) Delete(job *ReservedJob) error {
	if job.reserved == "" {
		return nil
	}
	return q.client.ZRem(q.ctx, q.reservedKey(job.Queue), job.reserved).Err()
}

// Release returns a failed job to the queue after a delay, keeping its
// attempt count. The job is re-queued before the reserved copy is
// dropped, so a crash between the steps duplicates rather than loses it.
func (q *RedisQueue) Release(job *ReservedJob, delay time.Duration) error {
	if err := q.enqueue(redisJob{
		ID:       job.ID,
		Queue:    normalizeQueue(job.Queue),
		Name:     job.Name,
		Payload:  job.Payload,
		Attempts: job.Attempts,
	}, delay); err != nil {
		return err
	}
	return q.Delete(job)
}

// Fail records a permanently failed job on the failed list and drops
// its reserved copy.
func (q *RedisQueue) Fail(job *ReservedJob, jobErr error) error {
	failed := FailedJob{
		ID:        job.ID,
		Queue:     normalizeQueue(job.Queue),
		Name:      job.Name,
		Payload:   job.Payload,
		Exception: jobErr.Error(),
		FailedAt:  time.Now().UTC(),
	}
	encoded, err := json.Marshal(failed)
	if err != nil {
		return err
	}
	if err := q.client.RPush(q.ctx, q.failedKey(), encoded).Err(); err != nil {
		return err
	}
	return q.Delete(job)
}

// ListFailed returns all failed jobs.
func (q *RedisQueue) ListFailed() ([]FailedJob, error) {
	raws, err := q.client.LRange(q.ctx, q.failedKey(), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	failed := make([]FailedJob, 0, len(raws))
	for _, raw := range raws {
		var job FailedJob
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			return nil, fmt.Errorf("queue: corrupt failed job: %w", err)
		}
		failed = append(failed, job)
	}
	return failed, nil
}

// RetryFailed re-dispatches a failed job and removes it from the list.
func (q *RedisQueue) RetryFailed(id int64) error {
	raw, failed, err := q.findFailed(id)
	if err != nil {
		return err
	}
	if err := q.enqueue(redisJob{
		ID:      failed.ID,
		Queue:   failed.Queue,
		Name:    failed.Name,
		Payload: failed.Payload,
	}, 0); err != nil {
		return err
	}
	return q.client.LRem(q.ctx, q.failedKey(), 1, raw).Err()
}

// ForgetFailed removes a failed job without retrying it.
func (q *RedisQueue) ForgetFailed(id int64) error {
	raw, _, err := q.findFailed(id)
	if err != nil {
		return err
	}
	return q.client.LRem(q.ctx, q.failedKey(), 1, raw).Err()
}

func (q *RedisQueue) findFailed(id int64) (string, *FailedJob, error) {
	raws, err := q.client.LRange(q.ctx, q.failedKey(), 0, -1).Result()
	if err != nil {
		return "", nil, err
	}
	for _, raw := range raws {
		var job FailedJob
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			continue
		}
		if job.ID == id {
			return raw, &job, nil
		}
	}
	return "", nil, fmt.Errorf("queue: failed job %d not found", id)
}

// Size returns the number of ready, delayed, and reserved jobs on a
// queue.
func (q *RedisQueue) Size(queue string) int {
	ready, _ := q.client.LLen(q.ctx, q.listKey(queue)).Result()
	delayed, _ := q.client.ZCard(q.ctx, q.delayedKey(queue)).Result()
	reserved, _ := q.client.ZCard(q.ctx, q.reservedKey(queue)).Result()
	return int(ready + delayed + reserved)
}

// Close closes the underlying client.
func (q *RedisQueue) Close() error { return q.client.Close() }
