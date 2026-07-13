// Package buildinfo reports the version of the running Struktly binary.
package buildinfo

import "runtime/debug"

// Version, Revision, and Date may be set with -ldflags when producing a
// release artifact. go install records the module version in build info.
var (
	Version  = "devel"
	Revision string
	Date     string
)

// Info is the machine-readable identity of one Struktly binary.
type Info struct {
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	Date     string `json:"date,omitempty"`
}

// Current returns build metadata without requiring a Git checkout at runtime.
func Current() Info {
	info := Info{Version: Version, Revision: Revision, Date: Date}
	if build, ok := debug.ReadBuildInfo(); ok {
		if info.Version == "devel" && build.Main.Version != "" && build.Main.Version != "(devel)" {
			info.Version = build.Main.Version
		}
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				if info.Revision == "" {
					info.Revision = setting.Value
				}
			case "vcs.time":
				if info.Date == "" {
					info.Date = setting.Value
				}
			}
		}
	}
	return info
}
