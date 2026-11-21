// Command icon draws the app icons: the layers of AppIcon.icon, the Icon
// Composer document that macOS 26 and later render in Liquid Glass, and
// resources/icon.png, the same icon as macOS draws it, full-bleed for older
// macOS and Windows; and from AppIcon-dev.icon, the same picture inverted,
// resources/icon-dev.png, the icon of mygo dev builds.
//
// Run it from the repository root on a Mac with Xcode 26 or later:
//
//	go run ./icon
//
// The icon.json of each document, its background, glass and dark-mode
// colors, is edited by hand or in Icon Composer.
package main

import (
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ictool = "/Applications/Xcode.app/Contents/Applications/Icon Composer.app/Contents/Executables/ictool"

// palette holds the colors of a document's record, label and tonearm.
// The app's is a vinyl record on a warm amber-to-dark background, with an
// orange-red label and chrome tonearm. The development app inverts the
// palette: cream record, dark-red label on a cool-dark background.
type palette struct {
	discTop, discMid, discEnd string
	groove                    string
	groovePeak                float64
	rim                       string
	rimOpacity                string
	labelTop, labelBottom     string
	labelRing                 string
	spindle                   string
	armTop, armBottom, armCap string
	bgStart, bgEnd            string
}

var appPalette = palette{
	discTop: "#1c1c1c", discMid: "#0e0e0e", discEnd: "#080808",
	groove: "#ffffff", groovePeak: 0.05,
	rim: "#ffffff", rimOpacity: "0.10",
	labelTop: "#ff7040", labelBottom: "#6e1008",
	labelRing: "#000000", spindle: "#0e0e0e",
	armTop: "#f0eeee", armBottom: "#303030", armCap: "#c8c4c8",
	bgStart: "#f5dfa0", bgEnd: "#2a0e06",
}

var devPalette = palette{
	discTop: "#2a2834", discMid: "#18161e", discEnd: "#0c0a10",
	groove: "#ffffff", groovePeak: 0.05,
	rim: "#ffffff", rimOpacity: "0.10",
	labelTop: "#a040ff", labelBottom: "#3a0870",
	labelRing: "#ffffff", spindle: "#1c1c2c",
	armTop: "#e8e4f0", armBottom: "#282030", armCap: "#c0bcd0",
	bgStart: "#c8b8f0", bgEnd: "#120820",
}

// icons are the documents drawn, with the PNG rendered from each and the
// palette of its record.
var icons = []struct {
	document, output string
	p                palette
}{
	{"icon/AppIcon.icon", "resources/icon.png", appPalette},
	{"icon/AppIcon-dev.icon", "resources/icon-dev.png", devPalette},
}

// A record on Icon Composer's 1024-point full-bleed canvas, seen close up:
// larger than the canvas, it runs off the left and bottom edges. And the
// tonearm over it, from its pivot to the headshell resting on the grooves.
var (
	rec   = struct{ cx, cy, r float64 }{330, 712, 560}
	pivot = image.Pt(820, 214)
	head  = image.Pt(585, 520)
)

func main() {
	// All the layers first: a Mac renders the PNGs, and elsewhere the SVGs
	// are still written for review before ictool stops the run.
	for _, ic := range icons {
		layers := map[string]string{
			"arm.svg":    armSVG(ic.p.armTop, ic.p.armBottom, ic.p.armCap),
			"record.svg": recordSVG(ic.p),
			"label.svg":  labelSVG(ic.p),
		}
		for name, s := range layers {
			if err := os.WriteFile(filepath.Join(ic.document, "Assets", name), []byte(s), 0o644); err != nil {
				log.Fatal(err)
			}
		}
	}
	for _, ic := range icons {
		if err := render(ic.document, ic.output); err != nil {
			log.Fatal(err)
		}
	}
}

func svg(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">` + "\n" + body + "\n</svg>\n"
}

func n(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func armSVG(top, bottom, cap string) string {
	p := pivot
	length := math.Hypot(float64(head.X-p.X), float64(head.Y-p.Y))
	angle := math.Atan2(float64(head.Y-p.Y), float64(head.X-p.X)) * 180 / math.Pi
	end := float64(p.X) + length - 30
	return svg(fmt.Sprintf(`<defs>
<linearGradient id="m" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="%[11]s"/><stop offset="1" stop-color="%[12]s"/></linearGradient>
</defs>
<g transform="rotate(%[3]s %[1]d %[2]d)">
<rect x="%[4]d" y="%[5]d" width="64" height="52" rx="15" fill="url(#m)"/>
<rect x="%[1]d" y="%[6]d" width="%[7]s" height="34" rx="17" fill="url(#m)"/>
<rect x="%[8]s" y="%[9]d" width="92" height="56" rx="14" fill="url(#m)" transform="rotate(28 %[10]s %[2]d)"/>
</g>
<circle cx="%[1]d" cy="%[2]d" r="72" fill="url(#m)"/>
<circle cx="%[1]d" cy="%[2]d" r="27" fill="%[13]s"/>`,
		p.X, p.Y, n(angle), p.X-104, p.Y-26, p.Y-17, n(length-30), n(end-22), p.Y-28, n(end), top, bottom, cap))
}

func recordSVG(p palette) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<defs>
<linearGradient id="d" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="%[1]s"/><stop offset=".5" stop-color="%[2]s"/>
<stop offset="1" stop-color="%[3]s"/></linearGradient>
<linearGradient id="g" x1="0" y1="0" x2=".6" y2=".8">
<stop offset="0" stop-color="#fff" stop-opacity=".10"/>
<stop offset=".35" stop-color="#fff" stop-opacity=".03"/>
<stop offset=".7" stop-color="#fff" stop-opacity="0"/></linearGradient>
<linearGradient id="r" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="%[4]s" stop-opacity=".16"/>
<stop offset=".35" stop-color="%[4]s" stop-opacity=".04"/>
<stop offset=".65" stop-color="%[4]s" stop-opacity="0"/></linearGradient>
</defs>
<circle cx="%[5]s" cy="%[6]s" r="%[7]s" fill="url(#d)"/>
`, p.discTop, p.discMid, p.discEnd, p.rim,
		n(rec.cx), n(rec.cy), n(rec.r))
	// Uniform grooves at constant low opacity — no banding.
	for r := rec.r*0.30; r < rec.r-10; r += 3.5 {
		fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-opacity="%s" stroke-width="1.3"/>`+"\n",
			n(rec.cx), n(rec.cy), n(r), p.groove, n(p.groovePeak))
	}
	fmt.Fprintf(&b, `<circle cx="%[1]s" cy="%[2]s" r="%[3]s" fill="url(#g)"/>
<circle cx="%[1]s" cy="%[2]s" r="%[3]s" fill="none" stroke="url(#r)" stroke-width="6"/>
`,
		n(rec.cx), n(rec.cy), n(rec.r))
	return svg(b.String())
}


