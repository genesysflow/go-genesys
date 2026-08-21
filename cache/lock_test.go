package cache_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockMutualExclusion(t *testing.T) {
	store := cache.NewMemoryStore()

	first := cache.NewLock(store, "resource", time.Minute)
	acquired, err := first.Get()
	require.NoError(t, err)
	require.True(t, acquired)

	second := cache.NewLock(store, "resource", time.Minute)
	acquired, err = second.Get()
	require.NoError(t, err)
	assert.False(t, acquired, "held locks cannot be re-acquired")

	// Releasing someone else's lock is a no-op.
	released, err := second.Release()
	require.NoError(t, err)
	assert.False(t, released)

	released, err = first.Release()
	require.NoError(t, err)
	assert.True(t, released)

	acquired, err = second.Get()
	require.NoError(t, err)
	assert.True(t, acquired, "freed locks can be taken")
}

func TestLockExpiresWithTTL(t *testing.T) {
	store := cache.NewMemoryStore()

	crashed := cache.NewLock(store, "job", 30*time.Millisecond)
	acquired, err := crashed.Get()
	require.NoError(t, err)
	require.True(t, acquired)

	time.Sleep(60 * time.Millisecond)

	next := cache.NewLock(store, "job", time.Minute)
	acquired, err = next.Get()
	require.NoError(t, err)
	assert.True(t, acquired, "expired locks are reclaimable")

	// The crashed holder must not release the new owner's lock.
	released, err := crashed.Release()
	require.NoError(t, err)
	assert.False(t, released)
}

func TestLockBlock(t *testing.T) {
	store := cache.NewMemoryStore()

	holder := cache.NewLock(store, "slow", time.Minute)
	_, err := holder.Get()
	require.NoError(t, err)

	// Release shortly after; Block should pick it up.
	go func() {
		time.Sleep(50 * time.Millisecond)
		holder.Release()
	}()

	waiter := cache.NewLock(store, "slow", time.Minute)
	acquired, err := waiter.Block(time.Second)
	require.NoError(t, err)
	assert.True(t, acquired)

	// A held lock times out.
	late := cache.NewLock(store, "slow", time.Minute)
	acquired, err = late.Block(50 * time.Millisecond)
	require.NoError(t, err)
	assert.False(t, acquired)
}

func TestWithLockSerialisesWorkers(t *testing.T) {
	store := cache.NewMemoryStore()

	var concurrent, maxConcurrent int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ran, err := cache.WithLock(store, "critical", time.Minute, time.Second, func() error {
				now := atomic.AddInt64(&concurrent, 1)
				for {
					max := atomic.LoadInt64(&maxConcurrent)
					if now <= max || atomic.CompareAndSwapInt64(&maxConcurrent, max, now) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt64(&concurrent, -1)
				return nil
			})
			assert.NoError(t, err)
			assert.True(t, ran)
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, atomic.LoadInt64(&maxConcurrent), "critical section never overlapped")
}

func TestLocksOnRedis(t *testing.T) {
	store, _ := newRedisStore(t)

	lock := cache.NewLock(store, "shared", time.Minute)
	acquired, err := lock.Get()
	require.NoError(t, err)
	require.True(t, acquired)

	rival := cache.NewLock(store, "shared", time.Minute)
	acquired, err = rival.Get()
	require.NoError(t, err)
	assert.False(t, acquired)

	released, err := lock.Release()
	require.NoError(t, err)
	assert.True(t, released)
}
