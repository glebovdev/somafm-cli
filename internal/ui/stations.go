package ui

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rs/zerolog/log"
)

func (ui *UI) createStationListTable() *tview.Table {
	table := tview.NewTable().
		SetBorders(false).
		SetSeparator(' ').
		SetSelectable(true, false).
		SetFixed(1, 0)

	table.SetBorder(true).
		SetTitle(fmt.Sprintf("Stations (%d)", ui.stationService.StationCount())).
		SetBorderColor(ui.colors.borders).
		SetTitleColor(ui.colors.foreground).
		SetBackgroundColor(ui.colors.background).
		SetBorderPadding(1, 0, 1, 1)

	table.SetSelectedStyle(tcell.StyleDefault.
		Foreground(ui.colors.background).
		Background(ui.colors.highlight))

	table.SetCell(0, 0, tview.NewTableCell(" ").
		SetTextColor(ui.colors.stationListHeaderForeground).
		SetBackgroundColor(ui.colors.stationListHeaderBackground).
		SetMaxWidth(2).
		SetSelectable(false))

	table.SetCell(0, 1, tview.NewTableCell(" ").
		SetTextColor(ui.colors.stationListHeaderForeground).
		SetBackgroundColor(ui.colors.stationListHeaderBackground).
		SetMaxWidth(2).
		SetSelectable(false))

	table.SetCell(0, 2, tview.NewTableCell("Name").
		SetTextColor(ui.colors.stationListHeaderForeground).
		SetBackgroundColor(ui.colors.stationListHeaderBackground).
		SetExpansion(1).
		SetSelectable(false))

	table.SetCell(0, 3, tview.NewTableCell("Genre").
		SetTextColor(ui.colors.stationListHeaderForeground).
		SetBackgroundColor(ui.colors.stationListHeaderBackground).
		SetExpansion(1).
		SetSelectable(false))

	table.SetCell(0, 4, tview.NewTableCell("Listeners").
		SetTextColor(ui.colors.stationListHeaderForeground).
		SetBackgroundColor(ui.colors.stationListHeaderBackground).
		SetAlign(tview.AlignRight).
		SetSelectable(false))

	stationCount := ui.stationService.StationCount()
	for i := 0; i < stationCount; i++ {
		ui.setStationRow(table, i+1, i)
	}

	// Track selected station ID for preserving selection after refresh
	table.SetSelectionChangedFunc(func(row, column int) {
		count := ui.stationService.StationCount()
		if row > 0 && row <= count {
			if s := ui.stationService.GetStation(row - 1); s != nil {
				ui.selectedStationID = s.ID
			}
		}
	})

	return table
}

func (ui *UI) setStationRow(table *tview.Table, row int, stationIndex int) {
	s := ui.stationService.GetStation(stationIndex)
	if s == nil {
		return
	}

	favIcon := " "
	if ui.config.IsFavorite(s.ID) {
		favIcon = "★"
	}
	table.SetCell(row, 0, tview.NewTableCell(favIcon).
		SetTextColor(ui.colors.foreground).
		SetMaxWidth(2))

	playIcon := " "
	if stationIndex == ui.playingIndex {
		if ui.player.IsPaused() {
			playIcon = "⏸"
		} else {
			playIcon = "➤"
		}
	}
	table.SetCell(row, 1, tview.NewTableCell(playIcon).
		SetTextColor(ui.colors.foreground).
		SetMaxWidth(2))

	table.SetCell(row, 2, tview.NewTableCell(s.Title).
		SetTextColor(ui.colors.foreground).
		SetMaxWidth(35).
		SetExpansion(2))

	genreText := strings.ReplaceAll(s.Genre, "|", ", ")
	table.SetCell(row, 3, tview.NewTableCell(genreText).
		SetTextColor(ui.colors.foreground).
		SetMaxWidth(27).
		SetExpansion(1))

	table.SetCell(row, 4, tview.NewTableCell(s.Listeners).
		SetTextColor(ui.colors.foreground).
		SetAlign(tview.AlignRight))
}

func (ui *UI) nextStation() {
	stationCount := ui.stationService.StationCount()
	if stationCount == 0 {
		return
	}

	row, _ := ui.stationList.GetSelection()
	currentIndex := row - 1
	nextIndex := (currentIndex + 1) % stationCount
	ui.stationList.Select(nextIndex+1, 0)
	ui.onStationSelected(nextIndex)
}

