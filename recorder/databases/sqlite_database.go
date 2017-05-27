package databases

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"bitbucket.org/kodek64/tesler/recorder"
	"github.com/golang/glog"
	_ "github.com/mattn/go-sqlite3"
)

type SqliteDatabase struct {
	conn *sql.DB
}

func OpenSqliteDatabase(path string) (Database, error) {

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	err = createTables(db)
	if err != nil {
		return nil, err
	}

	return &SqliteDatabase{
		conn: db,
	}, nil
}

// TODO: Support the full CarInfo in a normalized fashion.
func createTables(conn *sql.DB) error {
	sqlStmt := `
	create table if not exists CARINFO (
	  timestamp integer not null primary key,
	  driving_state text,
	  latitude real,
	  longitude real,
	  speed integer,
	  charging_state text,
	  battery_level integer);
	`
	_, err := conn.Exec(sqlStmt)
	return err
}

func (db *SqliteDatabase) Close() error {
	return db.conn.Close()
}

func (db *SqliteDatabase) GetLatest(ctx context.Context) (*recorder.CarInfo, error) {
	glog.Infof("Querying database for latest record.")
	q := "select timestamp, driving_state, latitude, longitude, speed, charging_state, battery_level from CARINFO order by timestamp desc limit 1"
	rows, err := db.conn.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var carInfo *recorder.CarInfo
	for rows.Next() {
		if carInfo != nil {
			// TODO: This is an assertion, but we should handle it more gracefully.
			panic("Got more than one record for a 'limit 1' query: " + q)
		}
		var timestamp int64
		var drivingState string
		var lat float64
		var lon float64
		var speed int
		var chargeState string
		var battLevel int
		err = rows.Scan(&timestamp, &drivingState, &lat, &lon, &speed, &chargeState, &battLevel)
		if err != nil {
			return nil, err
		}
		carInfo = &recorder.CarInfo{
			Timestamp:    time.Unix(timestamp, 0),
			Name:         "Eve", // TODO: Add to different table.
			DrivingState: drivingState,
			Position: recorder.CarPosition{
				Latitude:  lat,
				Longitude: lon,
				Speed:     speed,
			},
			ChargingState: chargeState,
			BatteryLevel:  battLevel,
			Charge: &recorder.ChargeInfo{
				Voltage:          0,
				ActualCurrent:    0,
				PilotCurrent:     0,
				TimeToFullCharge: new(float64),
			},
		}
	}
	if carInfo == nil {
		return nil, errors.New("Cannot find any records! Is the database empty?")
	}
	return carInfo, nil
}

func (db *SqliteDatabase) Insert(ctx context.Context, info *recorder.CarInfo) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("insert into CARINFO(timestamp, driving_state, latitude, longitude, speed, charging_state, battery_level) values(?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(
		info.Timestamp.Unix(),
		info.DrivingState,
		info.Position.Latitude,
		info.Position.Longitude,
		info.Position.Speed,
		info.ChargingState,
		info.BatteryLevel)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	glog.Infof("Saved record with timestamp %d into database.", info.Timestamp.Unix())
	return nil
}