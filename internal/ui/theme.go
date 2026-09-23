package ui

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme is a named color palette. Colors are hex ("#rrggbb"), ANSI indices
// ("0"-"255") or "" for the terminal default.
type Theme struct {
	Name    string `json:"name"`
	Extends string `json:"extends,omitempty"` // custom themes: base theme name
	Dark    bool   `json:"dark"`

	Text       string `json:"text"`        // editor text
	Muted      string `json:"muted"`       // secondary text
	Bar        string `json:"bar"`         // tab bar and status bar background
	Tab        string `json:"tab"`         // inactive tab background
	TabText    string `json:"tab_text"`    // inactive tab text
	Accent     string `json:"accent"`      // active tab, highlights
	AccentText string `json:"accent_text"` // text on accent
	Selection  string `json:"selection"`   // selected text background
	Guide      string `json:"guide"`       // indentation guides
	Surface    string `json:"surface"`     // dialog background
	Danger     string `json:"danger"`      // destructive actions
	Ok         string `json:"ok"`          // saved indicator
}

// AutoTheme follows the terminal: dark or light depending on its background.
const AutoTheme = "auto"

var builtinThemes = []Theme{
	{
		Name: "catppuccin-mocha", Dark: true,
		Text: "#cdd6f4", Muted: "#7f849c", Bar: "#181825", Tab: "#313244", TabText: "#a6adc8",
		Accent: "#cba6f7", AccentText: "#1e1e2e", Selection: "#45475a", Guide: "#313244",
		Surface: "#1e1e2e", Danger: "#f38ba8", Ok: "#a6e3a1",
	},
	{
		Name: "catppuccin-latte", Dark: false,
		Text: "#4c4f69", Muted: "#8c8fa1", Bar: "#e6e9ef", Tab: "#ccd0da", TabText: "#5c5f77",
		Accent: "#8839ef", AccentText: "#eff1f5", Selection: "#bcc0cc", Guide: "#ccd0da",
		Surface: "#eff1f5", Danger: "#d20f39", Ok: "#40a02b",
	},
	{
		Name: "nord", Dark: true,
		Text: "#d8dee9", Muted: "#6d7a96", Bar: "#2e3440", Tab: "#3b4252", TabText: "#a3adc2",
		Accent: "#88c0d0", AccentText: "#2e3440", Selection: "#434c5e", Guide: "#3b4252",
		Surface: "#3b4252", Danger: "#bf616a", Ok: "#a3be8c",
	},
	{
		Name: "gruvbox-dark", Dark: true,
		Text: "#ebdbb2", Muted: "#928374", Bar: "#1d2021", Tab: "#3c3836", TabText: "#bdae93",
		Accent: "#fabd2f", AccentText: "#282828", Selection: "#504945", Guide: "#3c3836",
		Surface: "#282828", Danger: "#fb4934", Ok: "#b8bb26",
	},
	{
		Name: "tokyo-night", Dark: true,
		Text: "#c0caf5", Muted: "#565f89", Bar: "#16161e", Tab: "#292e42", TabText: "#a9b1d6",
		Accent: "#7aa2f7", AccentText: "#1a1b26", Selection: "#33467c", Guide: "#292e42",
		Surface: "#1a1b26", Danger: "#f7768e", Ok: "#9ece6a",
	},
	{
		Name: "rose-pine-dawn", Dark: false,
		Text: "#575279", Muted: "#9893a5", Bar: "#fffaf3", Tab: "#f2e9e1", TabText: "#797593",
		Accent: "#d7827e", AccentText: "#faf4ed", Selection: "#dfdad9", Guide: "#f2e9e1",
		Surface: "#faf4ed", Danger: "#b4637a", Ok: "#56949f",
	},
	{
		// Uses the terminal's own 16-color palette, so it follows whatever
		// scheme the terminal is set to.
		Name: "terminal", Dark: true,
		Text: "", Muted: "8", Bar: "", Tab: "0", TabText: "7",
		Accent: "4", AccentText: "15", Selection: "8", Guide: "8",
		Surface: "", Danger: "1", Ok: "2",
	},
}

// themeSet is the list of themes the user can cycle through.
type themeSet struct {
	themes []Theme
}

// loadThemes returns the built-ins plus custom themes from dir/themes/*.json.
// A broken theme file is skipped and reported, never fatal.
func loadThemes(configDir string) (themeSet, []string) {
	set := themeSet{themes: append([]Theme(nil), builtinThemes...)}
	var problems []string
	files, _ := filepath.Glob(filepath.Join(configDir, "themes", "*.json"))
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			problems = append(problems, filepath.Base(f)+": "+err.Error())
			continue
		}
		var t Theme
		if err := json.Unmarshal(data, &t); err != nil {
			problems = append(problems, filepath.Base(f)+": "+err.Error())
			continue
		}
		if t.Name == "" {
			t.Name = strings.TrimSuffix(filepath.Base(f), ".json")
		}
		base := "catppuccin-mocha"
		if t.Extends != "" {
			base = t.Extends
		} else if !t.Dark {
			base = "catppuccin-latte"
		}
		if b, ok := set.find(base); ok {
			t = mergeTheme(b, t)
		}
		set.add(t)
	}
	return set, problems
}