func labelSVG(p palette) string {
	return svg(fmt.Sprintf(`<defs>
<radialGradient id="l" cx="%[1]s" cy="%[2]s" r="%[3]s" gradientUnits="userSpaceOnUse">
<stop offset="0" stop-color="%[6]s"/><stop offset=".45" stop-color="%[7]s"/>
<stop offset="1" stop-color="%[7]s"/></radialGradient>
</defs>
<circle cx="%[1]s" cy="%[2]s" r="%[3]s" fill="url(#l)"/>
<circle cx="%[1]s" cy="%[2]s" r="%[4]s" fill="none" stroke="%[8]s" stroke-opacity="0.22" stroke-width="2"/>
<circle cx="%[1]s" cy="%[2]s" r="%[5]s" fill="none" stroke="%[8]s" stroke-opacity="0.12" stroke-width="1.5"/>
<circle cx="%[1]s" cy="%[2]s" r="%[9]s" fill="%[10]s"/>
<circle cx="%[1]s" cy="%[2]s" r="%[11]s" fill="%[12]s"/>`,
		n(rec.cx), n(rec.cy),
		n(rec.r*0.296),           // label radius
		n(rec.r*0.25),            // inner ring 1
		n(rec.r*0.146),           // inner ring 2
		p.labelTop, p.labelBottom,
		p.labelRing,
		n(rec.r*0.041), p.spindle, // spindle hole outer
		n(rec.r*0.014), "#2c2c2c", // spindle hole inner
	))
}

// render has ictool draw document at the full 1024-pixel size: the icon
// fills its canvas, and macOS applies the rounded corners. An inset icon
// on the old macOS grid left a transparent ring when Windows showed this
// PNG as the app's icon.
func render(document, output string) error {
	dir, err := os.MkdirTemp("", "icon")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	drawn := filepath.Join(dir, "icon.png")
	if out, err := exec.Command(ictool, document, "--export-image", "--output-file", drawn,
		"--platform", "macOS", "--rendition", "Default", "--width", "1024", "--height", "1024", "--scale", "1").CombinedOutput(); err != nil {
		return fmt.Errorf("ictool (it comes with Xcode 26 or later): %v\n%s", err, out)
	}
	// ictool draws in Display P3, and the PNG this writes carries no
	// profile, which readers take as sRGB: convert, or the reds go dull.
	if out, err := exec.Command("sips", "--matchTo", "/System/Library/ColorSync/Profiles/sRGB Profile.icc", drawn).CombinedOutput(); err != nil {
		return fmt.Errorf("sips: %v\n%s", err, out)
	}
	b, err := os.ReadFile(drawn)
	if err != nil {
		return err
	}
	return os.WriteFile(output, b, 0o644)
}
