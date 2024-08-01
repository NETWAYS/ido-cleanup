package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

const readme = `
Icinga IDO Cleanup

For more details see: https://github.com/NETWAYS/ido-cleanup
`

var (
	dbDsn        = "icinga:icinga@/icinga2"
	instance     = "default"
	noop         bool
	debug        bool
	once         bool
	printVersion bool
	interval     = 60 * time.Second
	fastInterval = 10 * time.Second
	limit        = 10000
	ages         = map[string]*uint{}
)

var defaultAges = map[string]uint{
	"statehistory":         365,
	"contactnotifications": 365,
	"notifications":        365,
	"logentries":           365,
	"downtimehistory":      365,
	"commenthistory":       365,
	"eventhandlers":        365,
}

func main() {
	handleArguments()

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.Info("starting ido-cleanup")

	// Setup database connection
	db, err := sql.Open("mysql", dbDsn)
	if err != nil {
		logrus.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		logrus.Fatal("could not connect to database: ", err)
	}

	db.SetConnMaxLifetime(time.Minute * 15)

	// Load instance ID
	instanceID, err := getInstanceID(db)
	if err != nil {
		_ = db.Close()

		logrus.Fatal(err)
	}

	defer db.Close()

	// Signal handler
	interrupt := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	alive := true

	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Start initial cleanup and prepare timer
	currentInterval := interval

	if runCleanup(db, instanceID) {
		logrus.WithField("interval", fastInterval).Debug("updating interval")

		currentInterval = fastInterval
	}

	// Stop here when only once is requested
	if once {
		logrus.Info("stopping after one cleanup")
		return
	}

	timer := time.NewTimer(currentInterval)

	go func() {
		sig := <-interrupt

		logrus.Info("received signal ", sig)
		timer.Stop()

		done <- true
	}()

	for alive {
		select {
		case <-done:
			alive = false
		case <-timer.C:
			nextInterval := interval

			if runCleanup(db, instanceID) {
				nextInterval = fastInterval
			}

			if currentInterval != nextInterval {
				logrus.WithField("interval", nextInterval).Debug("updating interval")

				timer.Reset(nextInterval)
			}
		}
	}

	logrus.Info("stopping ido-cleanup")
}

func handleArguments() {
	if v := os.Getenv("DB_DSN"); v != "" {
		dbDsn = v
	}

	pflag.StringVar(&dbDsn, "db", dbDsn, "DB Connecting string (env:DB_DSN)")
	pflag.StringVar(&instance, "instance", instance, "IDO instance name")
	pflag.IntVar(&limit, "limit", limit, "Limit deleting rows in one query")
	pflag.DurationVar(&interval, "interval", interval, "Cleanup every X seconds")
	pflag.DurationVar(&fastInterval, "fast-interval", fastInterval,
		"Cleanup every X seconds - when more then 2x limit rows to delete")
	pflag.BoolVar(&once, "once", false, "Just run once")
	pflag.BoolVar(&noop, "noop", false, "Just check - don't purge")
	pflag.BoolVar(&debug, "debug", false, "Enable debug logging")
	pflag.BoolVarP(&printVersion, "version", "V", false, "Print version and exit")

	pflag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stdout, "%s\n\n", strings.Trim(readme, "\r\n"))
		_, _ = fmt.Fprintf(os.Stdout, "Usage of %s:\n", os.Args[0])

		pflag.PrintDefaults()
	}

	for _, table := range knownTables {
		age := uint(0)
		if v, ok := defaultAges[table.Name]; ok {
			age = v
		}

		ages[table.Name] = &age

		pflag.UintVar(ages[table.Name], table.Name, age, "How long to keep entries of "+table.Name+" in days")
	}

	pflag.CommandLine.SortFlags = false
	pflag.Parse()

	if printVersion {
		_, _ = fmt.Fprintf(os.Stdout, "NETWAYS ido-cleanup version %s\n", buildVersion())
		os.Exit(0)
	}
}

func runCleanup(db *sql.DB, instanceID int) (busy bool) {
	for _, table := range knownTables {
		age, set := ages[table.Name]
		if !set || *age == 0 {
			continue
		}

		start := time.Now()
		entry := logrus.WithField("table", table.Name)

		// Look for the time stamp of the oldest entry and log it
		oldest, err := table.OldestTime(db, instanceID)
		if err != nil {
			entry.Error(err)
		} else if !oldest.IsZero() {
			entry = entry.WithField("oldest", oldest)
		}

		// Until when we want to delete
		deleteSince := time.Now().AddDate(0, 0, -int(*age))

		// Only count?
		if noop {
			rows, err := table.Count(db, instanceID, deleteSince)
			if err != nil {
				entry.Error(err)

				continue
			}

			entry.WithFields(logrus.Fields{
				"rows": rows,
				"took": time.Since(start),
			}).Info("would delete rows")

			continue
		}

		// Run the cleanup
		rows, err := table.Cleanup(db, instanceID, deleteSince, limit)
		if err != nil {
			entry.Error(err)

			continue
		}

		// when we deleted as much rows as the limit is, return true, so we can switch to a faster interval
		if rows >= int64(limit) {
			busy = true
		}

		entry = entry.WithFields(logrus.Fields{
			"rows": rows,
			"took": time.Since(start),
		})

		if rows > 0 {
			entry.Info("deleted rows")
		} else {
			entry.Debug("deleted rows")
		}
	}

	return
}

func getInstanceID(db *sql.DB) (id int, err error) {
	row := db.QueryRow("SELECT instance_id FROM icinga_instances WHERE instance_name = ?", instance)

	err = row.Scan(&id)
	if err != nil {
		err = fmt.Errorf("could not find instance '%s': %w", instance, err)
		return
	}

	return
}
