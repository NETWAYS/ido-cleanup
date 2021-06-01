package main

import (
	"database/sql"
	"fmt"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
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
		log.SetLevel(log.DebugLevel)
	}

	log.Info("starting ido-cleanup")

	// Setup database connection
	db, err := sql.Open("mysql", dbDsn)
	if err != nil {
		log.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal("could not connect to database: ", err)
	}

	db.SetConnMaxLifetime(time.Minute * 15)

	// Load instance ID
	instanceID, err := getInstanceID(db)
	if err != nil {
		_ = db.Close()

		log.Fatal(err)
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
		log.WithField("interval", fastInterval).Debug("updating interval")

		currentInterval = fastInterval
	}

	// Stop here when only once is requested
	if once {
		log.Info("stopping after one cleanup")
		return
	}

	timer := time.NewTimer(currentInterval)

	go func() {
		sig := <-interrupt

		log.Info("received signal ", sig)
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
				log.WithField("interval", nextInterval).Debug("updating interval")

				timer.Reset(nextInterval)
			}
		}
	}

	log.Info("stopping ido-cleanup")
}

func handleArguments() {
	if v := os.Getenv("DB_DSN"); v != "" {
		dbDsn = v
	}

	flag.StringVar(&dbDsn, "db", dbDsn, "DB Connecting string (env:DB_DSN)")
	flag.StringVar(&instance, "instance", instance, "IDO instance name")
	flag.IntVar(&limit, "limit", limit, "Limit deleting rows in one query")
	flag.DurationVar(&interval, "interval", interval, "Cleanup every X seconds")
	flag.DurationVar(&fastInterval, "fast-interval", fastInterval,
		"Cleanup every X seconds - when more then 2x limit rows to delete")
	flag.BoolVar(&once, "once", false, "Just run once")
	flag.BoolVar(&noop, "noop", false, "Just check - don't purge")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.BoolVarP(&printVersion, "version", "V", false, "Print version and exit")

	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stdout, "%s\n\n", strings.Trim(readme, "\r\n"))
		_, _ = fmt.Fprintf(os.Stdout, "Usage of %s:\n", os.Args[0])

		flag.PrintDefaults()
	}

	for _, table := range knownTables {
		age := uint(0)
		if v, ok := defaultAges[table.Name]; ok {
			age = v
		}

		ages[table.Name] = &age

		flag.UintVar(ages[table.Name], table.Name, age, "How long to keep entries of "+table.Name+" in days")
	}

	flag.CommandLine.SortFlags = false
	flag.Parse()

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
		entry := log.WithField("table", table.Name)

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

			entry.WithFields(log.Fields{
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

		entry = entry.WithFields(log.Fields{
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
