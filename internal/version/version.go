package version

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns a stable single-line version string for CLI output.
func String() string {
	return fmt.Sprintf(
		"translategemma-ui version=%s commit=%s date=%s",
		normalized(Version, "dev"),
		normalized(Commit, "unknown"),
		normalized(Date, "unknown"),
	)
}

func normalized(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
