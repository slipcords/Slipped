//go:build !cli

/*
 * SPDX-License-Identifier: GPL-3.0
 * Vencord Installer, a cross platform gui/cli app for installing Vencord
 * Copyright (c) 2023 Vendicated and Vencord contributors
 */

package main

import (
	"bytes"
	_ "embed"
	"errors"
	"image"
	_ "image/png"
	"math"
	"os"
	path "path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"slipped/buildinfo"

	g "github.com/AllenDang/giu"
	"github.com/AllenDang/imgui-go"
)

var (
	discords        []any
	radioIdx        int
	customChoiceIdx int

	customDir              string
	autoCompleteDir        string
	autoCompleteFile       string
	autoCompleteCandidates []string
	autoCompleteIdx        int
	lastAutoComplete       string
	didAutoComplete        bool

	modalId      = 0
	modalTitle   = "Oh No :("
	modalMessage = "You should never see this"

	acceptedOpenAsar   bool
	showedUpdatePrompt bool

	win *g.MasterWindow
)

//go:embed winres/icon.png
var iconBytes []byte

func init() {
	LogLevel = LevelDebug
}

func main() {
	initFonts()
	InitGithubDownloader()
	discords = FindDiscords()

	customChoiceIdx = len(discords)

	go func() {
		<-GithubDoneChan
		g.Update()
	}()

	go func() {
		<-SelfUpdateCheckDoneChan
		g.Update()
	}()

	var linuxFlags g.MasterWindowFlags = 0
	if runtime.GOOS == "linux" {
		os.Setenv("GDK_SCALE", "1")
		os.Setenv("GDK_DPI_SCALE", "1")
	}

	win = g.NewMasterWindow("Slipped", 1280, 920, linuxFlags)

	icon, _, err := image.Decode(bytes.NewReader(iconBytes))
	if err != nil {
		Log.Warn("Failed to load application icon", err)
		Log.Debug(iconBytes, len(iconBytes))
	} else {
		win.SetIcon([]image.Image{icon})
	}
	win.Run(loop)
}

type CondWidget struct {
	predicate  bool
	ifWidget   func() g.Widget
	elseWidget func() g.Widget
}

func (w *CondWidget) Build() {
	if w.predicate {
		w.ifWidget().Build()
	} else if w.elseWidget != nil {
		w.elseWidget().Build()
	}
}

func getChosenInstall() *DiscordInstall {
	var choice *DiscordInstall
	if radioIdx == customChoiceIdx {
		choice = ParseDiscord(customDir, "")
		if choice == nil {
			choice = ParseDiscordNew(customDir, "", strings.Contains(customDir, "com.discordapp"))
		}
		if choice == nil {
			g.OpenPopup("#invalid-custom-location")
		}
	} else {
		choice = discords[radioIdx].(*DiscordInstall)
	}
	return choice
}

func InstallLatestBuilds() (err error) {
	if IsDevInstall {
		return
	}

	err = installLatestBuilds()
	if err != nil {
		ShowModal("Uh Oh!", "Failed to install the latest Slipcord builds from GitHub:\n"+err.Error())
	}
	return
}

func HandleScuffedInstall() {
	g.OpenPopup("#scuffed-install")
}

func onCustomInputChanged() {
	p := customDir
	if len(p) != 0 {
		// Select the custom option for people
		radioIdx = customChoiceIdx
	}

	dir := path.Dir(p)

	isNewDir := strings.HasSuffix(p, "/")
	wentUpADir := !isNewDir && dir != autoCompleteDir

	if isNewDir || wentUpADir {
		autoCompleteDir = dir
		// reset all the funnies
		autoCompleteIdx = 0
		lastAutoComplete = ""
		autoCompleteFile = ""
		autoCompleteCandidates = nil

		// Generate autocomplete items
		files, err := os.ReadDir(dir)
		if err == nil {
			for _, file := range files {
				autoCompleteCandidates = append(autoCompleteCandidates, file.Name())
			}
		}
	} else if !didAutoComplete {
		// reset auto complete and update our file
		autoCompleteFile = path.Base(p)
		lastAutoComplete = ""
	}

	if wentUpADir {
		autoCompleteFile = path.Base(p)
	}

	didAutoComplete = false
}

