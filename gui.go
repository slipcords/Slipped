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
	"image/color"
	"slipped/buildinfo"

	g "github.com/AllenDang/giu"
	"github.com/AllenDang/imgui-go"

	// png decoder for icon
	_ "image/png"
	"os"
	path "path/filepath"
	"runtime"
	"strconv"
	"strings"
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

	win = g.NewMasterWindow("Slipped", 1200, 800, linuxFlags)

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

func handlePatch() {
	choice := getChosenInstall()
	if choice != nil {
		choice.Patch()
	}
}

func handleUnpatch() {
	choice := getChosenInstall()
	if choice != nil {
		choice.Unpatch()
	}
}

func handleOpenAsar() {
	if acceptedOpenAsar || getChosenInstall().IsOpenAsar() {
		handleOpenAsarConfirmed()
		return
	}

	g.OpenPopup("#openasar-confirm")
}

func handleOpenAsarConfirmed() {
	choice := getChosenInstall()
	if choice != nil {
		if choice.IsOpenAsar() {
			if err := choice.UninstallOpenAsar(); err != nil {
				handleErr(choice, err, "uninstall OpenAsar from")
			} else {
				g.OpenPopup("#openasar-unpatched")
				g.Update()
			}
		} else {
			if err := choice.InstallOpenAsar(); err != nil {
				handleErr(choice, err, "install OpenAsar on")
			} else {
				g.OpenPopup("#openasar-patched")
				g.Update()
			}
		}
	}
}

func handleErr(di *DiscordInstall, err error, action string) {
	if errors.Is(err, ErrAlreadyReported) {
		return
	}
	if errors.Is(err, os.ErrPermission) {
		switch runtime.GOOS {
		case "windows":
			err = errors.New("Permission denied. Make sure your Discord is fully closed (from the tray)!")
		case "darwin":
			// FIXME: This text is not selectable which is a bit mehhh
			command := "sudo chown -R \"${USER}:wheel\" " + di.path
			err = errors.New("Permission denied. Please grant the installer Full Disk Access in the system settings (privacy & security page).\n\nIf that also doesn't work, try running the following command in your terminal:\n" + command)
		case "linux":
			command := "sudo chown -R \"$USER:$USER\" " + di.path
			err = errors.New("Permission denied. Try to run the installer with sudo privileges.\n\nIf that also doesn't work, try running the following command in your terminal:\n" + command)
		default:
			err = errors.New("Permission denied. Maybe try running me as Administrator/Root?")
		}
	}

	ShowModal("Failed to "+action+" this Install", err.Error())
}

func HandleScuffedInstall() {
	g.OpenPopup("#scuffed-install")
}

func (di *DiscordInstall) Patch() {
	if CheckScuffedInstall() {
		return
	}
	if err := di.patch(); err != nil {
		handleErr(di, err, "patch")
	} else {
		g.OpenPopup("#patched")
	}
}

