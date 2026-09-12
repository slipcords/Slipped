//go:build !cli

package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"math"
	"time"

	g "github.com/AllenDang/giu"
)

//go:embed winres/fonts/SpaceGrotesk-Regular.ttf
var fontSpaceGroteskRegular []byte

//go:embed winres/fonts/SpaceGrotesk-Bold.ttf
var fontSpaceGroteskBold []byte

const themeFontSize = 17

// Palette: a warm night sky with a bronze settled below it. The gold comes
// from the app icon's own tones, not Discord's blurple.
var (
	colourSky       = color.RGBA{0x0C, 0x0A, 0x07, 0xFF}
	colourSky2      = color.RGBA{0x1E, 0x13, 0x05, 0xFF}
	colourSurface   = color.RGBA{0x1B, 0x12, 0x09, 0xFF}
	colourWell      = color.RGBA{0x15, 0x10, 0x0A, 0xFF}
	colourWellHover = color.RGBA{0x24, 0x1B, 0x0F, 0xFF}
	colourRail      = color.RGBA{0x38, 0x2B, 0x1C, 0xFF}
	colourGold      = color.RGBA{0xE9, 0xB4, 0x5B, 0xFF}
	colourGoldHi    = color.RGBA{0xF9, 0xCF, 0x87, 0xFF}
	colourGoldDeep  = color.RGBA{0x2E, 0x22, 0x12, 0xFF}
	colourGoldDim   = color.RGBA{0x3A, 0x2B, 0x14, 0xFF}
	colourInk       = color.RGBA{0xF6, 0xEC, 0xD8, 0xFF}
	colourMuted     = color.RGBA{0xA7, 0x93, 0x70, 0xFF}
	colourFaint     = color.RGBA{0x6B, 0x5C, 0x43, 0xFF}
	colourDanger    = color.RGBA{0xDB, 0x69, 0x53, 0xFF}
	colourDangerDim = color.RGBA{0x2E, 0x18, 0x12, 0xFF}
	colourSuccess   = color.RGBA{0x8F, 0xC8, 0x7F, 0xFF}
)

var (
	sliptFontBold *g.FontInfo
	curAlpha      float32 = 1
	curRise       float32
	skyT0         = time.Now()
)

func initFonts() {
	g.SetDefaultFontFromBytes(fontSpaceGroteskRegular, themeFontSize)
	sliptFontBold = g.AddFontFromBytes("Space Grotesk Bold", fontSpaceGroteskBold, themeFontSize)
}

func clamp01(a float32) float32 {
	if a < 0 {
		return 0
	}
	if a > 1 {
		return 1
	}
	return a
}

// rgba scales an opaque color's alpha by a (used for page fades).
func rgba(c color.RGBA, a float32) color.RGBA {
	return color.RGBA{c.R, c.G, c.B, uint8(math.Round(float64(c.A) * float64(clamp01(a))))}
}

func lerpColor(a, b color.RGBA, t float32) color.RGBA {
	f := clamp01(t)
	return color.RGBA{
		uint8(float64(a.R) + float64(int(b.R)-int(a.R))*float64(f)),
		uint8(float64(a.G) + float64(int(b.G)-int(a.G))*float64(f)),
		uint8(float64(a.B) + float64(int(b.B)-int(a.B))*float64(f)),
		0xFF,
	}
}

func fract(x float32) float32 { return x - float32(math.Floor(float64(x))) }
func hashf(i int) float32     { return fract(float32(math.Sin(float64(i)*127.1)) * 43758.5453) }

func boldText(size float32, col color.RGBA, txt string) g.Widget {
	return g.Style().
		SetFont(sliptFontBold).
		SetFontSize(size).
		SetColor(g.StyleColorText, rgba(col, curAlpha)).
		To(g.Label(txt))
}

func textC(size float32, col color.RGBA, txt string) g.Widget {
	return g.Style().
		SetFontSize(size).
		SetColor(g.StyleColorText, rgba(col, curAlpha)).
		To(g.Label(txt))
}

func mutedText(size float32, txt string) g.Widget { return textC(size, colourMuted, txt) }

// textCW is textC with wrapping enabled, for body copy inside a column.
func textCW(size float32, col color.RGBA, txt string) g.Widget {
	return g.Style().
		SetFontSize(size).
		SetColor(g.StyleColorText, rgba(col, curAlpha)).
		To(g.Label(txt).Wrapped(true))
}

func shortHash(h string) string {
	if len(h) > 10 {
		return h[:10]
	}
	return h
}