func makeAutoComplete() []any {
	input := strings.ToLower(autoCompleteFile)

	var candidates []any
	for _, e := range autoCompleteCandidates {
		file := strings.ToLower(e)
		if autoCompleteFile == "" || strings.HasPrefix(file, input) {
			candidates = append(candidates, e)
		}
	}
	return candidates
}

func makeRadioOnChange(i int) func() {
	return func() {
		radioIdx = i
	}
}

func Tooltip(label string) g.Widget {
	return g.Style().
		SetStyle(g.StyleVarWindowPadding, 10, 8).
		SetStyleFloat(g.StyleVarWindowRounding, 8).
		To(
			g.Tooltip(label),
		)
}

func InfoModal(id, title, description string) g.Widget {
	return RawInfoModal(id, title, description, false)
}

func RawInfoModal(id, title, description string, isOpenAsar bool) g.Widget {
	isDynamic := strings.HasPrefix(id, "#modal") && !strings.Contains(description, "\n")
	flags := g.WindowFlagsNoTitleBar
	if isDynamic {
		flags |= g.WindowFlagsAlwaysAutoResize
	}
	return g.Style().
		SetStyle(g.StyleVarWindowPadding, 32, 32).
		SetStyleFloat(g.StyleVarWindowRounding, 22).
		To(
			g.PopupModal(id).
				Flags(flags).
				Layout(
					g.Align(g.AlignCenter).To(
						g.Style().SetFontSize(30).To(
							g.Label(title),
						),
						g.Style().SetFontSize(20).To(
							g.Label(description).Wrapped(isDynamic),
						),
						&CondWidget{id == "#scuffed-install", func() g.Widget {
							return g.Column(
								g.Dummy(0, 10),
								g.Button("Take me there!").OnClick(func() {
									// this issue only exists on windows so using Windows specific path is oki
									username := os.Getenv("USERNAME")
									programData := os.Getenv("PROGRAMDATA")
									g.OpenURL("file://" + path.Join(programData, username))
								}).Size(200, 30),
							)
						}, nil},
						g.Dummy(0, 20),
						&CondWidget{isOpenAsar,
							func() g.Widget {
								return g.Row(
									g.Button("Accept").
										OnClick(func() {
											acceptedOpenAsar = true
											g.CloseCurrentPopup()
											startJob(jobOpenAsar)
										}).
										Size(100, 30),
									g.Button("Cancel").
										OnClick(func() {
											g.CloseCurrentPopup()
										}).
										Size(100, 30),
								)
							},
							func() g.Widget {
								return g.Button("Ok").
									OnClick(func() {
										g.CloseCurrentPopup()
									}).
									Size(100, 30)
							},
						},
					),
				),
		)
}

func UpdateModal() g.Widget {
	return g.Style().
		SetStyle(g.StyleVarWindowPadding, 32, 32).
		SetStyleFloat(g.StyleVarWindowRounding, 22).
		To(
			g.PopupModal("#update-prompt").
				Flags(g.WindowFlagsNoTitleBar | g.WindowFlagsAlwaysAutoResize).
				Layout(
					g.Align(g.AlignCenter).To(
						g.Style().SetFontSize(30).To(
							g.Label("Your Installer is outdated!"),
						),
						g.Style().SetFontSize(20).To(
							g.Label(
								"Would you like to update now?\n\n"+
									"Once you press Update Now, the new installer will automatically be downloaded.\n"+
									"The installer will temporarily seem unresponsive. Just wait!\n"+
									"Once the update is done, the Installer will automatically reopen.\n\n"+
									"On MacOs, Auto updates are not supported, so it will instead open in browser.",
							),
						),
						g.Row(
							g.Button("Update Now").
								OnClick(func() {
									if runtime.GOOS == "darwin" {
										g.CloseCurrentPopup()
										g.OpenURL(GetInstallerDownloadLink())
										return
									}

									err := UpdateSelf()
									g.CloseCurrentPopup()

									if err != nil {
										ShowModal("Failed to update self!", err.Error())
									} else {
										if err = RelaunchSelf(); err != nil {
											ShowModal("Failed to restart self! Please do it manually.", err.Error())
										}
									}
								}).
								Size(100, 30),
							g.Button("Later").
								OnClick(func() {
									g.CloseCurrentPopup()
								}).
								Size(100, 30),
						),
					),
				),
		)
}

