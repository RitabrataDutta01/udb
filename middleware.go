package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
)

func RateLimitMiddleware(limiter *RedisRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		allowed, retryAfter, err := limiter.Allow(r.Context(), ip)
		if err != nil {
			log.Printf("rate limiter error (failing open): %v", err)
			next.ServeHTTP(w, r)
			return
		}

		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter)))
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func startServer(limiter *RedisRateLimiter) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})

	log.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", RateLimitMiddleware(limiter, mux)))
}
