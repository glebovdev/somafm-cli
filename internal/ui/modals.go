package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/glebovdev/somafm-cli/internal/config"
	"github.com/rivo/tview"
)

func friendlyErrorMessage(errStr string) string {
	if strings.Contains(errStr, "no such host") {
		return "Unable to connect to server.\nPlease check your internet connection."
	}
	if strings.Contains(errStr, "connection refused") {
		return "Connection refused by server.\nThe service may be temporarily unavailable."
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return "Connection timed out.\nPlease check your internet connection."
	}
	if strings.Contains(errStr, "network is unreachable") || strings.Contains(errStr, "network read error") {
		return "Network is unreachable.\nPlease check your internet connection."
	}
	if strings.Contains(errStr, "status 401") {
		return "Stream access denied (401)."
	}
	if strings.Contains(errStr, "status 403") {
		return "Stream access forbidden (403)."
	}
	if strings.Contains(errStr, "status 404") {
		return "Stream not found (404)."
	}

	if idx := strings.Index(errStr, ": dial"); idx > 0 {
		return errStr[:idx]
	}
	if len(errStr) > 100 {
		return errStr[:100] + "..."
	}
	return errStr
}

func (ui *UI) showError(err error) {
	ui.showPlaybackErrorModal(friendlyErrorMessage(err.Error()))
}

func (ui *UI) showPlaybackErrorModal(message string) {
	doDismiss := func() {
		ui.pages.RemovePage("error-modal")
		ui.app.SetFocus(ui.stationList)
	}

	doRetry := func() {
		ui.pages.RemovePage("error-modal")
		ui.app.SetFocus(ui.stationList)
		if ui.currentStation != nil {
			ui.startPlayingAnimation()
			go func() {
				err := ui.player.Play(ui.currentStation)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					ui.app.QueueUpdateDraw(func() {
						ui.showError(err)
					})
				}
			}()
		}
	}

	messageView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(fmt.Sprintf("\n[::b]Playback Error[::-]\n\n%s", message))
	messageView.SetTextColor(ui.colors.foreground)
	messageView.SetBackgroundColor(ui.colors.modalBackground)

	hintView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText("[::d]Press [::b]R[::d] to retry  •  Press [::b]Esc[::d] to dismiss[::-]")
	hintView.SetTextColor(tcell.ColorDarkGray)
	hintView.SetBackgroundColor(ui.colors.modalBackground)

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(messageView, 0, 1, false).
		AddItem(hintView, 1, 0, false).
		AddItem(nil, 1, 0, false)
	content.SetBackgroundColor(ui.colors.modalBackground)

	frame := tview.NewFrame(content).
		SetBorders(0, 0, 1, 1, 1, 1)
	frame.SetBorder(true).
		SetBorderColor(ui.colors.highlight).
		SetBackgroundColor(ui.colors.modalBackground).
		SetTitle(" Error ").
		SetTitleColor(ui.colors.highlight).
		SetTitleAlign(tview.AlignCenter)

	modalWidth := 50
	modalHeight := 10

	lines := strings.Count(message, "\n") + 1
	if lines > 2 {
		modalHeight += lines - 2
	}
	if modalHeight > 15 {
		modalHeight = 15
	}

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(frame, modalHeight, 0, true).
			AddItem(nil, 0, 1, false),
			modalWidth, 0, true).
		AddItem(nil, 0, 1, false)
	modal.SetBackgroundColor(ui.colors.background)

	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape, tcell.KeyEnter:
			doDismiss()
			return nil
		case tcell.KeyRune:
			if event.Rune() == 'r' || event.Rune() == 'R' {
				doRetry()
				return nil
			}
		}
		return event
	})

	ui.pages.AddPage("error-modal", modal, true, true)
	ui.app.SetFocus(modal)
}

func (ui *UI) showHelpModal() {
	keyColor := ui.colors.helpHotkey.String()

	configPath, _ := config.GetConfigPath()

	helpText := fmt.Sprintf(`[::b]KEYBOARD SHORTCUTS[::-]

[%s]PLAYBACK[-]
  [%s]Enter[-]      Play selected station
  [%s]Space[-]      Pause / Resume
  [%s]<[-]          Previous station
  [%s]>[-]          Next station
  [%s]r[-]          Random station

[%s]VOLUME[-]
  [%s]+[-] / [%s]-[-]      Volume up / down
  [%s]←[-] / [%s]→[-]      Volume up / down
  [%s]m[-]          Mute / Unmute

[%s]STATIONS[-]
  [%s]↑[-] / [%s]↓[-]      Navigate list
  [%s]f[-]          Toggle favorite

[%s]APPLICATION[-]
  [%s]?[-]          Show this help
  [%s]a[-]          About %s
  [%s]q[-] / [%s]Esc[-]    Quit

[%s]CONFIG[-]: %s`,
		keyColor,
		keyColor, keyColor, keyColor, keyColor, keyColor,
		keyColor,
		keyColor, keyColor, keyColor, keyColor, keyColor,
		keyColor,
		keyColor, keyColor, keyColor,
		keyColor,
		keyColor, keyColor, config.AppName, keyColor, keyColor,
		keyColor, configPath)

	ui.showInfoModal("Help", helpText)
}