func (ui *UI) prevStation() {
	stationCount := ui.stationService.StationCount()
	if stationCount == 0 {
		return
	}

	row, _ := ui.stationList.GetSelection()
	currentIndex := row - 1
	prevIndex := currentIndex - 1
	if prevIndex < 0 {
		prevIndex = stationCount - 1
	}
	ui.stationList.Select(prevIndex+1, 0)
	ui.onStationSelected(prevIndex)
}

func (ui *UI) randomStation() {
	stationCount := ui.stationService.StationCount()
	if stationCount == 0 {
		return
	}

	randomIndex := rand.IntN(stationCount)
	ui.stationList.Select(randomIndex+1, 0)
	ui.onStationSelected(randomIndex)
}

func (ui *UI) selectAndShowStation(index int) {
	stationCount := ui.stationService.StationCount()
	if stationCount == 0 || index < 0 || index >= stationCount {
		return
	}

	ui.currentStation = ui.stationService.GetStation(index)

	ui.stationList.Select(index+1, 0)

	ui.playerPanel.Clear()
	contentPanel := ui.createContentPanel()
	ui.playerPanel.AddItem(contentPanel, 0, 1, false)

	ui.updateLogoPanel(ui.currentStation)

	log.Debug().Msgf("Showing station info (without playing): %s", ui.currentStation.Title)
}

func (ui *UI) selectAndShowStationByID(stationID string) bool {
	index := ui.stationService.FindIndexByID(stationID)
	if index < 0 {
		log.Debug().Msgf("Last played station '%s' not found in station list", stationID)
		return false
	}

	ui.selectAndShowStation(index)
	if s := ui.stationService.GetStation(index); s != nil {
		log.Debug().Msgf("Auto-selected last played station: %s", s.Title)
	}
	return true
}

func (ui *UI) toggleFavorite() {
	row, _ := ui.stationList.GetSelection()
	stationCount := ui.stationService.StationCount()
	if row <= 0 || row > stationCount {
		return
	}

	stationIndex := row - 1
	selectedStation := ui.stationService.GetStation(stationIndex)
	if selectedStation == nil {
		return
	}

	ui.config.ToggleFavorite(selectedStation.ID)

	favCell := ui.stationList.GetCell(row, 0)
	if favCell != nil {
		if ui.config.IsFavorite(selectedStation.ID) {
			favCell.SetText("★")
		} else {
			favCell.SetText(" ")
		}
	}

	go func() {
		if err := ui.config.Save(); err != nil {
			log.Error().Err(err).Msg("Failed to save config")
		}
	}()

	log.Debug().Msgf("Toggled favorite for station: %s", selectedStation.Title)
}

func (ui *UI) refreshStationTable() {
	stationCount := ui.stationService.StationCount()

	// Stations may have been re-sorted, so update index by ID
	if ui.playingStationID != "" {
		newIndex := ui.stationService.FindIndexByID(ui.playingStationID)
		if newIndex >= 0 {
			ui.playingIndex = newIndex
		}
	}

	for i := 0; i < stationCount; i++ {
		ui.setStationRow(ui.stationList, i+1, i)
	}

	if ui.selectedStationID != "" {
		newIndex := ui.stationService.FindIndexByID(ui.selectedStationID)
		if newIndex >= 0 {
			ui.stationList.Select(newIndex+1, 0)
		}
	}

	ui.stationList.SetTitle(fmt.Sprintf("Stations (%d)", stationCount))

	log.Debug().Int("count", stationCount).Msg("Station table refreshed")
}

func (ui *UI) updateStationListPlayingIndicator() {
	stationCount := ui.stationService.StationCount()
	if ui.playingIndex < 0 || ui.playingIndex >= stationCount {
		return
	}

	if !ui.player.IsPlaying() && !ui.player.IsPaused() {
		return
	}

	row := ui.playingIndex + 1
	s := ui.stationService.GetStation(ui.playingIndex)
	if s == nil {
		return
	}

	playCell := ui.stationList.GetCell(row, 1)
	if playCell != nil {
		if ui.player.IsPaused() {
			playCell.SetText("⏸")
		} else {
			playCell.SetText("➤")
		}
	}

	nameCell := ui.stationList.GetCell(row, 2)
	if nameCell == nil {
		return
	}

	name := s.Title
	indicator := ui.getPlayingIndicator()

	const maxNameWidth = 35
	maxLen := maxNameWidth - len(indicator) - 1
	if len(name) > maxLen {
		name = name[:maxLen-3] + "..."
	}

	nameText := name + " " + indicator
	nameCell.SetText(nameText)
}
