package pomxml

import (
	"testing"

	"github.com/iwamot/eolwhen/internal/decl"
)

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"pom.xml", true},
		{"pom.xml.bak", false},
		{"build.gradle", false},
		{"Pom.xml", false},
		{"", false},
	} {
		if got := Matches(tt.name); got != tt.want {
			t.Errorf("Matches(%q) = %v; want %v", tt.name, got, tt.want)
		}
	}
}

type want struct {
	product string
	version string
	line    int
}

func check(t *testing.T, body string, ws ...want) {
	t.Helper()
	ds, _, _ := Extract("pom.xml", []byte(body))
	if len(ds) != len(ws) {
		t.Fatalf("declarations = %+v; want %d", ds, len(ws))
	}
	for i, w := range ws {
		got := ds[i]
		if got.Ecosystem != Ecosystem {
			t.Errorf("[%d] ecosystem = %q; want %q", i, got.Ecosystem, Ecosystem)
		}
		if got.Product != w.product || got.Version != w.version {
			t.Errorf("[%d] = %s %q; want %s %q", i, got.Product, got.Version, w.product, w.version)
		}
		if got.Source != (decl.Source{File: "pom.xml", Line: w.line}) {
			t.Errorf("[%d] source = %s; want pom.xml:%d", i, got.Source, w.line)
		}
	}
}

const pom = `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
    <version>2.7.18</version>
  </parent>
  <properties>
    <struts.version>2.5.20</struts.version>
  </properties>
  <dependencies>
    <dependency>
      <groupId>log4j</groupId>
      <artifactId>log4j</artifactId>
      <version>1.2.17</version>
      <exclusions>
        <exclusion>
          <groupId>javax.mail</groupId>
          <artifactId>mail</artifactId>
        </exclusion>
      </exclusions>
    </dependency>
    <dependency>
      <groupId>org.apache.struts</groupId>
      <artifactId>struts2-core</artifactId>
      <version>${struts.version}</version>
    </dependency>
  </dependencies>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>org.apache.tomcat</groupId>
        <artifactId>tomcat</artifactId>
        <version>9.0.60</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <profiles>
    <profile>
      <dependencies>
        <dependency>
          <groupId>org.hibernate</groupId>
          <artifactId>hibernate-core</artifactId>
          <version>5.6.15.Final</version>
        </dependency>
      </dependencies>
    </profile>
  </profiles>
</project>
`

func TestExtract(t *testing.T) {
	check(t, pom,
		// The line kept is the version's, that being the line to go and
		// change.
		want{"org.springframework.boot/spring-boot-starter-parent", "2.7.18", 6},
		// What sits inside an exclusion is not this dependency's group.
		want{"log4j/log4j", "1.2.17", 15},
		// A property this same file sets stands in for its reference.
		want{"org.apache.struts/struts2-core", "2.5.20", 26},
		// dependencyManagement is where a project says which version its
		// modules build with, and a profile is the same thing conditionally.
		want{"org.apache.tomcat/tomcat", "9.0.60", 34},
		want{"org.hibernate/hibernate-core", "5.6.15.Final", 44})
}

func TestExtractSetsAsideWhatThisFileDoesNotSettle(t *testing.T) {
	const (
		fromParent   = "names no version here, so the POM it inherits from decides"
		fromProperty = "takes its version from a property this file does not settle"
		newest       = "asks for whatever is newest, not a version"
		aRange       = "names a range of versions rather than one, so the build decides which one"
	)
	for _, tt := range []struct{ name, version, text, reason string }{
		// The usual way a POM inheriting from a parent is written.
		{"no version at all", "", "", fromParent},
		{"a property set in the parent", "<version>${spring.version}</version>", "${spring.version}", fromProperty},
		{"a property inside a value", "<version>1.${minor}</version>", "1.${minor}", fromProperty},
		{"a property with more after it", "<version>${v}-SNAPSHOT</version>", "${v}-SNAPSHOT", fromProperty},
		{"a reference nothing closes", "<version>${v</version>", "${v", fromProperty},
		{"a range", "<version>[1.0,2.0)</version>", "[1.0,2.0)", aRange},
		{"the newest release", "<version>LATEST</version>", "LATEST", newest},
		{"the newest stable release", "<version>RELEASE</version>", "RELEASE", newest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := "<project>\n  <dependencies>\n    <dependency>\n      <groupId>g</groupId>\n" +
				"      <artifactId>a</artifactId>\n      " + tt.version + "\n    </dependency>\n  </dependencies>\n</project>\n"
			ds, us, _ := Extract("pom.xml", []byte(body))
			if len(ds) != 0 {
				t.Fatalf("declarations = %+v; want none", ds)
			}
			if len(us) != 1 || us[0].Product != "g/a" || us[0].Text != tt.text || us[0].Reason != tt.reason || !us[0].Moving {
				t.Errorf("unreadable = %+v; want g/a %q set aside as %q", us, tt.text, tt.reason)
			}
		})
	}
}

