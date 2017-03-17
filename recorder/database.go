package recorder

import (
	"context"
	"database/sql"

	"github.com/golang/glog"
	_ "github.com/mattn/go-sqlite3"
)

const dbFilename = "./tesla.db"

type CarDatabase struct {
	conn *sql.DB
}

func OpenDatabase() (*CarDatabase, error) {

	db, err := sql.Open("sqlite3", dbFilename)
	if err != nil {
		return nil, err
	}

	err = createTables(db)
	if err != nil {
		return nil, err
	}

	return &CarDatabase{
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

func (db *CarDatabase) Close() {
	db.conn.Close()
}

func (db *CarDatabase) Insert(ctx context.Context, info *CarInfo) error {
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
