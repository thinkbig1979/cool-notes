// Package store reads and writes the single notes file.
//
// File format: every note starts with a separator line, followed by its body.
// Notes are separated by one blank line for readability.
//
//	=============== note: Shopping list ===============
//	Shopping list
//		milk
//
//	=============== note: Project ideas ===============
//	Project ideas
//
// A line is a separator only if, ignoring trailing whitespace, it is three or
// more '=', a space, the word "note", an optional ": title", a space, and three
// or more '='. The title is cosmetic: it is regenerated from the note's first
// line on every save and ignored when parsing. Anything that does not match
// exactly, such as a damaged separator, is ordinary text in the note above it.
//
// A body line that would itself parse as a separator is written with a
// leading backslash and restored on read, so note text can never split a note.
package store

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var separatorRe = regexp.MustCompile(`^={3,} (?i:note)(?:: .*)? ={3,}$`)

const (
	ruleWidth     = 15
	titleMaxRunes = 40
)

// IsSeparator reports whether line is a note separator.
func IsSeparator(line string) bool {
	return separatorRe.MatchString(strings.TrimRight(line, " \t\r"))
}

// isEscaped reports whether line is a separator protected by one or more
// leading backslashes.
func isEscaped(line string) bool {
	trimmed := strings.TrimLeft(line, `\`)
	return len(trimmed) < len(line) && IsSeparator(trimmed)
}

// Title derives a display title from a note body: its first non-blank line,
// with whitespace collapsed and long titles shortened.
func Title(body string) string {
	for line := range strings.SplitSeq(body, "\n") {
		t := strings.Join(strings.Fields(line), " ")
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > titleMaxRunes {
			r := []rune(t)
			t = strings.TrimRight(string(r[:titleMaxRunes-3]), " ") + "..."
		}
		return t
	}
	return ""
}

// Separator builds the separator line for a note body.
func Separator(body string) string {
	rule := strings.Repeat("=", ruleWidth)
	if t := Title(body); t != "" {
		return rule + " note: " + t + " " + rule
	}
	return rule + " note " + rule
}

// Parse splits file contents into note bodies.
func Parse(data string) []string {
	data = strings.TrimPrefix(data, "\ufeff")
	data = strings.ReplaceAll(data, "\r\n", "\n")
	if data == "" {
		return nil
	}
	data = strings.TrimSuffix(data, "\n")

	var segments [][]string
	var cur []string
	seenSeparator := false
	for line := range strings.SplitSeq(data, "\n") {
		switch {
		case IsSeparator(line):
			if seenSeparator || strings.TrimSpace(strings.Join(cur, "")) != "" {
				segments = append(segments, cur)
			}
			cur = nil
			seenSeparator = true
		case isEscaped(line):
			cur = append(cur, line[1:])
		default:
			cur = append(cur, line)
		}
	}
	if seenSeparator || strings.TrimSpace(strings.Join(cur, "")) != "" {
		segments = append(segments, cur)
	}

	notes := make([]string, len(segments))
	for i, seg := range segments {
		// Drop the blank spacer line written between notes.
		if i < len(segments)-1 && len(seg) > 0 && seg[len(seg)-1] == "" {
			seg = seg[:len(seg)-1]
		}
		notes[i] = strings.Join(seg, "\n")
	}
	return notes
}

// Serialize renders note bodies into file contents. Parse(Serialize(n))
// returns n unchanged.
func Serialize(notes []string) string {
	var b strings.Builder
	for i, body := range notes {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(Separator(body))
		b.WriteByte('\n')
		for j, line := range strings.Split(body, "\n") {
			if j > 0 {
				b.WriteByte('\n')
			}
			if IsSeparator(line) || isEscaped(line) {
				b.WriteByte('\\')
			}
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
