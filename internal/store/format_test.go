package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	cases := [][]string{
		{""},
		{"", ""},
		{"a"},
		{"a\n"},
		{"a\n\n", "b"},
		{"Shopping\n\tmilk\n\teggs", "Ideas\n\t- one\n\t\t- nested", ""},
		{"=============== note: fake ===============", "x"},
		{`\=== note ===`, `\\=== note: t ===`},
		{"=== note ===   ", "== note =="},
		{"\n\nleading blanks", "trailing blanks\n\n\n"},
		{"unicode ✓ 日本語\ttab"},
	}
	for _, notes := range cases {
		got := Parse(Serialize(notes))
		if !reflect.DeepEqual(got, notes) {
			t.Errorf("round trip\n in: %q\nout: %q\nfile:\n%s", notes, got, Serialize(notes))
		}
	}
}

func TestParseEmpty(t *testing.T) {
	for _, in := range []string{"", "\n", "   \n\n", "\ufeff"} {
		if got := Parse(in); len(got) != 0 {
			t.Errorf("Parse(%q) = %q, want no notes", in, got)
		}
	}
}

func TestSerializeLooksRight(t *testing.T) {
	got := Serialize([]string{"Shopping list\n\tmilk", ""})
	want := "=============== note: Shopping list ===============\n" +
		"Shopping list\n\tmilk\n" +
		"\n" +
		"=============== note ===============\n" +
		"\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMalformedSeparatorIsText(t *testing.T) {
	file := strings.Join([]string{
		"=== note: A ===",
		"body a",
		"== note: broken ===", // too few '='
		"=== nota ===",        // wrong word
		"=== note: x ==",      // too few trailing '='
		"===note===",          // missing spaces
		" === note ===",       // leading space
		"text",
		"",
		"=== NOTE: B ===", // case-insensitive
		"body b",
	}, "\n")
	got := Parse(file)
	want := []string{
		"body a\n== note: broken ===\n=== nota ===\n=== note: x ==\n===note===\n === note ===\ntext",
		"body b",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestHandEditedFiles(t *testing.T) {
	cases := map[string]struct {
		in   string
		want []string
	}{
		"no separators at all": {"just some text\nmore", []string{"just some text\nmore"}},
		"text before first":    {"preamble\n=== note ===\nbody", []string{"preamble", "body"}},
		"blank before first":   {"\n\n=== note ===\nbody", []string{"body"}},
		"crlf line endings":    {"=== note: a ===\r\nx\r\n\r\n=== note ===\r\ny\r\n", []string{"x", "y"}},
		"no spacer line":       {"=== note ===\nx\n=== note ===\ny", []string{"x", "y"}},
		"trailing whitespace":  {"=== note ===  \t\nx", []string{"x"}},
		"no trailing newline":  {"=== note ===\nx", []string{"x"}},
		"stale title ignored":  {"=== note: old title ===\nnew title", []string{"new title"}},
		"adjacent separators":  {"=== note ===\n=== note ===\nx", []string{"", "x"}},
	}
	for name, c := range cases {
		if got := Parse(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestTitle(t *testing.T) {
	cases := map[string]string{
		"":                           "",
		"\n\n  \t\n":                 "",
		"\n\tHello   world  \nnext":  "Hello world",
		strings.Repeat("abcde ", 20): "abcde abcde abcde abcde abcde abcde a...",
	}
	for in, want := range cases {
		if got := Title(in); got != want {
			t.Errorf("Title(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFileSaveLoadAndExternal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "notes.txt")
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := f.Load()
	if err != nil || len(notes) != 0 {
		t.Fatalf("Load missing file = %q, %v", notes, err)
	}
	if err := f.Save(1, []string{"one", "two"}); err != nil {
		t.Fatal(err)
	}
	if ext, err := f.CheckExternal(); err != nil || ext != nil {
		t.Fatalf("own write reported as external: %v %v", ext, err)
	}
	// Stale save is skipped.
	if err := f.Save(2, []string{"new"}); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(1, []string{"stale"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Load(); !reflect.DeepEqual(got, []string{"new"}) {
		t.Fatalf("after stale save got %q", got)
	}

	// Another editor replaces the file.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("=== note ===\nedited elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ext, err := f.CheckExternal()
	if err != nil || ext == nil || !reflect.DeepEqual(ext.Notes, []string{"edited elsewhere"}) {
		t.Fatalf("external change = %+v, %v", ext, err)
	}
	if again, _ := f.CheckExternal(); again != nil {
		t.Fatal("external change reported twice")
	}

	name, err := f.Backup(ext.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(name); string(data) != string(ext.Raw) {
		t.Fatalf("backup content = %q", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}
