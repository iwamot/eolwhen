package projectfile

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"App.csproj", true},
		{"App.fsproj", true},
		{"App.vbproj", true},
		{"App.CSPROJ", true},
		{"App.csproj.user", false},
		{"App.sln", false},
		{"Directory.Build.props", false},
		{"csproj", false},
		{"", false},
	} {
		if got := Matches(tt.name); got != tt.want {
			t.Errorf("Matches(%q) = %v; want %v", tt.name, got, tt.want)
		}
	}
}

// project wraps properties in the markup a project file is made of, so that
// a case reads as the one line it is about.
func project(props string) string {
	return "<Project Sdk=\"Microsoft.NET.Sdk\">\n  <PropertyGroup>\n" + props + "  </PropertyGroup>\n</Project>\n"
}

type want struct {
	product string
	version string
	line    int
}

func check(t *testing.T, body string, ws ...want) {
	t.Helper()
	ds, us, _ := Extract("App.csproj", []byte(body))
	if len(us) != 0 {
		t.Fatalf("unreadable = %+v; want none", us)
	}
	if len(ds) != len(ws) {
		t.Fatalf("declarations = %+v; want %d", ds, len(ws))
	}
	for i, w := range ws {
		got := ds[i]
		if got.Ecosystem != "" {
			t.Errorf("[%d] ecosystem = %q; want none", i, got.Ecosystem)
		}
		if got.Product != w.product || got.Version != w.version {
			t.Errorf("[%d] = %s %q; want %s %q", i, got.Product, got.Version, w.product, w.version)
		}
		if got.Source != (decl.Source{File: "App.csproj", Line: w.line}) {
			t.Errorf("[%d] source = %s; want App.csproj:%d", i, got.Source, w.line)
		}
	}
}

func TestExtractReadsMonikers(t *testing.T) {
	for _, tt := range []struct {
		moniker string
		product string
		version string
	}{
		{"net6.0", "dotnet", "6.0"},
		{"net10.0", "dotnet", "10.0"},
		{"netcoreapp3.1", "dotnet", "3.1"},
		{"netcoreapp1.0", "dotnet", "1.0"},
		// A platform says which APIs the target adds, not which version of
		// it is being targeted.
		{"net8.0-windows10.0.19041.0", "dotnet", "8.0"},
		{"net6.0-android", "dotnet", "6.0"},
		// Bare digits are a .NET Framework, one digit per segment.
		{"net48", "dotnetfx", "4.8"},
		{"net481", "dotnetfx", "4.8.1"},
		{"net472", "dotnetfx", "4.7.2"},
		{"net40", "dotnetfx", "4.0"},
		{"net403", "dotnetfx", "4.0.3"},
		{"net20", "dotnetfx", "2.0"},
		{"net40-client", "dotnetfx", "4.0"},
		// The only 3.5 still supported is the service pack, and that is the
		// cycle endoflife.date tracks it under.
		{"net35", "dotnetfx", "3.5-sp1"},
		// MSBuild reads no case into a moniker, and neither does this.
		{"NET472", "dotnetfx", "4.7.2"},
		// Anything else is looked up under the name it gave.
		{"netstandard2.0", "netstandard", "2.0"},
		{"uap10.0", "uap", "10.0"},
		{"monoandroid12.0", "monoandroid", "12.0"},
		{"xamarin.ios", "xamarin.ios", ""},
		{"portable-net45+win8", "portable", ""},
	} {
		t.Run(tt.moniker, func(t *testing.T) {
			check(t, project("    <TargetFramework>"+tt.moniker+"</TargetFramework>\n"),
				want{tt.product, tt.version, 3})
		})
	}
}

func TestExtractReadsSeveralTargets(t *testing.T) {
	check(t, project("    <OutputType>Exe</OutputType>\n    <TargetFrameworks>net8.0;net48;netstandard2.0</TargetFrameworks>\n"),
		want{"dotnet", "8.0", 4},
		want{"dotnetfx", "4.8", 4},
		want{"netstandard", "2.0", 4})
}

func TestExtractIgnoresEmptyEntries(t *testing.T) {
	check(t, project("    <TargetFrameworks>net8.0; ;;net48</TargetFrameworks>\n"),
		want{"dotnet", "8.0", 3},
		want{"dotnetfx", "4.8", 3})
}

// A property group may be conditional, and the condition is where an
// MSBuild property is most often written; what is read is the value.
func TestExtractReadsEveryPropertyGroup(t *testing.T) {
	check(t, "<Project>\n"+
		"  <PropertyGroup>\n    <TargetFramework>net6.0</TargetFramework>\n  </PropertyGroup>\n"+
		"  <PropertyGroup Condition=\"'$(OS)' == 'Windows_NT'\">\n    <TargetFramework>net48</TargetFramework>\n  </PropertyGroup>\n"+
		"</Project>\n",
		want{"dotnet", "6.0", 3},
		want{"dotnetfx", "4.8", 6})
}

func TestExtractReadsTheLegacyProperty(t *testing.T) {
	for _, tt := range []struct {
		value   string
		version string
	}{
		{"v4.7.2", "4.7.2"},
		{"v4.0", "4.0"},
		{"v3.5", "3.5-sp1"},
		{"V4.8", "4.8"},
	} {
		t.Run(tt.value, func(t *testing.T) {
			check(t, "<Project ToolsVersion=\"12.0\" xmlns=\"http://schemas.microsoft.com/developer/msbuild/2003\">\n"+
				"  <PropertyGroup>\n    <TargetFrameworkVersion>"+tt.value+"</TargetFrameworkVersion>\n  </PropertyGroup>\n</Project>\n",
				want{"dotnetfx", tt.version, 3})
		})
	}
}

