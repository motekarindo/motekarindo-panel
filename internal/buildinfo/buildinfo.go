package buildinfo

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

type BuildInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func Info() BuildInfo {
	return BuildInfo{
		Name:    "Motekar Panel",
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}

func String() string {
	return fmt.Sprintf("%s %s (%s, %s)", Info().Name, Version, Commit, Date)
}
