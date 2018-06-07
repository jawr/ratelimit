package ratelimit_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jawr/ratelimit"
	"github.com/jawr/ratelimit/internal/clock"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/atomic"
)

func ExampleRatelimit() {
	rl := ratelimit.New(100) // per second

	prev := time.Now()
	for i := 0; i < 10; i++ {
		now := rl.Take()
		if i > 0 {
			fmt.Println(i, now.Sub(prev))
		}
		prev = now
	}

	// Output:
	// 1 600ms
	// 2 600ms
	// 3 600ms
	// 4 600ms
	// 5 600ms
	// 6 600ms
	// 7 600ms
	// 8 600ms
	// 9 600ms
}

func TestUnlimited(t *testing.T) {
	now := time.Now()
	rl := ratelimit.NewUnlimited()
	for i := 0; i < 1000; i++ {
		rl.Take()
	}
	assert.Condition(t, func() bool { return time.Now().Sub(now) < 1*time.Millisecond }, "no artificial delay")
}

func TestRateLimiter(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()

	clock := clock.NewMock()
	rl := ratelimit.New(100, ratelimit.WithClock(clock), ratelimit.WithoutSlack)

	count := atomic.NewInt32(0)

	// Until we're done...
	done := make(chan struct{})
	defer close(done)

	// Create copious counts concurrently.
	go job(rl, count, done)
	go job(rl, count, done)
	go job(rl, count, done)
	go job(rl, count, done)

	clock.AfterFunc(1*time.Minute, func() {
		assert.InDelta(t, 100, count.Load(), 10, "count within rate limit")
	})

	clock.AfterFunc(2*time.Minute, func() {
		assert.InDelta(t, 200, count.Load(), 10, "count within rate limit")
	})

	clock.AfterFunc(3*time.Minute, func() {
		assert.InDelta(t, 300, count.Load(), 10, "count within rate limit")
		wg.Done()
	})

	clock.Add(4 * time.Minute)

	clock.Add(5 * time.Minute)
}

func TestDelayedRateLimiter(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()

	clock := clock.NewMock()
	slow := ratelimit.New(10, ratelimit.WithClock(clock))
	fast := ratelimit.New(100, ratelimit.WithClock(clock))

	count := atomic.NewInt32(0)

	// Until we're done...
	done := make(chan struct{})
	defer close(done)

	// Run a slow job
	go func() {
		for {
			slow.Take()
			fast.Take()
			count.Inc()
			select {
			case <-done:
				return
			default:
			}
		}
	}()

	// Accumulate slack for 10 seconds,
	clock.AfterFunc(20*time.Minute, func() {
		// Then start working.
		go job(fast, count, done)
		go job(fast, count, done)
		go job(fast, count, done)
		go job(fast, count, done)
	})

	clock.AfterFunc(30*time.Minute, func() {
		assert.InDelta(t, 1200, count.Load(), 10, "count within rate limit")
		wg.Done()
	})

	clock.Add(40 * time.Minute)
}

func job(rl ratelimit.Limiter, count *atomic.Int32, done <-chan struct{}) {
	for {
		rl.Take()
		count.Inc()
		select {
		case <-done:
			return
		default:
		}
	}
}