func ShowModal(title, desc string) {
	modalTitle = title
	modalMessage = desc
	modalId++
	g.OpenPopup("#modal" + strconv.Itoa(modalId))
}

// humanizeErr turns a raw error into a message that tells the user what to do.
func humanizeErr(di *DiscordInstall, err error) string {
	if errors.Is(err, os.ErrPermission) {
		switch runtime.GOOS {
		case "windows":
			return "Permission denied. Make sure your Discord is fully closed (from the tray)!"
		case "darwin":
			command := "sudo chown -R \"${USER}:wheel\" " + di.path
			return "Permission denied. Please grant the installer Full Disk Access in the system settings (privacy & security page).\n\nIf that also doesn't work, try running the following command in your terminal:\n" + command
		case "linux":
			command := "sudo chown -R \"$USER:$USER\" " + di.path
			return "Permission denied. Try to run the installer with sudo privileges.\n\nIf that also doesn't work, try running the following command in your terminal:\n" + command
		default:
			return "Permission denied. Maybe try running me as Administrator/Root?"
		}
	}
	return err.Error()
}

// ---- wizard: five steps, one stage, a starfield behind it ----

type wizardPage int

const (
	pgWelcome wizardPage = iota
	pgPath
	pgAction
	pgProgress
	pgDone
)

type jobKind int

const (
	jobInstall jobKind = iota
	jobRepair
	jobUninstall
	jobOpenAsar
)

const stageW = 620

var (
	page      = pgWelcome
	pageT     time.Time
	welcomeAt time.Time

	jobKindNow     jobKind
	jobBranch      string
	jobTarget      string
	jobWasOpenAsar bool
	jobDi          *DiscordInstall
	jobDone        atomic.Int32
	jobErr         error
	jobStartedAt   time.Time
	jobDoneSeenAt  time.Time
)

func switchPage(p wizardPage) {
	page = p
	pageT = time.Now()
	if p == pgWelcome {
		welcomeAt = time.Now()
	}
}

func pageAnim() (alpha, rise float32) {
	el := float32(time.Since(pageT).Seconds())
	k := clamp01(el / 0.42)
	s := k * k * (3 - 2*k)
	return s, (1 - s) * 22
}

func startJob(k jobKind) {
	choice := getChosenInstall()
	if choice == nil {
		g.OpenPopup("#invalid-custom-location")
		return
	}
	if k == jobOpenAsar && !acceptedOpenAsar && !choice.IsOpenAsar() {
		g.OpenPopup("#openasar-confirm")
		return
	}
	jobKindNow = k
	jobBranch = choice.branch
	jobTarget = choice.path
	jobWasOpenAsar = choice.IsOpenAsar()
	jobDi = choice
	jobErr = nil
	jobDone.Store(0)
	jobDoneSeenAt = time.Time{}
	jobStartedAt = time.Now()
	switchPage(pgProgress)
	go runJob(choice)
}

func runJob(d *DiscordInstall) {
	record := func(e error) {
		if e != nil && !errors.Is(e, ErrAlreadyReported) {
			jobErr = e
		}
	}
	switch jobKindNow {
	case jobInstall:
		if CheckScuffedInstall() {
			jobErr = errors.New("Your Discord install looks broken — it ended up somewhere Discord doesn't expect. Fully quit Discord, delete the Discord and Squirrel folders next to it, reinstall Discord, then come back here.")
		} else {
			record(d.patch())
		}
	case jobRepair:
		if IsDevInstall {
			record(d.patch())
		} else {
			if e := installLatestBuilds(); e == nil {
				record(d.patch())
			} else {
				jobErr = e
			}
		}
	case jobUninstall:
		jobErr = d.unpatch()
	case jobOpenAsar:
		if d.IsOpenAsar() {
			jobErr = d.UninstallOpenAsar()
		} else {
			jobErr = d.InstallOpenAsar()
		}
	}
	jobDone.Store(1)
	g.Update()
}

func progressFrac() float32 {
	if jobDone.Load() == 1 {
		return 1
	}
	el := float32(time.Since(jobStartedAt).Seconds())
	return float32(math.Min(float64(el/2.4), 0.9))
}

