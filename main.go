package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

func runLoadTest(name string, total int, allowFn func() (bool, time.Duration, error)) {
	var allowed, denied, errs atomic.Int64
	var sampleRetry atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})

	t0 := time.Now()
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ok, retry, err := allowFn()
			if err != nil {
				errs.Add(1)
				return
			}
			if ok {
				allowed.Add(1)
			} else {
				denied.Add(1)
				if retry > 0 {
					sampleRetry.CompareAndSwap(0, int64(retry))
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	elapsed := time.Since(t0)

	fmt.Printf("[%s] Allowed: %d | Denied: %d | Errors: %d | Time: %v",
		name, allowed.Load(), denied.Load(), errs.Load(), elapsed)
	if r := time.Duration(sampleRetry.Load()); r > 0 {
		fmt.Printf(" | Retry-After: %ds", retryAfterSeconds(r))
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer client.Close()

	key := "192.168.1.5"
	client.Del(ctx, "ratelimit:"+key)

	memLimiter := NewRateLimiter(5, 0.05)
	redisLimiter := NewRedisRateLimiter(client, 5, 0.05)

	runLoadTest("In-Memory", 50, func() (bool, time.Duration, error) {
		return memLimiter.Allow(), 0, nil
	})

	runLoadTest("Redis Lua", 50, func() (bool, time.Duration, error) {
		return redisLimiter.Allow(ctx, key)
	})
	runLoadTest("Redis Lua", 50, func() (bool, time.Duration, error) {
		return redisLimiter.Allow(ctx, key)
	})

	startServer(redisLimiter)
}
