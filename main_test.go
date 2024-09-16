package main

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetInstanceID_WithOK(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	expected := 1

	rows := sqlmock.NewRows([]string{"instance_id"}).
		AddRow(expected)

	mock.ExpectQuery("SELECT instance_id FROM icinga_instances WHERE instance_name = ?").WillReturnRows(rows)

	id, err := getInstanceID(db, "default")

	if err != nil {
		t.Errorf("error was not expected while getting instance: %s", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}

	if id != expected {
		t.Errorf("actual: %d, expected: %d", id, expected)
	}
}

func TestGetInstanceID_WithError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	_, DbErr := getInstanceID(db, "default")

	if err != nil {
		t.Errorf("error was not expected while getting instance: %s", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}

	if DbErr == nil {
		t.Errorf("expected an error got nil")
	}
}

func TestOldestTime_WithOK(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	expected := "2024-07-02 08:25:44"
	rows := sqlmock.NewRows([]string{"start_time"}).
		AddRow(expected)

	mock.ExpectQuery("SELECT start_time FROM icinga_notifications WHERE (.+) ORDER BY start_time ASC LIMIT 1").WillReturnRows(rows)

	table := Table{"notifications", "start_time"}
	oldest, err := table.OldestTime(db, 1)

	if "2024-07-02 08:25:44 +0000 UTC" != oldest.String() {
		t.Errorf("returned timestamp not expected: %s", oldest.String())
	}

	if err != nil {
		t.Errorf("error was not expected while getting oldest time: %s", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestCleanup_WithOK(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	ts := time.Now()
	mock.ExpectExec("DELETE FROM icinga_notifications WHERE (.+) AND start_time (.+) LIMIT 10").
		WithArgs(1, ts).
		WillReturnResult(sqlmock.NewResult(1, 1))

	table := Table{"notifications", "start_time"}
	r, err := table.Cleanup(db, 1, ts, 10)

	if 1 != r {
		t.Errorf("returned rows not expected: %d", r)
	}

	if err != nil {
		t.Errorf("error was not expected while cleanup: %s", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestCount_WithOK(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"count(*)"}).
		AddRow(123)

	ts := time.Now()
	mock.ExpectQuery("SELECT count(.+) FROM icinga_notifications WHERE instance_id = (.+) AND start_time < (.+)").
		WithArgs(1, ts).
		WillReturnRows(rows)

	table := Table{"notifications", "start_time"}
	r, err := table.Count(db, 1, ts)

	if 123 != r {
		t.Errorf("returned rows not expected: %d", r)
	}

	if err != nil {
		t.Errorf("error was not expected while cleanup: %s", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}
