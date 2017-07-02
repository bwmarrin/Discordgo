package discordgo

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// customRateLimit holds information for defining a custom rate limit
type customRateLimit struct {
	suffix   string
	requests int
	reset    time.Duration
}

// RateLimiter holds all ratelimit buckets
type RateLimiter struct {
	sync.Mutex
	global           *int64
	buckets          map[string]*Bucket
	globalRateLimit  time.Duration
	customRateLimits []*customRateLimit
}

// NewRatelimiter returns a new RateLimiter
func NewRatelimiter() *RateLimiter {

	return &RateLimiter{
		buckets:          make(map[string]*Bucket),
		global:           new(int64),
		customRateLimits: []*customRateLimit{},
	}
}

// SetCustomRateLimit allows you to define a custom rate limit.
//    suffix  :   Suffix of the bucket key. (ex) https://discordapp.com/api/channels/279809045819555840/messages//reactions//
//    requests:   How many requests per reset
//    reset   :   How long the reset timer is.
func (r *RateLimiter) SetCustomRateLimit(suffix string, requests int, reset time.Duration) {
	r.Lock()
	defer r.Unlock()

	// if the ratelimit already exists, update the settings.
	var rl *customRateLimit
	for _, v := range r.customRateLimits {
		if v.suffix == suffix {
			v.requests = requests
			v.reset = reset
			rl = v
			break
		}
	}

	// Create a new ratelimit if it does not exist
	if rl == nil {
		rl = &customRateLimit{
			suffix:   suffix,
			requests: requests,
			reset:    reset,
		}
		r.customRateLimits = append(r.customRateLimits, rl)
	}

	// Apply the custom rate limit to all active buckets matching suffix.
	for _, v := range r.buckets {
		if strings.HasSuffix(v.Key, rl.suffix) {
			v.crlmut.Lock()
			v.customRateLimit = rl
			v.crlmut.Unlock()
		}
	}

}

// RemoveCustomRateLimit removes a custom ratelimit from the ratelimiter and all its buckets
//    suffix: The suffix of the custom ratelimiter to remove
func (r *RateLimiter) RemoveCustomRateLimit(suffix string) error {
	r.Lock()
	defer r.Unlock()

	found := false
	for i, v := range r.customRateLimits {
		if v.suffix == suffix {
			r.customRateLimits = append(r.customRateLimits[:i], r.customRateLimits[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return errors.New("err: custom rate limit not found")
	}

	// remove the ratelimit from all active buckets
	for _, b := range r.buckets {
		b.crlmut.Lock()
		if b.customRateLimit != nil && b.customRateLimit.suffix == suffix {
			b.customRateLimit = nil
		}
		b.crlmut.Unlock()
	}

	return nil
}

// getBucket retrieves or creates a bucket
func (r *RateLimiter) getBucket(key string) *Bucket {
	r.Lock()
	defer r.Unlock()

	if bucket, ok := r.buckets[key]; ok {
		return bucket
	}

	b := &Bucket{
		remaining: 1,
		Key:       key,
		global:    r.global,
	}

	// Check if there is a custom ratelimit set for this bucket ID.
	for _, rl := range r.customRateLimits {
		if strings.HasSuffix(b.Key, rl.suffix) {
			b.customRateLimit = rl
			break
		}
	}

	r.buckets[key] = b
	return b
}

// LockBucket Locks until a request can be made
func (r *RateLimiter) LockBucket(bucketID string) *Bucket {

	b := r.getBucket(bucketID)

	b.Lock()

	// If we ran out of calls and the reset time is still ahead of us
	// then we need to take it easy and relax a little
	if b.remaining < 1 && b.reset.After(time.Now()) {
		time.Sleep(b.reset.Sub(time.Now()))

	}

	// Check for global ratelimits
	sleepTo := time.Unix(0, atomic.LoadInt64(r.global))
	if now := time.Now(); now.Before(sleepTo) {
		time.Sleep(sleepTo.Sub(now))
	}

	b.remaining--
	return b
}

// Bucket represents a ratelimit bucket, each bucket gets ratelimited individually (-global ratelimits)
type Bucket struct {
	sync.Mutex
	Key       string
	remaining int
	limit     int
	reset     time.Time
	global    *int64

	// Custom Ratelimits
	crlmut          sync.Mutex
	lastReset       time.Time
	customRateLimit *customRateLimit
}

// Release unlocks the bucket and reads the headers to update the buckets ratelimit info
// and locks up the whole thing in case if there's a global ratelimit.
func (b *Bucket) Release(headers http.Header) error {
	defer b.Unlock()

	// Check if the bucket uses a custom ratelimiter
	b.crlmut.Lock()
	if rl := b.customRateLimit; rl != nil {
		b.crlmut.Unlock()

		if time.Now().Sub(b.lastReset) >= rl.reset {
			b.remaining = rl.requests - 1
			b.lastReset = time.Now()
		}
		if b.remaining < 1 {
			b.reset = time.Now().Add(rl.reset)
		}
		return nil
	}
	b.crlmut.Unlock()

	if headers == nil {
		return nil
	}

	remaining := headers.Get("X-RateLimit-Remaining")
	reset := headers.Get("X-RateLimit-Reset")
	global := headers.Get("X-RateLimit-Global")
	retryAfter := headers.Get("Retry-After")

	// Update global and per bucket reset time if the proper headers are available
	// If global is set, then it will block all buckets until after Retry-After
	// If Retry-After without global is provided it will use that for the new reset
	// time since it's more accurate than X-RateLimit-Reset.
	// If Retry-After after is not proided, it will update the reset time from X-RateLimit-Reset
	if retryAfter != "" {
		parsedAfter, err := strconv.ParseInt(retryAfter, 10, 64)
		if err != nil {
			return err
		}

		resetAt := time.Now().Add(time.Duration(parsedAfter) * time.Millisecond)

		// Lock either this single bucket or all buckets
		if global != "" {
			atomic.StoreInt64(b.global, resetAt.UnixNano())
		} else {
			b.reset = resetAt
		}
	} else if reset != "" {
		// Calculate the reset time by using the date header returned from discord
		discordTime, err := http.ParseTime(headers.Get("Date"))
		if err != nil {
			return err
		}

		unix, err := strconv.ParseInt(reset, 10, 64)
		if err != nil {
			return err
		}

		// Calculate the time until reset and add it to the current local time
		// some extra time is added because without it i still encountered 429's.
		// The added amount is the lowest amount that gave no 429's
		// in 1k requests
		delta := time.Unix(unix, 0).Sub(discordTime) + time.Millisecond*250
		b.reset = time.Now().Add(delta)
	}

	// Udpate remaining if header is present
	if remaining != "" {
		parsedRemaining, err := strconv.ParseInt(remaining, 10, 32)
		if err != nil {
			return err
		}
		b.remaining = int(parsedRemaining)
	}

	return nil
}