func jobTexts() (title, desc, done string) {
	switch jobKindNow {
	case jobInstall:
		return "Slipping Slipcord in…",
			"Patching the Discord in place. Nothing here touches anything else on your machine.",
			"Slipcord is in. Fully close Discord, then open it again — you'll find Slipcord under Settings."
	case jobRepair:
		return "Repairing Slipcord…",
			"Fetching the freshest Slipcord build, then patching it back in.",
			"Repaired — " + jobBranch + " is patched with the latest Slipcord build."
	case jobUninstall:
		return "Removing Slipcord…",
			"Giving " + jobBranch + " back its official app.asar.",
			"Slipcord is out of " + jobBranch + ". Discord is back to the official app."
	default:
		if jobWasOpenAsar {
			return "Working on OpenAsar…",
				"Swapping " + jobBranch + "'s core back to Discord's official app.",
				"OpenAsar was taken off " + jobBranch + "."
		}
		return "Working on OpenAsar…",
			"Swapping " + jobBranch + "'s core with the community OpenAsar.",
			"OpenAsar is now running on " + jobBranch + "."
	}
}

func chosenLabel() string {
	if radioIdx == customChoiceIdx {
		return "custom location"
	}
	if radioIdx >= 0 && radioIdx < len(discords) {
		return strings.Title(discords[radioIdx].(*DiscordInstall).branch)
	}
	return "?"
}

// ---- page building ----

func wizardLayout() g.Layout {
	ww, wh := win.GetSize()
	layout := g.Layout{
		g.Custom(func() {
			drawSky(ww, wh, g.GetCursorScreenPos(), g.GetCanvas())
		}),
		pageContent(ww, wh),
		footer(wh),
	}
	layout = append(layout,
		InfoModal("#scuffed-install", "Hold On!", "You have a broken Discord Install.\n"+
			"Sometimes Discord decides to install to the wrong location for some reason!\n"+
			"You need to fix this before patching, otherwise Slipcord will likely not work.\n\n"+
			"Use the below button to jump there and delete any folder called Discord or Squirrel.\n"+
			"If the folder is now empty, feel free to go back a step and delete that folder too.\n"+
			"Then see if Discord still starts. If not, reinstall it"),
		RawInfoModal("#openasar-confirm", "OpenAsar", "OpenAsar is an open-source alternative of Discord desktop's app.asar.\n"+
			"Slipcord is in no way affiliated with OpenAsar.\n"+
			"You're installing OpenAsar at your own risk. If you run into issues with OpenAsar,\n"+
			"no support will be provided, join the OpenAsar Server instead!\n\n"+
			"To install OpenAsar, press Accept and it gets installed right away.", true),
		InfoModal("#invalid-custom-location", "Invalid Location", "The specified location is not a valid Discord install.\nMake sure you select the base folder.\n\nHint: Discord snap is not supported. use flatpak or .deb"),
		InfoModal("#modal"+strconv.Itoa(modalId), modalTitle, modalMessage),
		UpdateModal(),
	)
	return layout
}

func pageContent(ww, wh int) g.Widget {
	switch page {
	case pgWelcome:
		return welcomePage(wh)
	case pgPath:
		return pathPage(wh)
	case pgAction:
		return actionPage(wh)
	case pgProgress:
		return progressPage(wh)
	default:
		return donePage(wh)
	}
}

func footer(wh int) g.Widget {
	return g.Custom(func() {
		g.SetCursorPos(image.Pt(26, wh-38))
		mutedText(13, "Installer "+buildinfo.InstallerTag+" ("+shortHash(buildinfo.InstallerGitHash)+") — downloads stay on this device").Build()
	})
}

func welcomePage(wh int) g.Widget {
	return g.Custom(func() {
		heroY := int(float32(wh)*0.185) + int(curRise)
		g.SetCursorPos(image.Pt(0, heroY))
		g.Align(g.AlignCenter).To(
			logo(112),
			g.Dummy(0, 24),
			boldText(92, colourGold, "Slipped"),
			g.Dummy(0, 8),
			textC(20, colourInk, "the Slipcord installer"),
			g.Dummy(0, 12),
			textCW(14, colourMuted, "it slips cleanly into your Discord — no browser, no sketchy site"),
			g.Dummy(0, 50),
			pips(0, 5),
			g.Dummy(0, 18),
			textC(13, colourFaint, "moving on in a moment — or press enter"),
		).Build()
	})
}