func (ui *UI) showAboutModal() {
	doDismiss := func() {
		ui.pages.RemovePage("modal")
		ui.app.SetFocus(ui.stationList)
	}

	linkColor := "skyblue"
	dimColor := "gray"

	aboutText := fmt.Sprintf(`[::b]%s[::-]
[%s]%s[-]

Version: %s
Author:  %s ([%s:::%s]%s[-:::-])
Project: [%s:::%s]%s[-:::-]
License: MIT

───────────────────────────────────────────

[%s]Radio content from[-] [::b]SomaFM[::-]
Listener-supported • [%s:::%s]%s[-:::-]`,
		config.AppName,
		dimColor, config.AppTagline,
		config.AppVersion,
		config.AppAuthor, linkColor, config.AppAuthorURL, config.AppAuthorURLShort,
		linkColor, config.AppProjectURL, config.AppProjectShort,
		dimColor,
		linkColor, config.AppDonateURL, config.AppDonateShort)

	messageView := tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true).
		SetText("\n" + aboutText)
	messageView.SetTextColor(ui.colors.foreground)
	messageView.SetBackgroundColor(ui.colors.modalBackground)

	hintView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText("[::d]Press any key to close[::-]")
	hintView.SetTextColor(tcell.ColorDarkGray)
	hintView.SetBackgroundColor(ui.colors.modalBackground)

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(messageView, 0, 1, false).
		AddItem(nil, 2, 0, false).
		AddItem(hintView, 1, 0, false).
		AddItem(nil, 1, 0, false)
	content.SetBackgroundColor(ui.colors.modalBackground)

	frame := tview.NewFrame(content).
		SetBorders(1, 0, 1, 1, 2, 2)
	frame.SetBorder(true).
		SetBorderColor(ui.colors.borders).
		SetBackgroundColor(ui.colors.modalBackground).
		SetTitle(" About ").
		SetTitleColor(ui.colors.highlight).
		SetTitleAlign(tview.AlignCenter)

	modalWidth := 50
	modalHeight := 20

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(frame, modalHeight, 0, true).
			AddItem(nil, 0, 1, false),
			modalWidth, 0, true).
		AddItem(nil, 0, 1, false)
	modal.SetBackgroundColor(ui.colors.background)

	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		doDismiss()
		return nil
	})

	ui.pages.AddPage("modal", modal, true, true)
	ui.app.SetFocus(modal)
}

func (ui *UI) showInfoModal(title, message string) {
	doDismiss := func() {
		ui.pages.RemovePage("modal")
		ui.app.SetFocus(ui.stationList)
	}

	messageView := tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true).
		SetWordWrap(true).
		SetText("\n" + message)
	messageView.SetTextColor(ui.colors.foreground)
	messageView.SetBackgroundColor(ui.colors.modalBackground)

	hintView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText("[::d]Press any key to close[::-]")
	hintView.SetTextColor(tcell.ColorDarkGray)
	hintView.SetBackgroundColor(ui.colors.modalBackground)

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(messageView, 0, 1, false).
		AddItem(nil, 2, 0, false).
		AddItem(hintView, 1, 0, false).
		AddItem(nil, 1, 0, false)
	content.SetBackgroundColor(ui.colors.modalBackground)

	frame := tview.NewFrame(content).
		SetBorders(1, 0, 1, 1, 2, 2)
	frame.SetBorder(true).
		SetBorderColor(ui.colors.borders).
		SetBackgroundColor(ui.colors.modalBackground).
		SetTitle(" " + title + " ").
		SetTitleColor(ui.colors.highlight).
		SetTitleAlign(tview.AlignCenter)

	lines := strings.Count(message, "\n") + 1
	modalWidth := 45
	modalHeight := lines + 10
	if modalHeight > 38 {
		modalHeight = 38
	}

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(frame, modalHeight, 0, true).
			AddItem(nil, 0, 1, false),
			modalWidth, 0, true).
		AddItem(nil, 0, 1, false)
	modal.SetBackgroundColor(ui.colors.background)

	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		doDismiss()
		return nil
	})

	ui.pages.AddPage("modal", modal, true, true)
	ui.app.SetFocus(modal)
}

func (ui *UI) showInitialErrorScreen(title, message string, onRetry, onQuit func()) {
	content := fmt.Sprintf("[::b]%s[::-]\n\n%s", title, message)

	textView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText(content)
	textView.SetTextColor(ui.colors.foreground)
	textView.SetBackgroundColor(ui.colors.modalBackground)

	helpText := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText("[::d]Press [::b]R[::d] to retry  •  Press [::b]Q[::d] to quit[::-]")
	helpText.SetTextColor(ui.colors.foreground)
	helpText.SetBackgroundColor(ui.colors.background)

	frame := tview.NewFrame(textView).
		SetBorders(2, 2, 2, 2, 2, 2)
	frame.SetBorder(true).
		SetBorderColor(ui.colors.highlight).
		SetBackgroundColor(ui.colors.modalBackground).
		SetTitle(" Connection Error ").
		SetTitleColor(ui.colors.highlight)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(frame, 60, 1, true).
			AddItem(nil, 0, 1, false), 10, 1, true).
		AddItem(helpText, 2, 0, false).
		AddItem(nil, 0, 1, false)
	layout.SetBackgroundColor(ui.colors.background)

	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'r', 'R':
				if onRetry != nil {
					onRetry()
				}
				return nil
			case 'q', 'Q':
				if onQuit != nil {
					onQuit()
				}
				return nil
			}
		case tcell.KeyEscape:
			if onQuit != nil {
				onQuit()
			}
			return nil
		}
		return event
	})

	ui.app.SetRoot(layout, true)
	ui.app.SetFocus(layout)
}

func (ui *UI) handleInitialError(err error) {
	friendlyMsg := friendlyErrorMessage(err.Error())

	ui.showInitialErrorScreen(
		"Unable to Load Stations",
		friendlyMsg,
		func() { // onRetry
			ui.app.SetRoot(ui.loadingScreen, true)
			go func() {
				if err := ui.fetchStationsAndInitUI(); err != nil {
					ui.app.QueueUpdateDraw(func() {
						ui.handleInitialError(err)
					})
				}
			}()
		},
		func() { // onQuit
			ui.app.Stop()
		},
	)
}