func TestExtractReadsNothingElse(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"empty", ""},
		{"markup that does not parse", "<project><dependencies><dependency></project>"},
		{"a document that is not a POM", "{\"dependencies\": []}"},
		{"a POM declaring nothing", "<project>\n  <modelVersion>4.0.0</modelVersion>\n</project>\n"},
		// Maven needs a group and an artifact to name one, and so does a
		// purl.
		{"a dependency with no group", "<project>\n  <dependencies>\n    <dependency>\n      <artifactId>a</artifactId>\n      <version>1.0</version>\n    </dependency>\n  </dependencies>\n</project>\n"},
		{"a dependency with no artifact", "<project>\n  <dependencies>\n    <dependency>\n      <groupId>g</groupId>\n      <version>1.0</version>\n    </dependency>\n  </dependencies>\n</project>\n"},
		// A parent is the project's own, and a dependency lives under a
		// dependencies list; the same names elsewhere are something else.
		{"a parent somewhere else", "<project>\n  <build>\n    <parent>\n      <groupId>g</groupId>\n      <artifactId>a</artifactId>\n      <version>1.0</version>\n    </parent>\n  </build>\n</project>\n"},
		{"a properties table somewhere else", "<project>\n  <profiles>\n    <properties>\n      <v>1.0</v>\n    </properties>\n  </profiles>\n</project>\n"},
		{"markup a dependency does not close", "<project>\n  <dependencies>\n    <dependency>\n      <groupId>g<X></groupId>\n"},
		{"a tag closed by another", "<project></module>"},
		{"markup a property does not close", "<project>\n  <properties>\n    <v><X></v>\n  </properties>\n</project>\n"},
		{"a file cut short inside an element", "<project>\n  <dependencies>\n    <dependency>\n      <groupId>g"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, _ := Extract("pom.xml", []byte(tt.body))
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("= %+v, %+v; want neither", ds, us)
			}
		})
	}
}

// A file Visual Studio or an older editor wrote may start with a byte order
// mark, which is not markup.
func TestExtractSkipsAByteOrderMark(t *testing.T) {
	check(t, "\xef\xbb\xbf<project>\n  <parent>\n    <groupId>g</groupId>\n    <artifactId>a</artifactId>\n    <version>1.0</version>\n  </parent>\n</project>\n",
		want{"g/a", "1.0", 5})
}

// A property may be set after the dependency that uses it, the file being
// read whole before either is answered.
func TestExtractReadsAPropertySetBelow(t *testing.T) {
	check(t, "<project>\n  <dependencies>\n    <dependency>\n      <groupId>g</groupId>\n      <artifactId>a</artifactId>\n"+
		"      <version>${v}</version>\n    </dependency>\n  </dependencies>\n  <properties>\n    <v>1.2.3</v>\n  </properties>\n</project>\n",
		want{"g/a", "1.2.3", 6})
}

// Markup that does not parse sets the POM aside whole, and the dependencies
// read before the decoder stopped go with it.
func TestExtractSetsAsideAFileItCannotParse(t *testing.T) {
	whole := "<project>\n  <dependencies>\n    <dependency>\n      <groupId>g</groupId>\n      <artifactId>a</artifactId>\n      <version>1.0</version>\n    </dependency>\n"
	for _, tt := range []struct{ name, body, skipped string }{
		{"markup that does not parse", "<project><dependencies><dependency></project>", decl.NotXML},
		{"a dependency the file never closes", "<project>\n  <dependencies>\n    <dependency>\n      <groupId>g<X></groupId>\n", decl.NotXML},
		// The first dependency is whole and the file is not, and both go.
		{"cut short after a whole dependency", whole, decl.NotXML},
		{"a POM declaring nothing", "<project>\n  <modelVersion>4.0.0</modelVersion>\n</project>\n", ""},
		{"empty", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ds, us, skipped := Extract("pom.xml", []byte(tt.body))
			if skipped != tt.skipped {
				t.Errorf("skipped = %q; want %q", skipped, tt.skipped)
			}
			if len(ds) != 0 || len(us) != 0 {
				t.Errorf("Extract = %+v, %+v; want nothing", ds, us)
			}
		})
	}
}
