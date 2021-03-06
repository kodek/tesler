package databases

import (
	"context"

	"github.com/golang/glog"
	influxdb "github.com/influxdata/influxdb1-client/v2"
	"github.com/kodek/tesler/recorder/car"
)

type influxDbDatabase struct {
	conn     influxdb.Client
	database string
}

func (this *influxDbDatabase) GetLatest(ctx context.Context) (*car.Snapshot, error) {
	panic("implement me")
}

func (this *influxDbDatabase) Insert(ctx context.Context, snapshot car.Snapshot) error {
	glog.Infof("Recording measurement to influxdb")

	bp, err := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database:  this.database,
		Precision: "s",
	})
	if err != nil {
		return err
	}

	// Indexed tags
	tags := map[string]string{
		"car_name": snapshot.Name,
		"vin":      snapshot.Vin,
	}

	// Charging
	chargeFields := map[string]interface{}{
		"state":            snapshot.ChargingState,
		"batt_level":       snapshot.BatteryLevel,
		"range_left":       snapshot.RangeLeft,
		"charge_limit_soc": snapshot.ChargeLimitSoc,
	}
	if snapshot.ChargeSession != nil {
		ci := snapshot.ChargeSession
		chargeFields["voltage"] = ci.Voltage
		chargeFields["actual_current"] = ci.ActualCurrent
		chargeFields["pilot_current"] = ci.PilotCurrent
		chargeFields["charge_miles_added"] = ci.ChargeMilesAdded
		chargeFields["charge_rate"] = ci.ChargeRate
		// NOTE: "time_to_full_charge" accidentally stored pointers. We're writing to a new field
		// until we reset the database.
		chargeFields["time_to_full_charge_hrs"] = ci.TimeToFullCharge
	}
	charge, err := influxdb.NewPoint("charge", tags, chargeFields, snapshot.Timestamp)
	if err != nil {
		return err
	}
	bp.AddPoint(charge)

	// Position
	posMap := map[string]interface{}{
		"latitude":      snapshot.Bearings.Latitude,
		"longitude":     snapshot.Bearings.Longitude,
		"power":         snapshot.Power,
		"odometer":      snapshot.Odometer,
		"speed":         snapshot.Bearings.Speed,
		"driving_state": snapshot.DrivingState,
	}
	pos, err := influxdb.NewPoint(
		"position",
		tags,
		posMap,
		snapshot.Timestamp)
	if err != nil {
		return err
	}
	bp.AddPoint(pos)

	// Misc
	misc, err := influxdb.NewPoint(
		"misc",
		tags,
		map[string]interface{}{
			"wake_state":         snapshot.WakeState,
			"active_description": snapshot.ActiveDescription,
		}, snapshot.Timestamp)
	if err != nil {
		return err
	}
	bp.AddPoint(misc)

	err = this.conn.Write(bp)
	if err != nil {
		return err
	}

	glog.Info("Writing to InfluxDB successful")
	return nil
}

func (this *influxDbDatabase) Close() error {
	return this.conn.Close()
}

func OpenInfluxDbDatabase(address string, username string, password string, database string) (Database, error) {
	// Create a new HTTPClient
	c, err := influxdb.NewHTTPClient(influxdb.HTTPConfig{
		Addr:     address,
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, err
	}

	return &influxDbDatabase{
		conn:     c,
		database: database,
	}, nil
}
