package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const IcingaPrefix = "icinga_"

type Table struct {
	Name       string
	TimeColumn string
}

// Known tables from the IDO with their time column
// Compare with DbConnection::CleanUpHandler()
// https://github.com/Icinga/icinga2/blob/master/lib/db_ido/dbconnection.cpp
var knownTables = []Table{
	{"acknowledgements", "entry_time"},
	{"commenthistory", "entry_time"},
	{"contactnotifications", "start_time"},
	{"contactnotificationmethods", "start_time"},
	{"downtimehistory", "entry_time"},
	{"eventhandlers", "start_time"},
	{"externalcommands", "entry_time"},
	{"flappinghistory", "event_time"},
	{"hostchecks", "start_time"},
	{"logentries", "logentry_time"},
	{"notifications", "start_time"},
	{"processevents", "event_time"},
	{"statehistory", "state_time"},
	{"servicechecks", "start_time"},
	{"systemcommands", "start_time"},
}

// OldestTime retrieves the timestamp of the oldest row in the table.
func (t Table) OldestTime(db *sql.DB, instanceID int) (ts time.Time, err error) {
	query := fmt.Sprintf("SELECT %s FROM %s%s WHERE instance_id = ? ORDER BY %s ASC LIMIT 1", //nolint:gosec
		t.TimeColumn, IcingaPrefix, t.Name, t.TimeColumn)

	// log.Debugf("running query: %s - [%d]", query, instanceID)

	row := db.QueryRow(query, instanceID)

	var tsString string

	err = row.Scan(&tsString)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
			return
		}

		err = fmt.Errorf("could not get oldest time for %s: %w", t.Name, err)

		return
	}

	ts, err = time.Parse("2006-01-02 15:04:05", tsString)
	if err != nil {
		err = fmt.Errorf("could not parse date: %s - %w", tsString, err)
		return
	}

	return
}

// Cleanup purges old entries, filtered by instanceID, any entry older then since, limited by limit.
func (t Table) Cleanup(db *sql.DB, instanceID int, since time.Time, limit int) (rows int64, err error) {
	query := fmt.Sprintf("DELETE FROM %s%s WHERE instance_id = ? AND %s < ? LIMIT %d",
		IcingaPrefix, t.Name, t.TimeColumn, limit)

	// log.Debugf("running query: %s - [%d %s]", query, instanceID, since)

	result, err := db.Exec(query, instanceID, since)
	if err != nil {
		err = fmt.Errorf("could not purge rows for %s: %w", t.Name, err)
		return
	}

	return result.RowsAffected() //nolint:wrapcheck
}

// Count returns the number of rows that should be deleted based on since.
func (t Table) Count(db *sql.DB, instanceID int, since time.Time) (rows int64, err error) {
	query := fmt.Sprintf("SELECT count(*) FROM %s%s WHERE instance_id = ? AND %s < ?", //nolint:gosec
		IcingaPrefix, t.Name, t.TimeColumn)

	// log.Debugf("running query: %s - [%d %s]", query, instanceID, since)

	row := db.QueryRow(query, instanceID, since)

	err = row.Scan(&rows)
	if err != nil {
		err = fmt.Errorf("could not purge rows for %s: %w", t.Name, err)
		return
	}

	return
}
