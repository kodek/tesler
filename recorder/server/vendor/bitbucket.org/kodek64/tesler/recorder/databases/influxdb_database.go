package databases

import (
	"context"

	"bitbucket.org/kodek64/tesler/recorder/car"
	"github.com/golang/glog"
	influxdb "github.com/influxdata/influxdb/client/v2"
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
	pos, err := influxdb.NewPoint(
		"position",
		tags,
		map[string]interface{}{
			"latitude":      snapshot.Bearings.Latitude,
			"longitude":     snapshot.Bearings.Longitude,
			"speed":         snapshot.Bearings.Speed,
			"odometer":      snapshot.Odometer,
			"driving_state": snapshot.DrivingState,
		}, snapshot.Timestamp)
	if err != nil {
		return err
	}
	bp.AddPoint(pos)

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
