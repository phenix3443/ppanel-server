package buildinfo

import "testing"

func TestChannelLabel(t *testing.T) {
	cases := map[string]string{
		ChannelStable:    "",
		ChannelBeta:      "Beta",
		ChannelNightly:   "Nightly",
		ChannelDev:       "Develop",
		"":               "Develop",
		"something-else": "Develop",
	}
	for channel, want := range cases {
		if got := ChannelLabel(channel); got != want {
			t.Fatalf("ChannelLabel(%q) = %q, want %q", channel, got, want)
		}
	}
}

// A stable release must not be labelled a development build. The label used to be
// inferred from a leading "v", which every release pipeline strips, so published
// releases reported themselves as "Develop" in the admin panel.
func TestDisplayLabelsChannelNotStringShape(t *testing.T) {
	cases := []struct {
		name      string
		version   string
		buildTime string
		channel   string
		want      string
	}{
		{name: "stable", version: "1.15.12", buildTime: "2026-08-06", channel: ChannelStable, want: "1.15.12(2026-08-06)"},
		{name: "stable with tag prefix", version: "v1.15.12", buildTime: "2026-08-06", channel: ChannelStable, want: "1.15.12(2026-08-06)"},
		{name: "beta", version: "1.16.0-beta.1", buildTime: "2026-08-06", channel: ChannelBeta, want: "1.16.0-beta.1(2026-08-06) Beta"},
		{name: "nightly", version: "f69eb4c", buildTime: "2026-08-06", channel: ChannelNightly, want: "f69eb4c(2026-08-06) Nightly"},
		{name: "local build", version: "1.15.12-21-gf69eb4c", buildTime: "2026-08-06", channel: ChannelDev, want: "1.15.12-21-gf69eb4c(2026-08-06) Develop"},
		{name: "nothing injected", version: "unknown version", buildTime: "unknown time", channel: ChannelDev, want: "unknown(unknown) Develop"},
	}

	origVersion, origBuildTime, origChannel := Version, BuildTime, Channel
	t.Cleanup(func() {
		Version, BuildTime, Channel = origVersion, origBuildTime, origChannel
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Version, BuildTime, Channel = tc.version, tc.buildTime, tc.channel
			if got := Display(); got != tc.want {
				t.Fatalf("Display() = %q, want %q", got, tc.want)
			}
		})
	}
}
