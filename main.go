// cool-notes is a tabbed terminal note editor that keeps every note in one
// plain-text file and saves as you type.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/thinkbig1979/cool-notes/internal/config"
	"github.com/thinkbig1979/cool-notes/internal/ui"
)

func main() {
	file := flag.String("file", "", "notes file to open (overrides the configured one)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: cool-notes [--file path]\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nSettings live in $XDG_CONFIG_HOME/cool-notes (override with COOL_NOTES_CONFIG_DIR).\n")
	}
	flag.Parse()
	if err := run(*file); err != nil {
		fmt.Fprintln(os.Stderr, "cool-notes:", err)
		os.Exit(1)
	}
}

func run(fileFlag string) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	opts := ui.Options{ConfigDir: dir, Config: cfg}
	if fileFlag != "" {
		if opts.FileFlag, err = config.ExpandPath(fileFlag); err != nil {
			return err
		}
	}
	m, err := ui.New(opts)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m)
	// Closing the terminal sends SIGHUP; quit cleanly so the final flush runs.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		<-sigs
		p.Quit()
	}()
	final, runErr := p.Run()
	// Belt and braces: whatever ended the program, write the latest text.
	if fm, ok := final.(*ui.Model); ok {
		if err := fm.Flush(); err != nil {
			return fmt.Errorf("saving notes: %w", err)
		}
	} else if err := m.Flush(); err != nil {
		return fmt.Errorf("saving notes: %w", err)
	}
	return runErr
}