// mergeTheme fills unset fields of t from base.
func mergeTheme(base, t Theme) Theme {
	out := base
	out.Name, out.Extends = t.Name, t.Extends
	if t.Extends == "" {
		out.Dark = t.Dark
	}
	for dst, src := range map[*string]string{
		&out.Text: t.Text, &out.Muted: t.Muted, &out.Bar: t.Bar, &out.Tab: t.Tab,
		&out.TabText: t.TabText, &out.Accent: t.Accent, &out.AccentText: t.AccentText,
		&out.Selection: t.Selection, &out.Guide: t.Guide, &out.Surface: t.Surface,
		&out.Danger: t.Danger, &out.Ok: t.Ok,
	} {
		if src != "" {
			*dst = src
		}
	}
	return out
}

func (s *themeSet) add(t Theme) {
	for i := range s.themes {
		if s.themes[i].Name == t.Name {
			s.themes[i] = t
			return
		}
	}
	s.themes = append(s.themes, t)
}

func (s themeSet) find(name string) (Theme, bool) {
	for _, t := range s.themes {
		if t.Name == name {
			return t, true
		}
	}
	return Theme{}, false
}

// names lists "auto" followed by every theme, in cycling order.
func (s themeSet) names() []string {
	n := []string{AutoTheme}
	for _, t := range s.themes {
		n = append(n, t.Name)
	}
	return n
}

// resolve picks the theme for a configured name. "auto" and unknown names
// follow the terminal background.
func (s themeSet) resolve(name string, darkBg bool) Theme {
	if t, ok := s.find(name); ok && name != AutoTheme {
		return t
	}
	if darkBg {
		t, _ := s.find("catppuccin-mocha")
		return t
	}
	t, _ := s.find("catppuccin-latte")
	return t
}

// next returns the theme name after current in cycling order.
func (s themeSet) next(current string, step int) string {
	names := s.names()
	i := 0
	for j, n := range names {
		if n == current {
			i = j
		}
	}
	return names[((i+step)%len(names)+len(names))%len(names)]
}

func col(s string) color.Color {
	if s == "" {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(s)
}

// styles are the lipgloss styles derived from a theme.
type styles struct {
	theme Theme

	bar, tab, tabClose, activeTab, activeClose, tabArrow, newTab lipgloss.Style
	text, selection, guide                                       lipgloss.Style
	status, statusMuted, statusOk, statusBusy, statusErr         lipgloss.Style
	statusKey                                                    lipgloss.Style
	dialog, dialogTitle, dialogText, dialogMuted                 lipgloss.Style
	button, buttonFocus, dangerFocus                             lipgloss.Style
	input                                                        lipgloss.Style
	hintKey, hintLabel                                           lipgloss.Style
	placeholder                                                  lipgloss.Style
}

func newStyles(t Theme) styles {
	bar := lipgloss.NewStyle().Background(col(t.Bar))
	surface := lipgloss.NewStyle().Background(col(t.Surface))
	return styles{
		theme:       t,
		bar:         bar,
		tab:         lipgloss.NewStyle().Background(col(t.Tab)).Foreground(col(t.TabText)),
		tabClose:    lipgloss.NewStyle().Background(col(t.Tab)).Foreground(col(t.Muted)),
		activeTab:   lipgloss.NewStyle().Background(col(t.Accent)).Foreground(col(t.AccentText)).Bold(true),
		activeClose: lipgloss.NewStyle().Background(col(t.Accent)).Foreground(col(t.AccentText)),
		tabArrow:    bar.Foreground(col(t.Accent)).Bold(true),
		newTab:      bar.Foreground(col(t.Accent)).Bold(true),

		text:      lipgloss.NewStyle().Foreground(col(t.Text)),
		selection: lipgloss.NewStyle().Background(col(t.Selection)).Foreground(col(t.Text)),
		guide:     lipgloss.NewStyle().Foreground(col(t.Guide)),

		status:      bar.Foreground(col(t.TabText)),
		statusMuted: bar.Foreground(col(t.Muted)),
		statusOk:    bar.Foreground(col(t.Ok)),
		statusBusy:  bar.Foreground(col(t.Accent)),
		statusErr:   bar.Foreground(col(t.Danger)).Bold(true),
		statusKey:   bar.Foreground(col(t.Accent)),

		dialog: surface.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(col(t.Accent)).
			BorderBackground(col(t.Surface)).
			Padding(1, 3),
		dialogTitle: surface.Foreground(col(t.Text)).Bold(true),
		dialogText:  surface.Foreground(col(t.Text)),
		dialogMuted: surface.Foreground(col(t.Muted)),
		button:      lipgloss.NewStyle().Background(col(t.Tab)).Foreground(col(t.TabText)).Padding(0, 2),
		buttonFocus: lipgloss.NewStyle().Background(col(t.Accent)).Foreground(col(t.AccentText)).Bold(true).Padding(0, 2),
		dangerFocus: lipgloss.NewStyle().Background(col(t.Danger)).Foreground(col(t.AccentText)).Bold(true).Padding(0, 2),
		hintKey:     lipgloss.NewStyle().Background(col(t.Tab)).Foreground(col(t.Accent)).Bold(true),
		hintLabel:   lipgloss.NewStyle().Foreground(col(t.Muted)),
		placeholder: lipgloss.NewStyle().Foreground(col(t.Muted)).Italic(true),
		input: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(col(t.Accent)).
			Padding(0, 1),
	}
}
