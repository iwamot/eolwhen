package dockerfile

import (
	"regexp"
	"strings"
)

// instruction is one Dockerfile instruction: the text of every line it
// spans, joined, and the line the first of them sits on. The line reported
// is where the instruction starts, which is where a reader looks for it.
type instruction struct {
	text string
	line int
}

// heredocDirectives are the instructions whose arguments may open a
// heredoc. The body that follows is the file being written or the script
// being run, so a FROM among those lines is a string rather than a base
// image. Only these three take one, and ONBUILD carries one of them.
var heredocDirectives = map[string]bool{"ADD": true, "COPY": true, "RUN": true}

// reHeredoc matches the word that opens a heredoc: an optional file
// descriptor, the <<, an optional dash asking for leading tabs to be
// stripped from the body, and the word that ends it.
var reHeredoc = regexp.MustCompile(`^\d*<<(-?)\s*([^<]*)$`)

// parserDirectives are the names a comment above the first instruction may
// carry. A comment that is not one of them ends the directives, and the
// escape character is whatever it was until then.
var parserDirectives = map[string]bool{"check": true, "escape": true, "syntax": true}

// instructions splits a Dockerfile into the instructions it holds. Two
// things keep an instruction from being a single line of text: it may be
// continued onto the next line with a trailing escape character, and a RUN,
// COPY or ADD may open a heredoc whose body belongs to it rather than
// standing on its own.
func instructions(data []byte) []instruction {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	esc := escape(lines)
	var out []instruction
	for i := 0; i < len(lines); i++ {
		if blank(lines[i]) {
			continue
		}
		start := i
		text, continues := trimEscape(strings.TrimSpace(lines[i]), esc)
		for continues && i+1 < len(lines) {
			i++
			// A comment or an empty line inside a continuation is dropped
			// and the instruction goes on, so neither ends it.
			if blank(lines[i]) {
				continue
			}
			var part string
			// Joined without anything in between: an author who broke a
			// word across two lines wrote the space themselves if they
			// wanted one.
			part, continues = trimEscape(lines[i], esc)
			text += part
		}
		out = append(out, instruction{text: text, line: start + 1})
		i = skipHeredocs(lines, i, text)
	}
	return out
}

// escape is the character that continues a line onto the next. A Dockerfile
// may change it to a backtick with a parser directive at the top, which
// Windows builds do so that paths do not read as escapes.
func escape(lines []string) byte {
	for _, line := range lines {
		name, value, ok := directive(line)
		if !ok {
			break
		}
		if name == "escape" && value == "`" {
			return '`'
		}
	}
	return '\\'
}

// directive splits a parser directive — a # name=value comment above the
// first instruction — into its parts, and reports whether the line is one.
func directive(line string) (name, value string, ok bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "#")
	if !ok {
		return "", "", false
	}
	name, value, ok = strings.Cut(rest, "=")
	if !ok {
		return "", "", false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if !parserDirectives[name] {
		return "", "", false
	}
	return name, strings.TrimSpace(value), true
}

// blank reports whether a line carries no instruction text: empty, or a
// comment.
func blank(line string) bool {
	line = strings.TrimSpace(line)
	return line == "" || strings.HasPrefix(line, "#")
}

// trimEscape cuts the escape character that continues a line onto the next,
// and reports whether there was one. Two in a row are an escaped escape
// rather than a continuation.
func trimEscape(line string, esc byte) (string, bool) {
	trimmed := strings.TrimRight(line, " \t")
	rest, ok := strings.CutSuffix(trimmed, string(esc))
	if !ok || strings.HasSuffix(rest, string(esc)) {
		return line, false
	}
	return rest, true
}

// skipHeredocs moves past the bodies of the heredocs an instruction opened.
// i is the instruction's last line, and the returned index is the last line
// its heredocs take up, so the next instruction starts after all of them.
// A body with no terminator runs to the end of the file, which is what the
// builder makes of it too.
func skipHeredocs(lines []string, i int, text string) int {
	fields := strings.Fields(text)
	// ONBUILD holds another instruction, heredoc and all.
	if len(fields) > 0 && strings.EqualFold(fields[0], "ONBUILD") {
		fields = fields[1:]
	}
	if len(fields) == 0 || !heredocDirectives[strings.ToUpper(fields[0])] {
		return i
	}
	for _, field := range fields[1:] {
		m := reHeredoc.FindStringSubmatch(field)
		if m == nil {
			continue
		}
		// Quotes around the word say the body is not to be expanded; the
		// word that ends it is the same either way.
		name := strings.Trim(m[2], `"'`)
		if name == "" {
			continue
		}
		chomp := m[1] == "-"
		for i+1 < len(lines) {
			i++
			end := lines[i]
			if chomp {
				end = strings.TrimLeft(end, "\t")
			}
			if end == name {
				break
			}
		}
	}
	return i
}
