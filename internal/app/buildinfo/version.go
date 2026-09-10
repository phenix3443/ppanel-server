package buildinfo

import (
	"fmt"
	"strings"
)

// Release channels. A build that does not have Channel injected is a local
// developer build.
const (
	ChannelStable  = "stable"
	ChannelBeta    = "beta"
	ChannelNightly = "nightly"
	ChannelDev     = "dev"
)

// Version PPanel version
var (
	Version   = "unknown version"
	BuildTime = "unknown time"
	// Channel names the release channel this binary was built for. Every build
	// path injects it alongside Version; never infer the channel from the shape
	// of the version string.
	Channel = ChannelDev
)

// ChannelLabel returns the suffix shown next to the version for builds that did
// not come from the stable channel. Stable builds carry no suffix.
func ChannelLabel(channel string) string {
	switch channel {
	case ChannelStable:
		return ""
	case ChannelBeta:
		return "Beta"
	case ChannelNightly:
		return "Nightly"
	default:
		return "Develop"
	}
}

// Display returns the version string the CLI and the admin API show operators.
// The channel comes from Channel and never from the shape of Version: the release
// pipelines strip the tag's leading "v", so classifying a build by that prefix
// reported every published release as a development build.
func Display() string {
	version := Version
	if version == "" || version == "unknown version" {
		version = "unknown"
	}
	buildTime := BuildTime
	if buildTime == "" || buildTime == "unknown time" {
		buildTime = "unknown"
	}

	display := fmt.Sprintf("%s(%s)", strings.TrimPrefix(version, "v"), buildTime)
	if label := ChannelLabel(Channel); label != "" {
		display += " " + label
	}
	return display
}
