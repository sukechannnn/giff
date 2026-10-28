package util

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
)

// minReadableContrast is the WCAG contrast ratio text must reach on a
// highlighted row. The cursor color is bright enough that dim syntax colors,
// such as comments, all but vanish on it otherwise.
const minReadableContrast = 3.0

// readableColors caches readableOn by "fg|bg"
var readableColors sync.Map

// EnsureReadableForeground rewrites the foreground of every style tag in a
// tview-tagged line drawn on rowBg, lightening the colors that would be hard
// to read on their background toward white, which keeps their hue. A tag with
// a background of its own, such as a search highlight, is measured against it.
func EnsureReadableForeground(line string, rowBg string) string {
	var sb strings.Builder
	i := 0
	for i < len(line) {
		if line[i] == '[' {
			rest := line[i:]
			if m := MatchTviewEscapedTag(rest); m != "" {
				sb.WriteString(m)
				i += len(m)
				continue
			}
			if m := MatchTviewRegionTag(rest); m != "" {
				sb.WriteString(m)
				i += len(m)
				continue
			}
			if m := MatchTviewStyleTag(rest); m != "" {
				parts := strings.SplitN(m[1:len(m)-1], ":", 2)
				bg := rowBg
				if len(parts) == 2 {
					if tagBg := strings.SplitN(parts[1], ":", 2)[0]; tagBg != "" && tagBg != "-" {
						bg = tagBg
					}
				}
				parts[0] = readableOn(parts[0], bg)
				sb.WriteString("[" + strings.Join(parts, ":") + "]")
				i += len(m)
				continue
			}
		}
		sb.WriteByte(line[i])
		i++
	}
	return sb.String()
}

// readableOn returns fg, lightened toward white just enough to reach
// minReadableContrast on bg. A color that is readable already, the default
// color, and names tcell does not know are returned unchanged.
func readableOn(fg, bg string) string {
	if fg == "" || fg == "-" {
		return fg
	}
	key := fg + "|" + bg
	if v, ok := readableColors.Load(key); ok {
		return v.(string)
	}

	result := fg
	fgColor, bgColor := tcell.GetColor(fg), tcell.GetColor(bg)
	if fgColor != tcell.ColorDefault && bgColor != tcell.ColorDefault {
		r, g, b := fgColor.RGB()
		bgLum := luminance(bgColor.RGB())
		if contrastRatio(luminance(r, g, b), bgLum) < minReadableContrast {
			for step := 1; step <= 10; step++ {
				t := float64(step) / 10
				lr, lg, lb := mix(r, t), mix(g, t), mix(b, t)
				if contrastRatio(luminance(lr, lg, lb), bgLum) >= minReadableContrast || step == 10 {
					result = fmt.Sprintf("#%02x%02x%02x", lr, lg, lb)
					break
				}
			}
		}
	}
	readableColors.Store(key, result)
	return result
}

// mix moves a color channel the fraction t of the way to white
func mix(c int32, t float64) int32 {
	return c + int32(math.Round(float64(255-c)*t))
}

// luminance returns the WCAG relative luminance of an RGB color
func luminance(r, g, b int32) float64 {
	linear := func(c int32) float64 {
		v := float64(c) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

// contrastRatio returns the WCAG contrast ratio of two luminances
func contrastRatio(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}
