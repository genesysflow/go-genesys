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
}

// RedisQueue is a durable queue driver backed by Redis lists, with a
// sorted set holding delayed and released jobs until they come due.
type RedisQueue struct {
	client *redis.Client
	prefix string
	ctx    context.Context
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
	return NewRedisQueueWithClient(client, config.Prefix)
}

// NewRedisQueueWithClient wraps an existing Redis client.
func NewRedisQueueWithClient(client *redis.Client, prefix string) *RedisQueue {
	if prefix == "" {
		prefix = "queues:"
	}
	return &RedisQueue{client: client, prefix: prefix, ctx: context.Background()}
}

// Client exposes the underlying Redis client.
func (q *RedisQueue) Client() *redis.Client { return q.client }

func normalizeQueue(queue string) string {
	if queue == "" {
		return "default"
	}
	return queue
}

func (q *RedisQueue) listKey(queue string) string {
	return q.prefix + normalizeQueue(queue)
}

func (q *RedisQueue) delayedKey(queue string) string {
	return q.prefix + normalizeQueue(queue) + ":delayed"
}

func (q *RedisQueue) failedKey() string { return q.prefix + "failed" }
func (q *RedisQueue) idKey() string     { return q.prefix + "next_id" }

// Push dispatches a job onto the default queue.
func (q *RedisQueue) Push(job Job) error {
	return q.Later(0, job)
}

// Later dispatches a job to become available after the given delay.
func (q *RedisQueue) Later(delay time.Duration, job Job) error {
	name, body, err := marshalJob(job)
	if err != nil {
		return err
	}
	id, err := q.client.Incr(q.ctx, q.idKey()).Result()
	if err != nil {
		return err
	}
	envelope := redisJob{ID: id, Queue: "default", Name: name, Payload: body, Attempts: 0}
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

// migrateDue moves delayed jobs that have come due onto the ready list.
func (q *RedisQueue) migrateDue(queue string) error {
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	due, err := q.client.ZRangeByScore(q.ctx, q.delayedKey(queue), &redis.ZRangeBy{
		Min: "-inf", Max: now,
	}).Result()
	if err != nil {
		return err
	}
	for _, member := range due {
		removed, err := q.client.ZRem(q.ctx, q.delayedKey(queue), member).Result()
		if err != nil {
			return err
		}
		if removed > 0 { // we won the race for this job
			if err := q.client.RPush(q.ctx, q.listKey(queue), member).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

// Pop reserves the next available job. Returns (nil, nil) when empty.
func (q *RedisQueue) Pop(queue string) (*ReservedJob, error) {
	if err := q.migrateDue(queue); err != nil {
		return nil, err
	}
	raw, err := q.client.LPop(q.ctx, q.listKey(queue)).Result()
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
		Attempts: envelope.Attempts + 1,
	}, nil
}

// Delete removes a completed job. Popping already removed it from the
// list, so nothing remains to clean up.
func (q *RedisQueue) Delete(_ *ReservedJob) error { return nil }

// Release returns a failed job to the queue after a delay, keeping its
// attempt count.
func (q *RedisQueue) Release(job *ReservedJob, delay time.Duration) error {
	return q.enqueue(redisJob{
		ID:       job.ID,
		Queue:    job.Queue,
		Name:     job.Name,
		Payload:  job.Payload,
		Attempts: job.Attempts,
	}, delay)
}

// Fail records a permanently failed job on the failed list.
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
	return q.client.RPush(q.ctx, q.failedKey(), encoded).Err()
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

// Size returns the number of ready plus delayed jobs on a queue.
func (q *RedisQueue) Size(queue string) int {
	ready, _ := q.client.LLen(q.ctx, q.listKey(queue)).Result()
	delayed, _ := q.client.ZCard(q.ctx, q.delayedKey(queue)).Result()
	return int(ready + delayed)
}

// Close closes the underlying client.
func (q *RedisQueue) Close() error { return q.client.Close() }
