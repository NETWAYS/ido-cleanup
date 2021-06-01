package main

import "fmt"

// nolint: gochecknoglobals
var (
	version = "0.0.0-dev"
	commit  = ""
	date    = ""
)

//goland:noinspection GoBoolExpressions
func buildVersion() string {
	result := version

	if commit != "" {
		result = fmt.Sprintf("%s\ncommit: %s", result, commit)
	}

	if date != "" {
		result = fmt.Sprintf("%s\ndate: %s", result, date)
	}

	return result
}