func (di *DiscordInstall) Unpatch() {
	if err := di.unpatch(); err != nil {
		handleErr(di, err, "unpatch")
	} else {
		g.OpenPopup("#unpatched")
	}
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

// go can you give me []any?
// to pass to giu RangeBuilder?
// yeeeeees
// actually returns []string like a boss
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
	return g.Style().
		SetStyle(g.StyleVarWindowPadding, 30, 30).
		SetStyleFloat(g.StyleVarWindowRounding, 12).
		To(
			g.PopupModal(id).
				Flags(g.WindowFlagsNoTitleBar | Ternary(isDynamic, g.WindowFlagsAlwaysAutoResize, 0)).
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
		SetStyle(g.StyleVarWindowPadding, 30, 30).
		SetStyleFloat(g.StyleVarWindowRounding, 12).
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

// renderHeader draws the brand lockup (wordmark + tagline) and, right-aligned,
// the installer version with an outdated notice when one is available.
func renderHeader() g.Widget {
	var rightMeta g.Widget = mutedText(14, "Installer "+buildinfo.InstallerTag+" ("+buildinfo.InstallerGitHash+")")
	if IsSelfOutdated {
		rightMeta = g.Column(
			rightMeta,
			g.Style().SetFontSize(13).SetColor(g.StyleColorText, colourWarning).
				To(g.Label("A new installer version is available")),
		)
	}

	return g.Row(
		g.Column(
			boldText(38, "Slipped"),
			g.Style().SetFontSize(14).SetColor(g.StyleColorText, colourMuted).
				To(g.Label("the Slipcord installer")),
		),
		g.Align(g.AlignRight).To(rightMeta),
	)
}

// renderSecurityBanner replaces the old full-bleed yellow warning with a calm,
// hairline amber card that carries the same message without shouting.
func renderSecurityBanner(width float32) g.Widget {
	msg := "Only **GitHub** and **github.com/Slipcords/Slipped** are the official places to get Slipcord. Any other site claiming to be us is malicious.\n" +
		"If you downloaded from any other source, delete or uninstall everything from it immediately, run a malware scan, and change your Discord password."
	return g.Style().
		SetColor(g.StyleColorChildBg, colourWarningDim).
		SetColor(g.StyleColorBorder, colourWarning).
		SetStyle(g.StyleVarWindowPadding, 18, 14).
		SetStyleFloat(g.StyleVarChildBorderSize, 1).
		SetStyleFloat(g.StyleVarChildRounding, 10).
		To(
			g.Child().Border(true).Size(width, 98).Layout(
				g.Column(
					g.Style().SetFont(sliptFontBold).SetFontSize(15).SetColor(g.StyleColorText, colourWarning).
						To(g.Label("Security notice")),
					g.Dummy(0, 4),
					g.Markdown(&msg),
				),
			),
		)
}

// installOptionRow is one selectable target: a blurple-highlighted headline row
// with a success-coloured badge and the install path underneath.
func installOptionRow(name, installPath string, patched, selected bool, onClick func()) g.Widget {
	return g.Custom(func() {
		availW, _ := g.GetAvailableRegion()
		headlineStyle := selectionStyle(selected).
			SetStyle(g.StyleVarFramePadding, 14, 10).
			SetStyleFloat(g.StyleVarFrameRounding, 8)

		if patched {
			g.Row(
				headlineStyle.To(
					g.Selectable(name).Selected(selected).Size(availW-84, 0).OnClick(onClick),
				),
				g.Style().SetFontSize(13).SetColor(g.StyleColorText, colourSuccess).
					To(g.Label("PATCHED")),
			).Build()
		} else {
			headlineStyle.To(
				g.Selectable(name).Selected(selected).Size(availW, 0).OnClick(onClick),
			).Build()
		}

		g.Dummy(0, 3).Build()
		mutedText(13, installPath).Build()
		g.Dummy(0, 4).Build()
	})
}

func metaLine(key, value string, warn bool) g.Widget {
	return g.Row(
		mutedText(14, key),
		g.Dummy(10, 0),
		g.Style().
			SetFontSize(14).
			SetColor(g.StyleColorText, Ternary(warn, colourWarning, colourText)).
			To(g.Label(value)),
	)
}

func renderInstaller() g.Widget {
	candidates := makeAutoComplete()
	if len(candidates) > 6 {
		candidates = candidates[:6]
	}
	wi, hi := win.GetSize()
	w := float32(wi) - 80
	contentH := float32(hi) - 60

	mainH := contentH - 280
	if mainH < 220 {
		mainH = 220
	}

	var currentDiscord *DiscordInstall
	if radioIdx != customChoiceIdx {
		currentDiscord = discords[radioIdx].(*DiscordInstall)
	}
	var isOpenAsar = currentDiscord != nil && currentDiscord.IsOpenAsar()

	if CanUpdateSelf() && !showedUpdatePrompt {
		showedUpdatePrompt = true
		g.OpenPopup("#update-prompt")
	}

	// left pane: install targets
	var leftCol g.Layout
	if len(discords) == 0 {
		s := "No Discord installs found. You first need to install Discord."
		if runtime.GOOS == "linux" {
			s += " snap is not supported."
		}
		leftCol = append(leftCol, mutedText(15, s))
	}
	for i, v := range discords {
		d := v.(*DiscordInstall)
		leftCol = append(leftCol, installOptionRow(strings.Title(d.branch), d.path, d.isPatched, radioIdx == i, makeRadioOnChange(i)))
	}
	leftCol = append(leftCol,
		installOptionRow("Custom Install Location", Ternary(customDir != "", customDir, "Select a folder"), false, radioIdx == customChoiceIdx, makeRadioOnChange(customChoiceIdx)),
		g.Dummy(0, 6),
		inputBoxStyle().To(
			g.InputText(&customDir).Hint("The custom location").
				Flags(g.InputTextFlagsCallbackCompletion).
				OnChange(onCustomInputChanged).
				// this library has its own autocomplete but it's broken
				Callback(
					func(data imgui.InputTextCallbackData) int32 {
						if len(candidates) == 0 {
							return 0
						}
						// just wrap around
						if autoCompleteIdx >= len(candidates) {
							autoCompleteIdx = 0
						}

						// used by change handler
						didAutoComplete = true

						start := len(customDir)
						// Delete previous auto complete
						if lastAutoComplete != "" {
							start -= len(lastAutoComplete)
							data.DeleteBytes(start, len(lastAutoComplete))
						} else if autoCompleteFile != "" { // delete partial input
							start -= len(autoCompleteFile)
							data.DeleteBytes(start, len(autoCompleteFile))
						}

						// Insert auto complete
						lastAutoComplete = candidates[autoCompleteIdx].(string)
						data.InsertBytes(start, []byte(lastAutoComplete))
						autoCompleteIdx++

						return 0
					},
				),
		),
	)
	for _, c := range candidates {
		leftCol = append(leftCol, mutedText(13, c))
	}

	// right pane: status
	dirLine := "Slipcord will be downloaded to:"
	if IsDevInstall {
		dirLine = "Dev Install:"
	}
	var rightCol g.Layout
	rightCol = append(rightCol,
		sectionTitle("Status"),
		g.Dummy(0, 10),
		g.Style().SetFontSize(16).To(g.Label(dirLine)),
		g.Style().SetFontSize(16).To(g.Label(SlipcordDirectory).Wrapped(true)),
		g.Dummy(0, 10),
		g.Style().
			SetColor(g.StyleColorButton, colourSurfaceAlt).
			SetColor(g.StyleColorButtonHovered, colourSurfaceHover).
			SetColor(g.StyleColorButtonActive, colourSurfaceAlt).
			SetStyle(g.StyleVarFramePadding, 12, 8).
			SetStyleFloat(g.StyleVarFrameRounding, 8).
			To(
				g.Button("Open Directory").OnClick(func() {
					g.OpenURL("file://" + path.Dir(SlipcordDirectory))
				}).Size(150, 32),
			),
		&CondWidget{!IsDevInstall, func() g.Widget {
			return g.Style().SetFontSize(13).SetColor(g.StyleColorText, colourMuted).To(
				g.Label("To customise this location, set the environment variable 'SLIPCORD_USER_DATA_DIR' and restart me").Wrapped(true),
			)
		}, nil},
		g.Dummy(0, 14),
		g.Style().SetColor(g.StyleColorSeparator, colourLine).To(g.Separator()),
		g.Dummy(0, 8),
		metaLine("Installer", buildinfo.InstallerTag+" ("+buildinfo.InstallerGitHash+")", IsSelfOutdated),
		metaLine("Local Slipcord", Ternary(InstalledHash == "", "None", shortHash(InstalledHash)), false),
		&CondWidget{
			GithubError == nil,
			func() g.Widget {
				if IsDevInstall {
					return metaLine("Latest", "not updating (DevMode)", false)
				}
				return metaLine("Latest Slipcord", shortHash(LatestHash), false)
			}, func() g.Widget {
				return renderErrorCard(DiscordRed, "Failed to fetch Info from GitHub: "+GithubError.Error(), 40)
			},
		},
	)

	mainRow := g.Row(
		paneCard(w*0.60, mainH, leftCol...),
		g.Dummy(12, 0),
		paneCard(w*0.38, mainH, rightCol...),
	)

	bw := (w - 36) / 4
	openAsarBg := colourSuccess
	openAsarHover := colourSuccessHover
	openAsarActive := colourSuccessDim
	if isOpenAsar {
		openAsarBg = colourDanger
		openAsarHover = colourDangerHover
		openAsarActive = colourDangerDk
	}
	actionBar := g.Row(
		actionButton("Install", colourAccent, colourAccentHover, colourAccentDim, "Patch the selected Discord Install", GithubError != nil, bw, 46, handlePatch),
		actionButton("Reinstall / Repair", colourSurfaceAlt, colourSurfaceHover, colourSurfaceAlt, "Reinstall & Update Slipcord", GithubError != nil, bw, 46, func() {
			if IsDevInstall {
				handlePatch()
			} else {
				err := InstallLatestBuilds()
				if err == nil {
					handlePatch()
				}
			}
		}),
		actionButton("Uninstall", colourDanger, colourDangerHover, colourDangerDk, "Unpatch the selected Discord Install", false, bw, 46, handleUnpatch),
		actionButton(Ternary(isOpenAsar, "Uninstall OpenAsar", Ternary(currentDiscord != nil, "Install OpenAsar", "(Un-)Install OpenAsar")), openAsarBg, openAsarHover, openAsarActive, "Manage OpenAsar", false, bw, 46, handleOpenAsar),
	)

	return g.Layout{
		renderHeader(),
		g.Dummy(0, 8),
		g.Style().SetColor(g.StyleColorSeparator, colourLine).To(g.Separator()),
		g.Dummy(0, 16),
		renderSecurityBanner(w),
		g.Dummy(0, 16),
		mainRow,
		g.Dummy(0, 14),
		actionBar,

		InfoModal("#patched", "Successfully Patched", "If Discord is still open, fully close it first.\n"+
			"Then, start it and verify Slipcord installed successfully by looking for its category in Discord Settings"),
		InfoModal("#unpatched", "Successfully Unpatched", "If Discord is still open, fully close it first. Then start it again, it should be back to stock!"),
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
			"To install OpenAsar, press Accept and click 'Install OpenAsar' again.", true),
		InfoModal("#openasar-patched", "Successfully Installed OpenAsar", "If Discord is still open, fully close it first. Then start it again and verify OpenAsar installed successfully!"),
		InfoModal("#openasar-unpatched", "Successfully Uninstalled OpenAsar", "If Discord is still open, fully close it first. Then start it again and it should be back to stock!"),
		InfoModal("#invalid-custom-location", "Invalid Location", "The specified location is not a valid Discord install.\nMake sure you select the base folder.\n\nHint: Discord snap is not supported. use flatpak or .deb"),
		InfoModal("#modal"+strconv.Itoa(modalId), modalTitle, modalMessage),

		UpdateModal(),
	}
}

func renderErrorCard(col color.Color, message string, height float32) g.Widget {
	return g.Style().
		SetColor(g.StyleColorChildBg, colourDangerDim).
		SetColor(g.StyleColorBorder, col).
		SetColor(g.StyleColorText, col).
		SetStyle(g.StyleVarWindowPadding, 12, 10).
		SetStyleFloat(g.StyleVarChildBorderSize, 1).
		SetStyleFloat(g.StyleVarChildRounding, 10).
		To(
			g.Child().Border(true).Size(g.Auto, height).Layout(
				g.Row(
					g.Markdown(&message),
				),
			),
		)
}

func loop() {
	g.PushWindowPadding(40, 30)
	g.PushColorWindowBg(colourBg)

	g.SingleWindow().
		RegisterKeyboardShortcuts(
			g.WindowShortcut{Key: g.KeyUp, Callback: func() {
				if radioIdx > 0 {
					radioIdx--
				}
			}},
			g.WindowShortcut{Key: g.KeyDown, Callback: func() {
				if radioIdx < customChoiceIdx {
					radioIdx++
				}
			}},
		).
		Layout(renderInstaller())

	g.PopStyle()
}
