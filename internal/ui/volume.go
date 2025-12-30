package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/glebovdev/somafm-cli/internal/config"
	"github.com/rivo/tview"
	"github.com/rs/zerolog/log"
)

func (ui *UI) buildVolumeBar(container *tview.Flex) {
	const barHeight = 10

	ui.mu.Lock()
	displayVolume := ui.currentVolume
	isMuted := ui.isMuted
	if isMuted {
		displayVolume = ui.config.Volume
	}
	ui.mu.Unlock()

	filledLines := (displayVolume * barHeight) / 100
	emptyLines := barHeight - filledLines

	createText := func(text string, color tcell.Color) *tview.TextView {
		tv := tview.NewTextView()
		tv.SetText(text)
		tv.SetTextAlign(tview.AlignRight)
		tv.SetTextColor(color)
		tv.SetBackgroundColor(ui.colors.background)
		return tv
	}

	createBarLine := func(barText string, barColor tcell.Color, showPercent bool) *tview.Flex {
		line := tview.NewFlex().SetDirection(tview.FlexColumn)
		line.SetBackgroundColor(ui.colors.background)

		if showPercent {
			percentText := fmt.Sprintf("%d%%", displayVolume)

			var percentColor tcell.Color
			if isMuted {
				percentColor = config.GetColor(ui.config.Theme.MutedVolume)
			} else {
				percentColor = ui.colors.highlight
			}

			percentView := createText(percentText, percentColor)
			percentView.SetTextAlign(tview.AlignRight)

			if isMuted {
				percentView.SetTextStyle(tcell.StyleDefault.
					Foreground(percentColor).
					Background(ui.colors.background).
					Attributes(tcell.AttrStrikeThrough))
			}

			line.AddItem(percentView, 4, 0, false)
		} else {
			line.AddItem(createText("    ", ui.colors.foreground), 4, 0, false)
		}

		line.AddItem(createText(barText, barColor), 0, 1, false)

		return line
	}

	container.AddItem(createText("   max", ui.colors.foreground), 1, 0, false)

	for i := 0; i < emptyLines; i++ {
		container.AddItem(createBarLine(" ░░", ui.colors.foreground, false), 1, 0, false)
	}

	barColor := ui.colors.highlight
	if isMuted {
		barColor = config.GetColor(ui.config.Theme.MutedVolume)
	}
	for i := 0; i < filledLines; i++ {
		showPercent := (i == 0)
		container.AddItem(createBarLine(" ██", barColor, showPercent), 1, 0, false)
	}

	container.AddItem(createText("   min", ui.colors.foreground), 1, 0, false)

	container.AddItem(nil, 0, 1, false)
}

func (ui *UI) createGraphicalVolumeBar() *tview.Flex {
	volumeContainer := tview.NewFlex().SetDirection(tview.FlexRow)
	volumeContainer.SetBackgroundColor(ui.colors.background)
	ui.buildVolumeBar(volumeContainer)
	return volumeContainer
}

func (ui *UI) updateVolumeDisplay() {
	if ui.volumeView != nil {
		ui.volumeView.Clear()
		ui.buildVolumeBar(ui.volumeView)
	}
}

func (ui *UI) adjustVolume(delta int) {
	ui.mu.Lock()

	if ui.isMuted {
		ui.currentVolume = ui.config.Volume
		ui.isMuted = false
		ui.statusRenderer.SetMuted(false)
		ui.mu.Unlock()

		ui.player.SetVolume(ui.currentVolume)
		ui.updateVolumeDisplay()
		log.Debug().Msgf("Auto-unmuted, restored volume to %d%%", ui.currentVolume)
		return
	}

	ui.currentVolume = config.ClampVolume(ui.currentVolume + delta)
	ui.mu.Unlock()

	ui.player.SetVolume(ui.currentVolume)
	ui.updateVolumeDisplay()
	ui.SaveConfig()
	log.Debug().Msgf("Volume adjusted to %d%%", ui.currentVolume)
}

func (ui *UI) toggleMute() {
	ui.mu.Lock()
	if ui.isMuted {
		ui.currentVolume = ui.config.Volume
		ui.isMuted = false
		log.Debug().Msgf("Unmuted, restored volume to %d%%", ui.currentVolume)
	} else {
		if ui.currentVolume == 0 {
			ui.config.Volume = config.DefaultVolume
		} else {
			ui.config.Volume = ui.currentVolume
		}
		ui.currentVolume = 0
		ui.isMuted = true
		log.Debug().Msgf("Muted, saved volume %d%%", ui.config.Volume)
	}
	ui.statusRenderer.SetMuted(ui.isMuted)
	ui.mu.Unlock()

	ui.player.SetVolume(ui.currentVolume)
	ui.updateVolumeDisplay()
	ui.SaveConfig()
}
