package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// These values are expected to be overridden via -ldflags in release builds.
var (
	Version = "0.3.0"
	Commit  = "unknown"
	Date    = ""
)

type Info struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	Date       string `json:"date,omitempty"`
	GoVersion  string `json:"go_version"`
	SDKVersion string `json:"sdk_version,omitempty"`
}

func Current() Info {
	return Info{
		Version:    Version,
		Commit:     Commit,
		Date:       Date,
		GoVersion:  runtime.Version(),
		SDKVersion: dependencyVersion("github.com/openai/openai-go/v3"),
	}
}

func dependencyVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == path {
			if dep.Replace != nil && strings.TrimSpace(dep.Replace.Version) != "" {
				return dep.Replace.Version
			}
			return dep.Version
		}
	}
	return ""
}
