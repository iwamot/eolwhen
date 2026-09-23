package ruby

import "testing"

func TestRead(t *testing.T) {
	const (
		dev    = "names a development build, not a version"
		newest = "names the newest stable release, not a version"
	)
	for _, tt := range []struct {
		in                      string
		engine, version, reason string
	}{
		// The reference implementation, written with its name or without.
		{"3.3", "ruby", "3.3", ""},
		{"ruby-2.6.5", "ruby", "2.6.5", ""},
		// Another implementation is its own software.
		{"jruby-9.4", "jruby", "9.4", ""},
		{"jruby-9.4.8.0", "jruby", "9.4.8.0", ""},
		{"truffleruby-24.1", "truffleruby", "24.1", ""},
		// A build of TruffleRuby, not the GraalVM the catalog knows.
		{"truffleruby+graalvm-24.1", "truffleruby+graalvm", "24.1", ""},
		// Builds of a development branch follow it on purpose.
		{"head", "ruby", "head", dev},
		{"ucrt", "ruby", "ucrt", dev},
		{"ruby-head", "ruby", "head", dev},
		{"ruby-debug", "ruby", "debug", dev},
		{"jruby-head", "jruby", "head", dev},
		{"truffleruby+graalvm-head", "truffleruby+graalvm", "head", dev},
		// An implementation on its own is its newest stable release.
		{"ruby", "ruby", "", newest},
		{"jruby", "jruby", "", newest},
		{"truffleruby", "truffleruby", "", newest},
		// A word naming a build of the reference implementation only.
		{"jruby-ucrt", "jruby", "ucrt", ""},
		// A word the action does not know is left for the caller.
		{"foo", "ruby", "foo", ""},
		{"mruby-3.2", "ruby", "mruby-3.2", ""},
	} {
		t.Run(tt.in, func(t *testing.T) {
			engine, version, reason := Read(tt.in)
			if engine != tt.engine || version != tt.version || reason != tt.reason {
				t.Errorf("Read(%q) = %q, %q, %q; want %q, %q, %q", tt.in, engine, version, reason, tt.engine, tt.version, tt.reason)
			}
		})
	}
}
