package version

import "strings"

var (
	Version = "0.1.0"
	Commit  = "unknown"
	Built   = "unknown"
)

func UserAgent() string {
	value := strings.TrimSpace(Version)
	if value == "" {
		value = "dev"
	}
	return "ask/" + value
}
