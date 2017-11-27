package clock

import "time"

type Clock interface {
	Now() time.Time
}

func NewReal() Clock {
	return realClock{}
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}
