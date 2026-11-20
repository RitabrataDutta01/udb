package main

import (
	"context"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local max_tokens = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])

-- 1. Use Redis server time (seconds, microseconds) -> milliseconds
local time_res = redis.call("TIME")
local now = tonumber(time_res[1]) * 1000 + math.floor(tonumber(time_res[2]) / 1000)

local data = redis.call("HMGET", key, "tokens", "last_refill")
local tokens = tonumber(data[1])
local last_refill = tonumber(data[2])

if not tokens or not last_refill then
	tokens = max_tokens
	last_refill = now
else
	local elapsed_seconds = (now - last_refill) / 1000.0
	if elapsed_seconds < 0 then
		elapsed_seconds = 0
	end
	tokens = math.min(max_tokens, tokens + (elapsed_seconds * refill_rate))
	last_refill = now
end

-- 2. TTL = time to refill a completely empty bucket (in ms)
local ttl_ms = math.ceil((max_tokens / refill_rate) * 1000)

if tokens >= 1 then
	tokens = tokens - 1
	redis.call("HSET", key, "tokens", tokens, "last_refill", last_refill)
	redis.call("PEXPIRE", key, ttl_ms)
	return {1, 0}
end

-- 3. Denied: calculate how many ms until 1 full token is available
local retry_after_ms = math.ceil(((1 - tokens) / refill_rate) * 1000)
redis.call("HSET", key, "tokens", tokens, "last_refill", last_refill)
redis.call("PEXPIRE", key, ttl_ms)
return {0, retry_after_ms}
`)

type RedisRateLimiter struct {
	client     *redis.Client
	maxTokens  float64
	refillRate float64
}

func NewRedisRateLimiter(client *redis.Client, maxTokens, refillRate float64) *RedisRateLimiter {
	return &RedisRateLimiter{
		client:     client,
		maxTokens:  maxTokens,
		refillRate: refillRate,
	}
}

func (r *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	redisKey := "ratelimit:" + key

	res, err := tokenBucketScript.Run(
		ctx,
		r.client,
		[]string{redisKey},
		r.maxTokens,
		r.refillRate,
	).Int64Slice()
	if err != nil {
		return false, 0, err
	}

	allowed := res[0] == 1
	retryAfter := time.Duration(res[1]) * time.Millisecond
	return allowed, retryAfter, nil
}

func retryAfterSeconds(d time.Duration) int {
	return int(math.Ceil(d.Seconds()))
}
