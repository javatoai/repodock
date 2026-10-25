package desktop

import "github.com/egoist/mygo/ui"

type palette struct {
	background, surface, soft, line, text, muted, accent, accentSoft, success, danger ui.Color
}

func applyTheme(c *ui.Context, preference string) palette {
	dark := preference == "dark" || (preference == "system" && c.Theme().Dark)
	p := palette{
		ui.Hex("#F4F7FB"), ui.Hex("#FFFFFF"), ui.Hex("#F7F9FC"), ui.Hex("#E4EAF2"), ui.Hex("#182338"),
		ui.Hex("#626E80"), ui.Hex("#356AE6"), ui.Hex("#EBF1FF"), ui.Hex("#22805A"), ui.Hex("#CF4557"),
	}
	t := ui.LightTheme()
	if dark {
		p = palette{
			ui.Hex("#0D1421"), ui.Hex("#151E2D"), ui.Hex("#1B2739"), ui.Hex("#28354A"), ui.Hex("#E8EDF5"),
			ui.Hex("#98A8BE"), ui.Hex("#82A9FF"), ui.Hex("#223657"), ui.Hex("#74D5AA"), ui.Hex("#FF94A0"),
		}
		t = ui.DarkTheme()
	}
	t.Background, t.Surface = p.background, p.surface
	t.SurfaceHover, t.SurfacePressed = p.soft, p.accentSoft
	t.Text, t.TextMuted, t.Border = p.text, p.muted, p.line
	t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#356AE6"), ui.Hex("#285CD3"), ui.Hex("#2454C6")
	t.AccentText, t.Danger = ui.Hex("#FFFFFF"), p.danger
	t.FontSize, t.Radius, t.Spacing = 14, 9, 4
	c.SetTheme(t)
	return p
}

var iconPaths = map[string]string{
	"home":     `<path d="m3 10 9-7 9 7v11h-6v-7H9v7H3z"/>`,
	"search":   `<circle cx="10" cy="10" r="6"/><path d="m15 15 6 6"/>`,
	"more":     `<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`,
	"dock":     `<path d="M4 17v3h16v-3M12 3v12m-5-5 5 5 5-5"/>`,
	"repo":     `<rect x="4" y="3" width="16" height="18" rx="3"/><path d="M9 3v18m4-13h3m-3 4h3"/>`,
	"github":   `<path d="M9 19c-4 1-4-2-6-2m12 5v-4c0-1-.3-2-1-2 4-.5 7-2 7-6a5 5 0 0 0-1-3 5 5 0 0 0 0-4s-1-1-4 2a14 14 0 0 0-8 0C5 2 4 3 4 3a5 5 0 0 0 0 4 5 5 0 0 0-1 3c0 4 3 5.5 7 6-.7 0-1 1-1 2v4"/>`,
	"settings": `<path d="m9 3-1 3-3 1-2 5 2 5 3 1 1 3h6l1-3 3-1 2-5-2-5-3-1-1-3z"/><circle cx="12" cy="12" r="3"/>`,
	"chevron":  `<path d="m7 10 5 5 5-5"/>`,
	"arrow":    `<path d="M5 12h14m-5-5 5 5-5 5"/>`,
	"launch":   `<path d="M14 3h7v7m0-7L11 13M9 4H4v17h17v-5"/>`,
	"folder":   `<path d="M3 7V4h6l3 3h9v13H3z"/>`,
	"close":    `<path d="m6 6 12 12M18 6 6 18"/>`,
	"check":    `<path d="m5 12 4 4L19 6"/>`,
	"refresh":  `<path d="M20 8a8 8 0 1 0 0 8m0-13v5h-5"/>`,
	"lock":     `<rect x="5" y="10" width="14" height="11" rx="3"/><path d="M8 10V7a4 4 0 0 1 8 0v3m-4 5v2"/>`,
	"clock":    `<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`,
	"trash":    `<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15m-9 4v7m4-7v7"/>`,
	"sun":      `<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1 1m12 12 1 1M5 19l1-1M18 6l1-1"/>`,
	"copy":     `<rect x="8" y="8" width="13" height="13" rx="2"/><path d="M16 8V3H3v13h5"/>`,
	"warning":  `<path d="m12 3 10 18H2zM12 9v5m0 3v1"/>`,
}

var icons = func() map[string]*ui.SVG {
	m := make(map[string]*ui.SVG)
	for name, path := range iconPaths {
		m[name] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`))
	}
	return m
}()

func icon(c *ui.Context, name string, size float32, color ui.Color) {
	ui.Icon(c, icons[name]).Size(size, size).Shrink(0).TextColor(color)
}

func iconButton(c *ui.Context, key, name, label string, p palette, disabled bool) bool {
	b := ui.ButtonBase(c.Key(key)).Size(32, 32).Radius(8).Center().Label(label).Tooltip(label).Disabled(disabled)
	color := p.muted
	if b.Hovered() {
		b.Background(p.soft)
		color = p.text
	}
	b.Children(func() { icon(c, name, 17, color) })
	return b.Clicked()
}

func button(c *ui.Context, key, name, label string, p palette, primary, disabled bool) bool {
	b := ui.ButtonBase(c.Key(key)).Row().Height(42).Padding(0, 18).Gap(9).AlignItems(ui.Center).Justify(ui.Center).Radius(10).Label(label).Disabled(disabled)
	fg, bg := p.text, p.soft
	if primary {
		fg, bg = ui.Hex("#FFFFFF"), ui.Hex("#356AE6")
		if b.Hovered() {
			bg = ui.Hex("#285CD3")
		}
	} else if b.Hovered() {
		bg = p.accentSoft
	}
	if disabled {
		fg, bg = p.muted, p.soft
	}
	b.Background(bg).Children(func() {
		if name != "" {
			icon(c, name, 17, fg)
		}
		ui.Text(c, label).FontSize(13).TextColor(fg).Bold().NoWrap()
	})
	return b.Clicked()
}
