// Command icon regenerates the app icon using the same native renderer as the UI.
package main

import (
	"image/png"
	"log"
	"os"

	"github.com/egoist/mygo/ui"
)

func main() {
	if err := os.MkdirAll("assets", 0755); err != nil {
		log.Fatal(err)
	}
	svg := ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4 17v3h16v-3M12 3v12m-5-5 5 5 5-5"/></svg>`))
	im := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Radius(220).Background(ui.Hex("#356AE6")).Center().Children(func() { ui.Icon(c, svg).Size(650, 650).TextColor(ui.Hex("#FFFFFF")) })
	}, 1024, 1024, 1)
	f, err := os.Create("assets/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, im); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
