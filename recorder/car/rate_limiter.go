package car

import (
	"time"

	"bitbucket.org/kodek64/tesler/recorder/clock"
	"github.com/golang/glog"
)

// TODO: Should be flags
const drivingRefreshDuration = 15 * time.Second
const normalRefreshDuration = 30 * time.Minute
const chargingRefreshDuration = 1 * time.Minute
const sleepingRefreshDuration = 1 * time.Hour

type RateLimiter struct {
	ticker       *time.Ticker
	nextDuration *durationCalculator
}

func NewRateLimiter() RateLimiter {
	firstDuration := 15 * time.Second // Chosen somewhat arbitrarily.
	dc := newDurationCalculator()
	return RateLimiter{
		// Use a fast rate for the first tick.
		ticker:       time.NewTicker(firstDuration),
		nextDuration: &dc,
	}
}

func (rl *RateLimiter) RateLimit(latestSnapshot Snapshot) {
	// Rate limit
	if rl.ticker != nil {
		<-rl.ticker.C
		rl.ticker.Stop()
	}

	// Select a new ticker based on current state
	duration := rl.nextDuration.calculate(latestSnapshot)
	rl.ticker = time.NewTicker(duration)
}

func newDurationCalculator() durationCalculator {
	return durationCalculator{
		clock: clock.NewReal(),
	}
}

// durationCalculator calculates how long to sleep for the next rate-limiting cycle.
type durationCalculator struct {
	clock clock.Clock
	// Add previous snapshot or whatever
}

func (dc *durationCalculator) calculate(latestSnapshot Snapshot) time.Duration {
	if latestSnapshot.DrivingState != "" {
		glog.Infof("Fast refreshing due to use: %s", drivingRefreshDuration)
		return drivingRefreshDuration
	}
	// Normal ticking: car is charging
	if latestSnapshot.ChargeSession != nil {
		if latestSnapshot.ChargingState == "Complete" {
			glog.Infof("Plugged in, but fully charged. Not using charging refresh rate.")
		} else {
			glog.Infof("Refreshing due to charging (not fully charged): %s", chargingRefreshDuration)
			return chargingRefreshDuration
		}
	}

	// It's between midnight and 8 am and car isn't charging or being used.
	// TODO: Change to car is not charging and it hasn't been used in 2 hours.
	now := dc.clock.Now()
	if now.Hour() >= 0 && now.Hour() <= 8 {
		glog.Infof("Slow refreshing due to hour of day %s being between 12 to 8 am: %s", now.Hour(), sleepingRefreshDuration)
		return sleepingRefreshDuration

	}
	glog.Infof("Normal refreshing, car likely parked/stopped: %s", normalRefreshDuration)
	return normalRefreshDuration
}
