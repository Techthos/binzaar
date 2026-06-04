package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Modern-dark palette (Tokyo Night-inspired). The hex* values feed the tcell
// theme; the tag* values are the same colors as inline color-tag markup for
// widgets that render dynamic colors (TextViews and Table cells).
const (
	hexBg     = "#1a1b26" // window background
	hexBgAlt  = "#24283b" // inputs, buttons, selected rows
	hexBorder = "#3b4261" // unfocused borders / graphics
	hexText   = "#c0caf5" // primary text
	hexDim    = "#565f89" // muted / secondary text
	hexAccent = "#7aa2f7" // primary accent (blue)

	tagAccent  = "#7aa2f7" // blue
	tagAccent2 = "#bb9af7" // purple
	tagText    = "#c0caf5" // light
	tagDim     = "#565f89" // muted
	tagGood    = "#9ece6a" // green
	tagBad     = "#f7768e" // red
	tagWarn    = "#e0af68" // amber
)

// key markup brackets: open colors+bolds a keybinding glyph, close fully resets.
const (
	keyOpen  = "[" + tagAccent + "::b]"
	keyClose = "[-:-:-]"
)

// init applies the global tview theme and rounded borders exactly once, at
// package load — before any Application or widget exists (per the tui rules)
// and before any headless test starts an event loop, so it never races a Draw.
func init() {
	bg := tcell.GetColor(hexBg)
	tview.Styles = tview.Theme{
		PrimitiveBackgroundColor:    bg,
		ContrastBackgroundColor:     tcell.GetColor(hexBgAlt),
		MoreContrastBackgroundColor: tcell.GetColor(hexAccent),
		BorderColor:                 tcell.GetColor(hexBorder),
		TitleColor:                  tcell.GetColor(hexAccent),
		GraphicsColor:               tcell.GetColor(hexBorder),
		PrimaryTextColor:            tcell.GetColor(hexText),
		SecondaryTextColor:          tcell.GetColor(hexAccent),
		TertiaryTextColor:           tcell.GetColor(hexDim),
		InverseTextColor:            bg,
		ContrastSecondaryTextColor:  tcell.GetColor(hexDim),
	}

	// Rounded corners for the resting (unfocused) border; the focused border
	// keeps tview's default heavier runes, which doubles as the focus cue.
	tview.Borders.TopLeft = '╭'
	tview.Borders.TopRight = '╮'
	tview.Borders.BottomLeft = '╰'
	tview.Borders.BottomRight = '╯'
}

// selectedStyle is the highlight for the selected row in a Table: dark text on
// the accent color, bold — readable and on-palette.
func selectedStyle() tcell.Style {
	return tcell.StyleDefault.
		Background(tcell.GetColor(hexAccent)).
		Foreground(tcell.GetColor(hexBg)).
		Bold(true)
}
