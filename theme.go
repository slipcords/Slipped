//go:build !cli

/*
 * SPDX-License-Identifier: GPL-3.0
 * Vencord Installer, a cross platform gui/cli app for installing Vencord
 * Copyright (c) 2023 Vendicated and Vencord contributors
 */

package main

import (
	_ "embed"
	"image/color"

	g "github.com/AllenDang/giu"
)

//go:embed winres/fonts/SpaceGrotesk-Regular.ttf
var fontSpaceGroteskRegular []byte

//go:embed winres/fonts/SpaceGrotesk-Bold.ttf
var fontSpaceGroteskBold []byte

/*
Design system — Slipcord installer.

Identity: Discord-grounded dark chrome (the subject of this installer is the
Discord desktop client), finished like a calm developer tool: flat hairline
borders over solid surfaces, Space Grotesk display type, one blurple call to
action, colour only ever carries state.

Tokens

	bg          #1B1E22   window
	surface     #23262B   cards / panes
	surfaceAlt  #2B2F35   inputs / secondary buttons
	line        #343A45   hairlines
	text        #E9EBEF
	muted       #98A1AE
	accent      #5865F2   Slipcord blurple (= DiscordBlue)
	accentHover #6B76F7
	success     #23A55A
	danger      #EC4144   (= DiscordRed)
	warning     #F0B232

Type

	Space Grotesk, embedded (OFL-1.1, see winres/fonts/OFL-SpaceGrotesk.txt).
	Regular is the default at 17px; Bold drives the wordmark and section heads.
*/
const themeFontSize = 17

var (
	colourBg           = color.RGBA{0x1B, 0x1E, 0x22, 0xFF}
	colourSurface      = color.RGBA{0x23, 0x26, 0x2B, 0xFF}
	colourSurfaceAlt   = color.RGBA{0x2B, 0x2F, 0x35, 0xFF}
	colourSurfaceHover = color.RGBA{0x33, 0x37, 0x3E, 0xFF}
	colourLine         = color.RGBA{0x34, 0x3A, 0x45, 0xFF}
	colourText         = color.RGBA{0xE9, 0xEB, 0xEF, 0xFF}
	colourMuted        = color.RGBA{0x98, 0xA1, 0xAE, 0xFF}

	colourAccentHover = color.RGBA{0x6B, 0x76, 0xF7, 0xFF}
	colourAccentDim   = color.RGBA{0x47, 0x51, 0xC4, 0xFF}

	colourSuccessHover = color.RGBA{0x2D, 0xB4, 0x5F, 0xFF}
	colourSuccessDim   = color.RGBA{0x1E, 0x8E, 0x4C, 0xFF}

	colourDangerHover = color.RGBA{0xF6, 0x5A, 0x5D, 0xFF}
	colourDangerDim   = color.RGBA{0xEC, 0x41, 0x44, 0x28}
	colourDangerDk    = color.RGBA{0xC2, 0x2E, 0x31, 0xFF}

	colourWarning    = color.RGBA{0xF0, 0xB2, 0x32, 0xFF}
	colourWarningDim = color.RGBA{0xF0, 0xB2, 0x32, 0x28}
)

var sliptFontBold *g.FontInfo

// initFonts registers the embedded type before the window is created so the
// ImGui font atlas is built with Space Grotesk as the default font.
func initFonts() {
	g.SetDefaultFontFromBytes(fontSpaceGroteskRegular, themeFontSize)
	sliptFontBold = g.AddFontFromBytes("Space Grotesk Bold", fontSpaceGroteskBold, themeFontSize)
}

func boldText(size float32, txt string) g.Widget {
	return g.Style().
		SetFont(sliptFontBold).
		SetFontSize(size).
		To(g.Label(txt))
}

func sectionTitle(txt string) g.Widget {
	return boldText(20, txt)
}

func mutedText(size float32, txt string) g.Widget {
	return g.Style().
		SetFontSize(size).
		SetColor(g.StyleColorText, colourMuted).
		To(g.Label(txt))
}

// paneCard draws one of the two content panes: a flat surface with a hairline.
func paneCard(width, height float32, layout ...g.Widget) g.Widget {
	return g.Style().
		SetColor(g.StyleColorChildBg, colourSurface).
		SetColor(g.StyleColorBorder, colourLine).
		SetStyle(g.StyleVarWindowPadding, 20, 18).
		SetStyleFloat(g.StyleVarChildBorderSize, 1).
		SetStyleFloat(g.StyleVarChildRounding, 12).
		To(g.Child().Border(true).Size(width, height).Layout(g.Column(layout...)))
}

// selectionStyle colours a selectable row: blurple when chosen, otherwise
// invisible on the pane surface until hovered.
func selectionStyle(selected bool) *g.StyleSetter {
	if selected {
		return g.Style().
			SetColor(g.StyleColorHeader, colourAccentDim).
			SetColor(g.StyleColorHeaderHovered, colourAccentHover).
			SetColor(g.StyleColorHeaderActive, colourAccentHover)
	}
	return g.Style().
		SetColor(g.StyleColorHeader, colourSurface).
		SetColor(g.StyleColorHeaderHovered, colourSurfaceHover).
		SetColor(g.StyleColorHeaderActive, colourSurfaceAlt)
}

// actionButton is the single shared style for the bottom action bar.
func actionButton(label string, bg, hover, active color.Color, tip string, disabled bool, width, height float32, onClick func()) g.Widget {
	return g.Style().
		SetColor(g.StyleColorButton, bg).
		SetColor(g.StyleColorButtonHovered, hover).
		SetColor(g.StyleColorButtonActive, active).
		SetStyle(g.StyleVarFramePadding, 18, 8).
		SetStyleFloat(g.StyleVarFrameRounding, 8).
		SetDisabled(disabled).
		To(
			g.Button(label).OnClick(onClick).Size(width, height),
			Tooltip(tip),
		)
}

// inputBoxStyle matches inputs to the rest of the theme.
func inputBoxStyle() *g.StyleSetter {
	return g.Style().
		SetColor(g.StyleColorFrameBg, colourSurfaceAlt).
		SetColor(g.StyleColorFrameBgHovered, colourSurfaceHover).
		SetColor(g.StyleColorFrameBgActive, colourSurfaceHover).
		SetStyle(g.StyleVarFramePadding, 12, 10).
		SetStyleFloat(g.StyleVarFrameRounding, 8)
}

func shortHash(h string) string {
	if len(h) > 10 {
		return h[:10]
	}
	return h
}
