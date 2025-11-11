package backoff

import (
	"math"
	"math/rand"
	"time"
)

// CalculateDelay calculates exponential backoff delay
// Formula: delay = base^attempts
// Example with base=2:
//   Attempt 1: 2^1 = 2 seconds
//   Attempt 2: 2^2 = 4 seconds
//   Attempt 3: 2^3 = 8 seconds
func CalculateDelay(attempt int, base int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	if base <= 1 {
		base = 2
	}

	delay := math.Pow(float64(base), float64(attempt))
	return time.Duration(delay) * time.Second
}

// CalculateDelayWithJitter adds randomness to prevent thundering herd
// Adds jitter: ±10% of the base delay
func CalculateDelayWithJitter(attempt int, base int) time.Duration {
	delay := CalculateDelay(attempt, base)

	// Add jitter: random value between -10% and +10% of delay
	jitterPercent := int64(float64(delay) * 0.1)
	jitter := time.Duration(rand.Int63n(jitterPercent*2) - jitterPercent)

	result := delay + jitter
	if result < 0 {
		return delay // Don't return negative
	}
	return result
}

// CalculateDelayWithMaximum caps the maximum delay
func CalculateDelayWithMaximum(attempt int, base int, maxDelay time.Duration) time.Duration {
	delay := CalculateDelay(attempt, base)
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}