func goldButton(label string, w, h float32, onClick func()) g.Widget {
	style := g.Style().
		SetColor(g.StyleColorButton, rgba(colourGold, curAlpha)).
		SetColor(g.StyleColorButtonHovered, rgba(colourGoldHi, curAlpha)).
		SetColor(g.StyleColorButtonActive, rgba(colourGoldDeep, curAlpha)).
		SetColor(g.StyleColorText, rgba(colourSky, 1)).
		SetStyle(g.StyleVarFramePadding, 0, 0).
		SetStyleFloat(g.StyleVarFrameRounding, 16)
	return g.Custom(func() {
		if w <= 0 {
			aw, _ := g.GetAvailableRegion()
			w = aw
		}
		style.To(g.Button(label).OnClick(onClick).Size(w, h)).Build()
	})
}

func ghostButton(label string, w, h float32, onClick func()) g.Widget {
	style := g.Style().
		SetColor(g.StyleColorButton, color.RGBA{0, 0, 0, 0}).
		SetColor(g.StyleColorButtonHovered, rgba(colourRail, curAlpha)).
		SetColor(g.StyleColorButtonActive, rgba(colourGoldDeep, curAlpha)).
		SetColor(g.StyleColorText, rgba(colourInk, curAlpha)).
		SetStyle(g.StyleVarFramePadding, 0, 0).
		SetStyleFloat(g.StyleVarFrameRounding, 16)
	return g.Custom(func() {
		if w <= 0 {
			aw, _ := g.GetAvailableRegion()
			w = aw
		}
		style.To(g.Button(label).OnClick(onClick).Size(w, h)).Build()
	})
}

// cardButton is the quiet choice row for the "what should I do" page.
func cardButton(label, tip string, w, h float32, onClick func()) g.Widget {
	btn := g.Style().
		SetColor(g.StyleColorButton, rgba(colourSurface, curAlpha)).
		SetColor(g.StyleColorButtonHovered, rgba(colourGoldDim, curAlpha)).
		SetColor(g.StyleColorButtonActive, rgba(colourGoldDeep, curAlpha)).
		SetColor(g.StyleColorText, rgba(colourInk, curAlpha)).
		SetStyle(g.StyleVarFramePadding, 0, 0).
		SetStyleFloat(g.StyleVarFrameRounding, 18).
		SetStyle(g.StyleVarButtonTextAlign, 0.06, 0.5).
		To(g.Button(label).OnClick(onClick).Size(w, h))
	if tip == "" {
		return btn
	}
	return g.Style().To(btn, Tooltip(tip))
}

func inputBoxStyle() *g.StyleSetter {
	return g.Style().
		SetColor(g.StyleColorFrameBg, rgba(colourWell, curAlpha)).
		SetColor(g.StyleColorFrameBgHovered, rgba(colourWellHover, curAlpha)).
		SetColor(g.StyleColorFrameBgActive, rgba(colourWellHover, curAlpha)).
		SetStyle(g.StyleVarFramePadding, 16, 14).
		SetStyleFloat(g.StyleVarFrameRounding, 14)
}

func selectionStyle(selected bool) *g.StyleSetter {
	if selected {
		return g.Style().
			SetColor(g.StyleColorHeader, rgba(colourGoldDeep, curAlpha)).
			SetColor(g.StyleColorHeaderHovered, rgba(colourGoldDim, curAlpha)).
			SetColor(g.StyleColorHeaderActive, rgba(colourGoldDim, curAlpha))
	}
	return g.Style().
		SetColor(g.StyleColorHeader, rgba(colourWell, curAlpha)).
		SetColor(g.StyleColorHeaderHovered, rgba(colourWellHover, curAlpha)).
		SetColor(g.StyleColorHeaderActive, rgba(colourWellHover, curAlpha))
}

// stageCard is the shared card frame every step sits in.
func stageCard(w, h float32, layout ...g.Widget) g.Widget {
	return g.Style().
		SetColor(g.StyleColorChildBg, rgba(colourSurface, curAlpha)).
		SetColor(g.StyleColorBorder, rgba(colourRail, curAlpha)).
		SetStyle(g.StyleVarWindowPadding, 40, 36).
		SetStyleFloat(g.StyleVarChildBorderSize, 1.2).
		SetStyleFloat(g.StyleVarChildRounding, 26).
		To(g.Child().Border(true).Size(w, h).Layout(g.Column(layout...)))
}

func pips(active, total int) g.Widget {
	return g.Custom(func() {
		pos := g.GetCursorScreenPos()
		sp := 30
		for i := 0; i < total; i++ {
			col := colourRail
			if i == active {
				col = colourGold
			}
			g.GetCanvas().AddCircleFilled(image.Pt(pos.X+i*sp+6, pos.Y+6), 5, rgba(col, curAlpha))
		}
		g.Dummy(float32(total*sp), 13).Build()
	})
}