func TestExtractReadsAcrossLines(t *testing.T) {
	check(t, project("    <TargetFramework>\n      net6.0\n    </TargetFramework>\n"),
		want{"dotnet", "6.0", 3})
}

func TestExtractSkipsAByteOrderMark(t *testing.T) {
	check(t, "\xef\xbb\xbf"+project("    <TargetFramework>net6.0</TargetFramework>\n"),
		want{"dotnet", "6.0", 3})
}

func TestExtractReportsWhatItCannotRead(t *testing.T) {
	for _, tt := range []struct {
		name   string
		props  string
		text   string
		reason string
		moving bool
	}{
		{
			"an MSBuild property",
			"    <TargetFramework>$(DefaultTargetFramework)</TargetFramework>\n",
			"$(DefaultTargetFramework)",
			"is written as an MSBuild property, whose value is set outside this file",
			true,
		},
		{
			"a version with no framework",
			"    <TargetFramework>8.0</TargetFramework>\n",
			"8.0",
			"is not a target framework moniker",
			false,
		},
		{
			"a platform with no framework",
			"    <TargetFramework>-windows</TargetFramework>\n",
			"-windows",
			"is not a target framework moniker",
			false,
		},
		{
			"a framework with no version",
			"    <TargetFramework>net</TargetFramework>\n",
			"net",
			"is not a target framework moniker",
			false,
		},
		{
			"a platform version with a letter in it",
			"    <TargetFramework>uap1x.0</TargetFramework>\n",
			"uap1x.0",
			"is not a target framework moniker",
			false,
		},
		{
			"a .NET version with a letter in it",
			"    <TargetFramework>net4x</TargetFramework>\n",
			"net4x",
			"is not a target framework moniker",
			false,
		},
		{
			"a legacy value without its v",
			"    <TargetFrameworkVersion>4.7.2</TargetFrameworkVersion>\n",
			"4.7.2",
			"is not a .NET Framework version",
			false,
		},
		{
			"a legacy value that is a word",
			"    <TargetFrameworkVersion>vLatest</TargetFrameworkVersion>\n",
			"vLatest",
			"is not a .NET Framework version",
			false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, _ := Extract("App.csproj", []byte(project(tt.props)))
			if len(ds) != 0 {
				t.Fatalf("declarations = %+v; want none", ds)
			}
			want := decl.Unreadable{
				Source: decl.Source{File: "App.csproj", Line: 3},
				Text:   tt.text,
				Reason: tt.reason,
				Moving: tt.moving,
			}
			if len(us) != 1 || us[0] != want {
				t.Errorf("unreadable = %+v; want [%+v]", us, want)
			}
		})
	}
}

func TestExtractReadsNothingElse(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{"an empty file", ""},
		{"markup that does not parse", "<Project><PropertyGroup><TargetFramework>net6.0</Project>"},
		{"a document that is not a project", "{\"TargetFramework\": \"net6.0\"}"},
		{"an unterminated property", "<Project>\n  <PropertyGroup>\n    <TargetFramework>net6.0"},
		{"markup a property does not close", project("    <TargetFramework>net6.0<X></TargetFramework>\n")},
		{"an encoding nothing here can read", "<?xml version=\"1.0\" encoding=\"windows-1252\"?>\n" +
			project("    <TargetFramework>net6.0</TargetFramework>\n")},
		{"an empty property", project("    <TargetFramework></TargetFramework>\n")},
		// The same name is metadata on a project reference, where it says
		// which build of another project to use.
		{"metadata on a project reference", "<Project>\n  <ItemGroup>\n" +
			"    <ProjectReference Include=\"../Lib/Lib.csproj\">\n      <TargetFramework>net48</TargetFramework>\n" +
			"    </ProjectReference>\n  </ItemGroup>\n</Project>\n"},
		{"a property outside a property group", "<Project>\n  <TargetFramework>net6.0</TargetFramework>\n</Project>\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, _ := Extract("App.csproj", []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("= %+v, %+v; want nothing", ds, us)
			}
		})
	}
}

// Markup inside a property is passed over, the version being the text.
func TestExtractPassesOverMarkupInsideAProperty(t *testing.T) {
	for _, tt := range []struct {
		name  string
		props string
	}{
		{"a comment", "    <TargetFramework>net6.0<!-- and no other --></TargetFramework>\n"},
		{"an element", "    <TargetFramework>net6.0<Ignored>net48</Ignored></TargetFramework>\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			check(t, project(tt.props), want{"dotnet", "6.0", 3})
		})
	}
}

// Markup that does not parse sets the file aside whole, and what was read
// before the decoder stopped goes with it: half a file is not half a set of
// declarations. A file that parses and declares no target framework is not
// set aside at all.
func TestExtractSetsAsideAFileItCannotParse(t *testing.T) {
	for _, tt := range []struct{ name, body, skipped string }{
		{"markup that does not parse", "<Project><PropertyGroup><TargetFramework>net6.0</Project>", decl.NotXML},
		{"a property the file never closes", "<Project>\n  <PropertyGroup>\n    <TargetFramework>net6.0", decl.NotXML},
		// The first group is whole and the second is not, and the whole
		// file goes: a run that kept net6.0 here would report a directory
		// as building for one runtime when the file names two.
		{"a second group cut short", project("    <TargetFramework>net6.0</TargetFramework>\n") +
			"<Project>\n  <PropertyGroup>\n    <TargetFramework>net7.0", decl.NotXML},
		{"no target framework in it", project("    <Nothing>x</Nothing>\n"), ""},
		{"empty", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, skipped := Extract("App.csproj", []byte(tt.body))
			if skipped != tt.skipped {
				t.Errorf("skipped = %q; want %q", skipped, tt.skipped)
			}
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}
