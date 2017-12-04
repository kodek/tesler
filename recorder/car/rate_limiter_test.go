package car

import (
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (fc *fakeClock) Now() time.Time {
	return fc.now
}

func TestDueToDriving(t *testing.T) {
	dc := newDurationCalculator()
	dc.clock = &fakeClock{
		now: time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	actual := dc.calculate(Snapshot{DrivingState: "Not Empty"})
	expected := drivingRefreshDuration
	if actual != expected {
		t.Errorf("Duration calculator while driving got %s, expected %s.", actual, expected)
	}

	actual = dc.calculate(Snapshot{DrivingState: ""})
	notExpected := drivingRefreshDuration
	if actual == notExpected {
		t.Errorf("Duration calculator while driving got %s, did not expect %s.", actual, notExpected)
	}
}

// func TestDueToCharging(t *testing.T) {
// 	dc := newDurationCalculator()
// 	dc.clock = &fakeClock{
// 		now: time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC),
// 	}
//
// 	d := dc.calculate(Snapshot{})
// 	if d != i.expectedDuration {
// 		t.Errorf("Duration calculator at hour %d got %s, expected %s.", i.hour, d, i.expectedDuration)
// 	}
// }

func TestDefaultRates(t *testing.T) {
	dc := newDurationCalculator()
	tests := []struct {
		hour             int
		expectedDuration time.Duration
	}{
		{0, sleepingRefreshDuration},
		{1, sleepingRefreshDuration},
		{7, sleepingRefreshDuration},
		{8, sleepingRefreshDuration},
		{9, normalRefreshDuration},
		{15, normalRefreshDuration},
		{23, normalRefreshDuration},
	}
	for _, i := range tests {
		dc.clock = &fakeClock{
			now: time.Date(2017, 1, 1, i.hour, 0, 0, 0, time.UTC),
		}
		d := dc.calculate(Snapshot{})
		if d != i.expectedDuration {
			t.Errorf("Duration calculator at hour %d got %s, expected %s.", i.hour, d, i.expectedDuration)
		}
	}
}