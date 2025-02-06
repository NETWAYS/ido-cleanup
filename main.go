package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql" // do not remove this although it looks weird. It adds the mysql db driver
	"github.com/spf13/pflag"
)

const readme = `
Icinga IDO Cleanup
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

	// Default log options
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	if debug {
		opts.Level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	logger.Info("starting ido-cleanup")

	// Setup database connection
	db, err := sql.Open("mysql", dbDsn)
	if err != nil {
		logger.Error("could not connect to database", "error", err)
		os.Exit(1)
	}

	err = db.Ping()
	if err != nil {
		logger.Error("could not connect to database", "error", err)
		os.Exit(1)
	}

	db.SetConnMaxLifetime(time.Minute * 15)

	// Load instance ID
	instanceID, err := getInstanceID(db, instance)
	if err != nil {
		_ = db.Close()

		logger.Error("could not get instance ID", "error", err)
		os.Exit(1)
	}

	defer db.Close()

	// Signal handler
	interrupt := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	alive := true

	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Start initial cleanup and prepare timer
	currentInterval := interval

	if runCleanup(db, instanceID, logger) {
		logger.Debug("updating interval", "interval", fastInterval)
		currentInterval = fastInterval
	}

	// Stop here when only once is requested
	if once {
		logger.Info("stopping after one cleanup")
		return
	}

	timer := time.NewTimer(currentInterval)

	go func() {
		sig := <-interrupt

		logger.Info("received signal", "signal", sig)
		timer.Stop()

		done <- true
	}()

	for alive {
		select {
		case <-done:
			alive = false
		case <-timer.C:
			nextInterval := interval

			if runCleanup(db, instanceID, logger) {
				nextInterval = fastInterval
			}

			if currentInterval != nextInterval {
				logger.Debug("updating interval", "interval", nextInterval)
				timer.Reset(nextInterval)
			}
		}
	}

	logger.Info("stopping ido-cleanup")
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

	// knownTables is a slice of Table structs
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

func runCleanup(db *sql.DB, instanceID int, logger *slog.Logger) (busy bool) {
	for _, table := range knownTables {
		age, set := ages[table.Name]
		if !set || *age == 0 {
			continue
		}

		start := time.Now()

		// Look for the time stamp of the oldest entry and log it
		oldest, err := table.OldestTime(db, instanceID)
		if err != nil {
			logger.Error("could not get entry", "error", err, "table", table.Name)
		}

		// Until when we want to delete
		if *age > math.MaxInt {
			logger.Error("age is limit to MaxInt", "error", err, "age", *age)
		}
		deleteSince := time.Now().AddDate(0, 0, -int(*age)) //nolint: gosec

		// Only count?
		if noop {
			rows, err := table.Count(db, instanceID, deleteSince)
			if err != nil {
				logger.Error("could not enumerate rows", "error", err, "table", table.Name, "oldest", oldest)
				continue
			}

			logger.Info("would delete rows",
				"table", table.Name,
				"oldest", oldest,
				"rows", rows,
				"took", time.Since(start),
			)

			continue
		}

		// Run the cleanup
		rows, err := table.Cleanup(db, instanceID, deleteSince, limit)
		if err != nil {
			logger.Error("could run cleanup", "error", err, "table", table.Name)
			continue
		}

		// when we deleted as much rows as the limit is, return true, so we can switch to a faster interval
		if rows >= int64(limit) {
			busy = true
		}

		if rows > 0 {
			logger.Info("deleted rows",
				"table", table.Name,
				"oldest", oldest,
				"rows", rows,
				"took", time.Since(start),
			)
		} else {
			logger.Debug("deleted rows",
				"table", table.Name,
				"oldest", oldest,
				"rows", rows,
				"took", time.Since(start),
			)
		}
	}

	return
}

func getInstanceID(db *sql.DB, instance string) (id int, err error) {
	row := db.QueryRow("SELECT instance_id FROM icinga_instances WHERE instance_name = ?", instance)

	err = row.Scan(&id)
	if err != nil {
		err = fmt.Errorf("could not find instance '%s': %w", instance, err)
		return
	}

	return
}