func pathPage(wh int) g.Widget {
	rows := len(discords) + 1
	estH := float32(340 + rows*88)

	var col g.Layout
	col = append(col,
		pips(1, 5),
		g.Dummy(0, 8),
		boldText(38, colourInk, "Pick a Discord to patch"),
		g.Dummy(0, 8),
		textCW(17, colourMuted, "Found on this machine — or point at a folder yourself."),
		g.Dummy(0, 22),
	)

	for i, v := range discords {
		d := v.(*DiscordInstall)
		col = append(col, installOptionRow(strings.Title(d.branch), d.path, d.isPatched, radioIdx == i, makeRadioOnChange(i)))
	}
	customPath := "not set yet"
	if customDir != "" {
		customPath = customDir
	}
	col = append(col,
		installOptionRow("Custom Install Location", customPath, false, radioIdx == customChoiceIdx, makeRadioOnChange(customChoiceIdx)),
		g.Dummy(0, 14),
		inputBoxStyle().To(
			g.InputText(&customDir).Hint("Other location — e.g. C:\\Users\\you\\AppData\\Local\\Discord").
				Flags(g.InputTextFlagsCallbackCompletion).
				OnChange(onCustomInputChanged).
				// this library has its own autocomplete but it's broken
				Callback(
					func(data imgui.InputTextCallbackData) int32 {
						candidates := makeAutoComplete()
						if len(candidates) == 0 {
							return 0
						}
						if autoCompleteIdx >= len(candidates) {
							autoCompleteIdx = 0
						}
						if didAutoComplete && lastAutoComplete != "" {
							autoCompleteIdx++
							if autoCompleteIdx >= len(candidates) {
								autoCompleteIdx = 0
							}
						}
						didAutoComplete = true
						start := len(customDir)
						if lastAutoComplete != "" {
							start -= len(lastAutoComplete)
							data.DeleteBytes(start, len(lastAutoComplete))
						} else if autoCompleteFile != "" {
							start -= len(autoCompleteFile)
							data.DeleteBytes(start, len(autoCompleteFile))
						}
						lastAutoComplete = candidates[autoCompleteIdx].(string)
						data.InsertBytes(start, []byte(lastAutoComplete))
						return 0
					},
				),
		),
		g.Dummy(0, 20),
		g.Row(
			goldButton("Continue", 200, 52, func() {
				if getChosenInstall() == nil {
					return
				}
				switchPage(pgAction)
			}),
			g.Dummy(14, 0),
			ghostButton("Back", 120, 52, func() { switchPage(pgWelcome) }),
		),
	)
	return stagePage(wh, estH, col...)
}

func actionPage(wh int) g.Widget {
	var col g.Layout
	col = append(col,
		pips(2, 5),
		g.Dummy(0, 8),
		boldText(38, colourInk, "What should I do?"),
		g.Dummy(0, 8),
		textCW(17, colourMuted, "Heads up: fully close Discord first, or Windows may refuse to touch its files."),
		g.Dummy(0, 18),
		boldText(15, colourFaint, "target: "+chosenLabel()),
		g.Dummy(0, 18),
		goldButton("Install Slipcord", 0, 60, func() { startJob(jobInstall) }),
		g.Dummy(0, 6),
		textC(14, colourFaint, "sync the latest Slipcord and slip it in"),
		g.Dummy(0, 14),
		goldButton("Repair or reinstall", 0, 60, func() { startJob(jobRepair) }),
		g.Dummy(0, 6),
		textC(14, colourFaint, "re-download Slipcord and patch it over whatever is there"),
		g.Dummy(0, 14),
		ghostButton("Remove Slipcord", 0, 60, func() { startJob(jobUninstall) }),
		g.Dummy(0, 6),
		textC(14, colourFaint, "restore the official Discord app"),
		g.Dummy(0, 14),
		ghostButton("Manage OpenAsar", 0, 60, func() { startJob(jobOpenAsar) }),
		g.Dummy(0, 6),
		textC(14, colourFaint, "an open-source replacement for Discord's core"),
		g.Dummy(0, 22),
		ghostButton("Back", 132, 52, func() { switchPage(pgPath) }),
	)
	return stagePage(wh, 760, col...)
}

func stagePage(wh int, estH float32, content ...g.Widget) g.Widget {
	ww, _ := win.GetSize()
	stageY := int(float32(wh)*0.11) + int(curRise)
	if maxH := float32(wh) - float32(stageY) - 64; estH > maxH {
		estH = maxH
	}
	return g.Custom(func() {
		g.SetCursorPos(image.Pt((ww-stageW)/2, stageY))
		stageCard(stageW, estH, content...).Build()
	})
}