// cometBar is the eased progress bar with a warm glowing tip.
func cometBar(frac, w, h float32) g.Widget {
	return g.Custom(func() {
		pos := g.GetCursorScreenPos()
		canvas := g.GetCanvas()
		canvas.AddRectFilled(pos, image.Pt(pos.X+int(w), pos.Y+int(h)), rgba(colourWell, curAlpha), h/2, 0)
		f := clamp01(frac)
		if f > 0 {
			fw := int(w * f)
			canvas.AddRectFilled(pos, image.Pt(pos.X+fw, pos.Y+int(h)), rgba(colourGold, curAlpha), h/2, 0)
			cy := pos.Y + int(h/2)
			t := float32(time.Since(skyT0).Seconds())
			pulse := 0.6 + 0.4*float32(math.Sin(float64(t)*9))
			canvas.AddCircleFilled(image.Pt(pos.X+fw, cy), h*1.6*pulse, rgba(colourGoldHi, 0.5*curAlpha))
			canvas.AddCircleFilled(image.Pt(pos.X+fw, cy), h*0.7, rgba(colourGoldHi, 0.95*curAlpha))
		}
		g.Dummy(w, h).Build()
	})
}

// drawSky paints the full-window night scene: gradient, drifting stars and a
// few shooting stars. Deterministic per frame, no per-app state.
func drawSky(ww, wh int, origin image.Point, canvas *g.Canvas) {
	t := float32(time.Since(skyT0).Seconds())

	strips := 7
	for i := 0; i <= strips; i++ {
		y0 := origin.Y + wh*i/strips
		y1 := origin.Y + wh*(i+1)/strips
		tt := float32(i) / float32(strips)
		canvas.AddRectFilled(image.Pt(origin.X, y0), image.Pt(origin.X+ww, y1), lerpColor(colourSky, colourSky2, tt), 0, 0)
	}

	for i := 0; i < 90; i++ {
		x := hashf(i*7+1) * float32(ww)
		drift := hashf(i*7+3) * 5
		y := fract(hashf(i*7+2)*float32(wh)+t*drift) * float32(wh)
		twk := 0.5 + 0.5*float32(math.Sin(float64(t*float32(0.7+2*hashf(i*7+4)))+float64(hashf(i*7+5)*6.28)))
		a := (0.10 + 0.45*twk) * curAlpha
		c := color.RGBA{0xF6, 0xEC, 0xD8, uint8(255 * a)}
		if i%12 == 0 {
			c = color.RGBA{0xF9, 0xCF, 0x87, uint8(255 * a)}
		}
		r := int(1 + 0.7*hashf(i*7+6))
		px := int(origin.X) + int(x)
		py := int(origin.Y) + int(y)
		canvas.AddRectFilled(image.Pt(px, py), image.Pt(px+r, py+r), c, 0, 0)
	}

	for i := 0; i < 3; i++ {
		period := 5.5 + 3.5*hashf(i*13+1)
		u := fract(t/period) / 0.26
		if u >= 1 {
			continue
		}
		ease := u * u * (3 - 2*u)
		alpha := float32(math.Sin(float64(u)*math.Pi)) * curAlpha
		sx := hashf(i*13+2) * 0.7 * float32(ww)
		sy := hashf(i*13+3) * 0.34 * float32(wh)
		d := 0.95 * float32(ww) * ease
		x := sx + d*0.82
		y := sy + d*0.36
		head := image.Pt(int(origin.X)+int(x), int(origin.Y)+int(y))
		tail := 170
		tip := image.Pt(head.X-tail, head.Y-int(float32(tail)*0.42))
		canvas.AddLine(tip, head, rgba(colourGoldDim, 1.2*alpha), 5)
		canvas.AddLine(tip, head, rgba(colourGoldHi, 0.9*alpha), 2)
		canvas.AddCircleFilled(head, 3, rgba(colourGoldHi, alpha))
	}
}

var logoTexture *g.Texture

func initIconTexture() {
	img, _, err := image.Decode(bytes.NewReader(iconBytes))
	if err != nil {
		Log.Error("Failed to decode app icon", err)
		return
	}
	rgbi := image.NewRGBA(img.Bounds())
	draw.Draw(rgbi, rgbi.Bounds(), img, image.Point{}, draw.Src)
	g.NewTextureFromRgba(rgbi, func(t *g.Texture) { logoTexture = t })
}

func logo(size float32) g.Widget {
	if logoTexture == nil {
		return g.Dummy(0, 0)
	}
	return g.Image(logoTexture).Size(size, size)
}