func installOptionRow(name, installPath string, patched, selected bool, onClick func()) g.Widget {
	return g.Custom(func() {
		availW, _ := g.GetAvailableRegion()
		selectionStyle(selected).
			SetStyle(g.StyleVarFramePadding, 18, 14).
			SetStyleFloat(g.StyleVarFrameRounding, 16).
			To(g.Selectable(name).Selected(selected).Size(availW-28, 56).OnClick(onClick)).
			Build()
		g.Dummy(0, 6).Build()
		row := []g.Widget{mutedText(14, installPath)}
		if patched {
			row = append(row, g.Dummy(10, 0), textC(13, colourSuccess, "patched"))
		}
		g.Row(row...).Build()
		g.Dummy(0, 14).Build()
	})
}

func progressPage(wh int) g.Widget {
	title, desc, _ := jobTexts()
	frac := progressFrac()
	failed := jobDone.Load() == 1 && jobErr != nil
	titleCol := colourInk
	if failed {
		titleCol = colourDanger
	}

	var col g.Layout
	col = append(col,
		pips(3, 5),
		g.Dummy(0, 8),
		boldText(38, titleCol, title),
		g.Dummy(0, 8),
		textCW(17, colourMuted, desc),
		g.Dummy(0, 30),
		cometBar(frac, 540, 18),
		g.Dummy(0, 20),
	)
	estH := float32(380)
	if failed {
		estH = 500
		col = append(col,
			textCW(15, colourDanger, humanizeErr(jobDi, jobErr)),
			g.Dummy(0, 24),
			ghostButton("Back", 132, 48, func() { switchPage(pgAction) }),
		)
	} else {
		foot := "your files stay on this device"
		if time.Since(jobStartedAt) > 9*time.Second {
			foot = "taking a little longer than usual — still working"
		}
		col = append(col, textC(14, colourFaint, foot))
	}
	return stagePage(wh, estH, col...)
}

func donePage(wh int) g.Widget {
	_, _, sub := jobTexts()
	return g.Custom(func() {
		heroY := int(float32(wh)*0.17) + int(curRise)
		g.SetCursorPos(image.Pt(0, heroY))
		g.Align(g.AlignCenter).To(
			logo(104),
			g.Dummy(0, 22),
			boldText(44, colourGold, "Completed!"),
			g.Dummy(0, 10),
			textCW(18, colourInk, sub),
			g.Dummy(0, 46),
			pips(4, 5),
			g.Dummy(0, 38),
			g.Row(
				goldButton("Install another target", 270, 54, func() { switchPage(pgPath) }),
				g.Dummy(14, 0),
				ghostButton("Close", 140, 54, func() { os.Exit(0) }),
			),
		).Build()
	})
}

func loop() {
	if logoTexture == nil {
		// Textures need a live (*MasterWindow).Run context; giu asserts on
		// NewTextureFromRgba before that, so load on the first frame.
		initIconTexture()
	}
	curAlpha, curRise = pageAnim()

	if page == pgWelcome && time.Since(welcomeAt) > 4*time.Second {
		switchPage(pgPath)
	} else if page == pgProgress && (jobDone.Load() == 1 || jobErr != nil) {
		if jobDoneSeenAt.IsZero() {
			jobDoneSeenAt = time.Now()
		} else if time.Since(jobDoneSeenAt) > 700*time.Millisecond {
			switchPage(pgDone)
		}
	}

	if page == pgWelcome && (g.IsKeyPressed(g.KeyEnter) || g.IsKeyPressed(g.KeySpace)) {
		switchPage(pgPath)
	}
	if page == pgPath {
		if g.IsKeyPressed(g.KeyUp) && radioIdx > 0 {
			radioIdx--
		}
		if g.IsKeyPressed(g.KeyDown) && radioIdx < customChoiceIdx {
			radioIdx++
		}
	}

	g.PushWindowPadding(0, 0)
	g.PushColorWindowBg(colourSky)

	g.SingleWindow().
		Flags(g.WindowFlagsNoDecoration | g.WindowFlagsNoSavedSettings).
		Layout(wizardLayout())

	g.PopStyleColor()
	g.PopStyle()
}
