package ui

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/glebovdev/somafm-cli/internal/config"
	"github.com/glebovdev/somafm-cli/internal/player"
	"github.com/glebovdev/somafm-cli/internal/service"
	"github.com/glebovdev/somafm-cli/internal/station"
	"github.com/rivo/tview"
	"github.com/rs/zerolog/log"
)

const (
	VolumeStep            = 5
	HeaderHeight          = 3
	FooterHeightWide      = 3 // Wide: 1 row with padding (top + text + bottom)
	FooterHeightNarrow    = 6 // Narrow: 2 rows × 3 lines each
	CoverWidth            = 26
	CoverHeight           = 12
	PlayerPanelHeight     = 12
	FooterBreakpoint      = 130 // Width threshold for responsive footer
	MinLoadingDisplayTime = 1200 * time.Millisecond
	MinStatusDisplayTime  = 300 * time.Millisecond
)

// PauseIcon uses platform-specific character (Windows renders ⏸ as emoji)
var PauseIcon = func() string {
	if runtime.GOOS == "windows" {
		return "❚❚"
	}
	return "⏸"
}()

type UI struct {
	app               *tview.Application
	stationService    *service.StationService
	player            *player.Player
	currentStation    *station.Station
	stationList       *tview.Table
	helpPanel         *tview.Box
	contentLayout     *tview.Flex
	playerPanel       *tview.Flex
	currentTrackView  *tview.TextView
	logoPanel         *tview.Image
	volumeView        *tview.Flex
	mainLayout        *tview.Flex
	loadingScreen     *tview.Flex
	loadingText       *tview.TextView
	progressBar       *tview.TextView
	pages             *tview.Pages
	stopUpdates       chan struct{}
	playingIndex      int
	playingStationID  string
	selectedStationID string
	currentVolume     int
	isMuted           bool
	config            *config.Config
	startRandom       bool
	lastFooterWidth   int // Track width to detect layout changes
	mu                sync.Mutex
	animationFrame    int
	playingSpinner    *PlayingSpinner
	statusRenderer    *StatusRenderer
	colors            struct {
		background                  tcell.Color
		foreground                  tcell.Color
		borders                     tcell.Color
		highlight                   tcell.Color
		headerBackground            tcell.Color
		stationListHeaderBackground tcell.Color
		stationListHeaderForeground tcell.Color
		helpBackground              tcell.Color
		helpForeground              tcell.Color
		helpHotkey                  tcell.Color
		genreTagBackground          tcell.Color
		modalBackground             tcell.Color
	}
}

func NewUI(player *player.Player, stationService *service.StationService, startRandom bool) *UI {
	cfg, err := config.Load()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load config, using defaults")
		cfg = config.DefaultConfig()
	}

	ui := &UI{
		app:            tview.NewApplication(),
		player:         player,
		stationService: stationService,
		stopUpdates:    make(chan struct{}),
		playingIndex:   -1,
		currentVolume:  cfg.Volume,
		isMuted:        false,
		config:         cfg,
		startRandom:    startRandom,
	}

	ui.colors.background = config.GetColor(cfg.Theme.Background)
	ui.colors.foreground = config.GetColor(cfg.Theme.Foreground)
	ui.colors.borders = config.GetColor(cfg.Theme.Borders)
	ui.colors.highlight = config.GetColor(cfg.Theme.Highlight)
	ui.colors.headerBackground = config.GetColor(cfg.Theme.HeaderBackground)
	ui.colors.stationListHeaderBackground = config.GetColor(cfg.Theme.StationListHeaderBackground)
	ui.colors.stationListHeaderForeground = config.GetColor(cfg.Theme.StationListHeaderForeground)
	ui.colors.helpBackground = config.GetColor(cfg.Theme.HelpBackground)
	ui.colors.helpForeground = config.GetColor(cfg.Theme.HelpForeground)
	ui.colors.helpHotkey = config.GetColor(cfg.Theme.HelpHotkey)
	ui.colors.genreTagBackground = config.GetColor(cfg.Theme.GenreTagBackground)
	ui.colors.modalBackground = config.GetColor(cfg.Theme.ModalBackground)

	player.SetVolume(cfg.Volume)
	log.Debug().Msgf("Loaded volume from config: %d%%", cfg.Volume)

	ui.statusRenderer = NewStatusRenderer(player)
	ui.statusRenderer.SetPrimaryColor(ui.colors.highlight.String())

	return ui
}

func (ui *UI) SaveConfig() {
	ui.mu.Lock()
	if !ui.isMuted {
		ui.config.Volume = ui.currentVolume
	}
	if ui.currentStation != nil {
		ui.config.LastStation = ui.currentStation.ID
	}
	ui.mu.Unlock()

	if err := ui.config.Save(); err != nil {
		log.Error().Err(err).Msg("Failed to save config")
	}
}

func (ui *UI) safeCloseChannel() {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	if ui.stopUpdates != nil {
		select {
		case <-ui.stopUpdates:
			// Already closed
		default:
			close(ui.stopUpdates)
		}
		ui.stopUpdates = nil
	}
}

func (ui *UI) recreateStopChannel() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	ui.stopUpdates = make(chan struct{})
}

func (ui *UI) stop() {
	ui.stationService.StopPeriodicRefresh()
	ui.player.Stop()
	ui.safeCloseChannel()
	ui.app.Stop()
}

// Shutdown stops the UI gracefully from external callers (e.g., signal handlers).
func (ui *UI) Shutdown() {
	ui.app.QueueUpdateDraw(func() {
		ui.stop()
	})
}

func (ui *UI) Run() error {
	ui.setupLoadingScreen()
	ui.app.SetRoot(ui.loadingScreen, true)
	ui.configureScreen()

	go ui.initAsync()

	return ui.app.Run()
}

func (ui *UI) configureScreen() {
	bgStyle := tcell.StyleDefault.Background(ui.colors.background)
	ui.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		screen.SetStyle(bgStyle)
		screen.Clear()
		return false
	})

	var titleSet sync.Once
	ui.app.SetAfterDrawFunc(func(screen tcell.Screen) {
		titleSet.Do(func() { screen.SetTitle(config.AppName) })
	})
}

func (ui *UI) initAsync() {
	if err := ui.fetchStationsAndInitUI(); err != nil {
		ui.app.QueueUpdateDraw(func() {
			ui.handleInitialError(err)
		})
	}
}

func (ui *UI) setupLoadingScreen() {
	ui.loadingText = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("Connecting to SomaFM... (1/3)")
	ui.loadingText.SetTextColor(ui.colors.foreground).
		SetBackgroundColor(ui.colors.background)

	ui.progressBar = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(ui.renderProgressBar(0))
	ui.progressBar.SetTextColor(ui.colors.highlight).
		SetBackgroundColor(ui.colors.background)

	content := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(ui.loadingText, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(ui.progressBar, 1, 0, false)
	content.SetBackgroundColor(ui.colors.background)

	ui.loadingScreen = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(content, 3, 0, false).
		AddItem(nil, 0, 1, false)

	ui.loadingScreen.SetBackgroundColor(ui.colors.background)
}

func (ui *UI) renderProgressBar(percent int) string {
	const width = 30
	filled := (percent * width) / 100
	empty := width - filled
	return strings.Repeat("█", filled) + strings.Repeat("░", empty)
}

func (ui *UI) animateProgress(fromPercent, toPercent int, duration time.Duration) {
	steps := toPercent - fromPercent
	if steps <= 0 {
		return
	}
	stepDuration := duration / time.Duration(steps)
	lastBar := ui.renderProgressBar(fromPercent)

	for p := fromPercent + 1; p <= toPercent; p++ {
		time.Sleep(stepDuration)
		if bar := ui.renderProgressBar(p); bar != lastBar {
			ui.app.QueueUpdateDraw(func() {
				ui.progressBar.SetText(bar)
			})
			lastBar = bar
		}
	}
}

func (ui *UI) fetchStationsAndInitUI() error {
	const totalStages = 3
	stagePercent := func(stage int) int { return (stage * 100) / totalStages }

	startTime := time.Now()

	animDone := make(chan struct{})
	go func() {
		ui.animateProgress(stagePercent(0), stagePercent(1), MinStatusDisplayTime)
		close(animDone)
	}()

	_, err := ui.stationService.GetStations()
	if err != nil {
		return fmt.Errorf("failed to fetch stations: %w", err)
	}
	log.Debug().Msgf("Loaded %d stations in %v", ui.stationService.StationCount(), time.Since(startTime))

	<-animDone

	ui.app.QueueUpdateDraw(func() {
		ui.loadingText.SetText("Loading configuration... (2/3)")
	})

	ui.config.CleanupFavorites(ui.stationService.GetValidStationIDs())
	ui.SaveConfig()

	ui.animateProgress(stagePercent(1), stagePercent(2), MinStatusDisplayTime)

	ui.app.QueueUpdateDraw(func() {
		ui.loadingText.SetText("Building interface... (3/3)")
	})

	ui.setupUI()
	ui.stationService.StartPeriodicRefresh(30*time.Second, ui.onStationsRefreshed)

	ui.animateProgress(stagePercent(2), stagePercent(3), MinStatusDisplayTime)

	// Floor, not ceiling: wait only if real work finished early.
	if elapsed := time.Since(startTime); elapsed < MinLoadingDisplayTime {
		time.Sleep(MinLoadingDisplayTime - elapsed)
	}
	log.Debug().Msgf("Total loading time: %v", time.Since(startTime))

	ui.app.QueueUpdateDraw(func() {
		ui.app.SetRoot(ui.pages, true).EnableMouse(true)
		ui.app.SetFocus(ui.stationList)

		if ui.startRandom {
			ui.randomStation()
			return
		}

		if ui.config.LastStation == "" {
			ui.selectAndShowStation(0)
			return
		}

		index := ui.stationService.FindIndexByID(ui.config.LastStation)
		if index < 0 {
			log.Debug().Msgf("Last station '%s' not found, showing first station", ui.config.LastStation)
			ui.selectAndShowStation(0)
			return
		}

		if ui.config.Autostart {
			log.Debug().Msgf("Autostart enabled, playing last station: %s", ui.config.LastStation)
			ui.stationList.Select(index+1, 0)
			ui.onStationSelected(index)
		} else {
			ui.selectAndShowStation(index)
		}
	})

	return nil
}

func (ui *UI) setupUI() {
	header := ui.createHeader()

	ui.playerPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	ui.playerPanel.SetBackgroundColor(ui.colors.background)

	ui.stationList = ui.createStationListTable()

	ui.helpPanel = ui.createFooter()

	ui.contentLayout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, HeaderHeight, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(ui.playerPanel, PlayerPanelHeight, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(ui.stationList, 0, 1, true).
		AddItem(ui.helpPanel, FooterHeightWide, 0, false)
	ui.contentLayout.SetBackgroundColor(ui.colors.background)

	wrapper := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 3, 0, false).
		AddItem(ui.contentLayout, 0, 1, true).
		AddItem(nil, 3, 0, false)
	wrapper.SetBackgroundColor(ui.colors.background)

	ui.mainLayout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 1, 0, false).
		AddItem(wrapper, 0, 1, true).
		AddItem(nil, 1, 0, false)
	ui.mainLayout.SetBackgroundColor(ui.colors.background)

	ui.pages = tview.NewPages().
		AddPage("main", ui.mainLayout, true, true)
	ui.pages.SetBackgroundColor(ui.colors.background)

	ui.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if ui.pages.HasPage("modal") {
			return event
		}
		return ui.globalInputHandler(event)
	})
}

func (ui *UI) createHeader() tview.Primitive {
	titleView := tview.NewTextView()
	titleView.SetText(" " + config.AppName)
	titleView.SetTextAlign(tview.AlignLeft)
	titleView.SetTextColor(ui.colors.foreground)
	titleView.SetBackgroundColor(ui.colors.headerBackground)

	versionView := tview.NewTextView()
	versionView.SetText("v" + config.AppVersion + " ")
	versionView.SetTextAlign(tview.AlignRight)
	versionView.SetTextColor(ui.colors.foreground)
	versionView.SetBackgroundColor(ui.colors.headerBackground)

	textFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(titleView, 0, 1, false).
		AddItem(versionView, 10, 0, false)
	textFlex.SetBackgroundColor(ui.colors.headerBackground)

	topSpacer := tview.NewBox().SetBackgroundColor(ui.colors.headerBackground)
	bottomSpacer := tview.NewBox().SetBackgroundColor(ui.colors.headerBackground)
	leftSpacer := tview.NewBox().SetBackgroundColor(ui.colors.headerBackground)
	rightSpacer := tview.NewBox().SetBackgroundColor(ui.colors.headerBackground)

	textWithPadding := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftSpacer, 1, 0, false).
		AddItem(textFlex, 0, 1, false).
		AddItem(rightSpacer, 1, 0, false)
	textWithPadding.SetBackgroundColor(ui.colors.headerBackground)

	headerFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topSpacer, 1, 0, false).
		AddItem(textWithPadding, 1, 0, false).
		AddItem(bottomSpacer, 1, 0, false)
	headerFlex.SetBackgroundColor(ui.colors.headerBackground)

	return headerFlex
}

func (ui *UI) updateLogoPanel(s *station.Station) {
	go func() {
		img, err := ui.stationService.LoadImage(s.XLImage)
		if err != nil {
			ui.app.QueueUpdateDraw(func() {
				ui.logoPanel.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
					errorMsg := fmt.Sprintf("Failed to load image: %v", err)
					tview.Print(screen, errorMsg, x, y, 10, tview.AlignCenter, tcell.ColorRed)
					return x, y, 10, 10
				})
			})
			return
		}

		ui.app.QueueUpdateDraw(func() {
			ui.logoPanel.SetImage(img)
		})
	}()
}

func (ui *UI) onStationSelected(index int) {
	stationCount := ui.stationService.StationCount()
	if index < 0 || index >= stationCount {
		return
	}

	if index == ui.playingIndex && ui.player.IsPlaying() {
		return
	}

	ui.player.Stop()
	ui.safeCloseChannel()
	ui.recreateStopChannel()

	previousPlayingIndex := ui.playingIndex

	ui.playingIndex = index
	ui.currentStation = ui.stationService.GetStation(index)
	ui.playingStationID = ui.currentStation.ID

	if previousPlayingIndex >= 0 && previousPlayingIndex < stationCount && previousPlayingIndex != index {
		ui.setStationRow(ui.stationList, previousPlayingIndex+1, previousPlayingIndex)
	}

	ui.updateStationListPlayingIndicator()

	ui.SaveConfig()

	ui.playerPanel.Clear()

	contentPanel := ui.createContentPanel()
	ui.playerPanel.AddItem(contentPanel, 0, 1, false)

	ui.updateLogoPanel(ui.currentStation)

	go func() {
		stationID := ui.currentStation.ID
		track, err := ui.stationService.GetCurrentTrackForStation(stationID)
		if err != nil {
			log.Debug().Err(err).Msg("Failed to fetch song history, using lastPlaying")
			return
		}
		if track != "" {
			ui.app.QueueUpdateDraw(func() {
				if ui.currentTrackView != nil {
					ui.currentTrackView.SetText(fmt.Sprintf(" [%s]%s[-]",
						ui.colors.highlight.String(),
						track))
				}
			})
			ui.player.SetInitialTrack(track)
		}
	}()

	ui.startPlayingAnimation()

	go func() {
		log.Info().Msgf("Starting playback for station: %s", ui.currentStation.Title)
		err := ui.player.Play(ui.currentStation)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Debug().Msg("Playback stopped (station changed)")
				return
			}
			log.Error().Err(err).Msg("Failed to play station")
			ui.app.QueueUpdateDraw(func() {
				ui.showError(fmt.Sprintf("Failed to play station: %v", err))
			})
		}
	}()
}

func (ui *UI) createGenreTags(genre string) *tview.Flex {
	container := tview.NewFlex().SetDirection(tview.FlexColumn)
	container.SetBackgroundColor(ui.colors.background)

	container.AddItem(tview.NewBox().SetBackgroundColor(ui.colors.background), 1, 0, false)

	if genre == "" {
		noGenre := tview.NewTextView()
		noGenre.SetText("N/A")
		noGenre.SetTextColor(ui.colors.foreground)
		noGenre.SetBackgroundColor(ui.colors.background)
		container.AddItem(noGenre, 3, 0, false)
		return container
	}

	genres := strings.Split(genre, "|")
	for i, g := range genres {
		g = strings.TrimSpace(g)

		tag := tview.NewTextView()
		tag.SetText(" " + g + " ")
		tag.SetTextColor(ui.colors.foreground)
		tag.SetBackgroundColor(ui.colors.genreTagBackground)
		tag.SetTextAlign(tview.AlignCenter)

		tagWidth := len(g) + 2
		container.AddItem(tag, tagWidth, 0, false)

		if i < len(genres)-1 {
			spacer := tview.NewBox().SetBackgroundColor(ui.colors.background)
			container.AddItem(spacer, 1, 0, false)
		}
	}

	container.AddItem(tview.NewBox().SetBackgroundColor(ui.colors.background), 0, 1, false)

	return container
}

func (ui *UI) createContentPanel() *tview.Flex {
	ui.logoPanel = tview.NewImage()
	ui.logoPanel.SetBackgroundColor(ui.colors.background)
	ui.logoPanel.SetAlign(tview.AlignLeft, tview.AlignTop)

	stationLabel := tview.NewTextView()
	stationLabel.SetText(" Station:")
	stationLabel.SetTextColor(ui.colors.foreground)
	stationLabel.SetBackgroundColor(ui.colors.background)
	stationLabel.SetWrap(false)

	stationNameView := tview.NewTextView()
	stationNameView.SetDynamicColors(true)
	stationNameView.SetText(fmt.Sprintf(" [%s]%s[-]",
		ui.colors.highlight.String(),
		ui.currentStation.Title))
	stationNameView.SetTextColor(ui.colors.highlight)
	stationNameView.SetBackgroundColor(ui.colors.background)
	stationNameView.SetWrap(false)
	stationNameView.SetTextStyle(tcell.StyleDefault.Background(ui.colors.background).Attributes(tcell.AttrBold))

	playingLabel := tview.NewTextView()
	playingLabel.SetText(" Playing:")
	playingLabel.SetTextColor(ui.colors.foreground)
	playingLabel.SetBackgroundColor(ui.colors.background)
	playingLabel.SetWrap(false)

	ui.currentTrackView = tview.NewTextView()
	ui.currentTrackView.SetDynamicColors(true)
	ui.currentTrackView.SetText(fmt.Sprintf(" [%s]%s[-]",
		ui.colors.highlight.String(),
		ui.currentStation.LastPlaying))
	ui.currentTrackView.SetTextColor(ui.colors.highlight)
	ui.currentTrackView.SetBackgroundColor(ui.colors.background)
	ui.currentTrackView.SetWrap(true)
	ui.currentTrackView.SetTextStyle(tcell.StyleDefault.Background(ui.colors.background).Attributes(tcell.AttrBold))

	genreLabel := tview.NewTextView()
	genreLabel.SetText(" Genre:")
	genreLabel.SetTextColor(ui.colors.foreground)
	genreLabel.SetBackgroundColor(ui.colors.background)
	genreLabel.SetWrap(false)

	genreView := ui.createGenreTags(ui.currentStation.Genre)

	descriptionLabel := tview.NewTextView()
	descriptionLabel.SetText(" Description:")
	descriptionLabel.SetTextColor(ui.colors.foreground)
	descriptionLabel.SetBackgroundColor(ui.colors.background)
	descriptionLabel.SetWrap(false)

	descriptionView := tview.NewTextView()
	descriptionView.SetDynamicColors(true)
	descriptionView.SetText(fmt.Sprintf(" [%s]%s[-]",
		ui.colors.foreground.String(),
		ui.currentStation.Description))
	descriptionView.SetTextColor(ui.colors.foreground)
	descriptionView.SetBackgroundColor(ui.colors.background)
	descriptionView.SetWrap(true)

	infoSpacer := tview.NewBox().SetBackgroundColor(ui.colors.background)

	infoContent := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(stationLabel, 1, 0, false).
		AddItem(stationNameView, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(playingLabel, 1, 0, false).
		AddItem(ui.currentTrackView, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(genreLabel, 1, 0, false).
		AddItem(genreView, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(descriptionLabel, 1, 0, false).
		AddItem(descriptionView, 0, 1, false).
		AddItem(infoSpacer, 0, 1, false)
	infoContent.SetBackgroundColor(ui.colors.background)

	ui.volumeView = ui.createGraphicalVolumeBar()

	// Wrap logo in vertical flex to constrain height
	logoWrapper := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.logoPanel, CoverHeight, 0, false).
		AddItem(nil, 0, 1, false)
	logoWrapper.SetBackgroundColor(ui.colors.background)

	contentFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(logoWrapper, CoverWidth, 0, false).
		AddItem(infoContent, 0, 1, false).
		AddItem(ui.volumeView, 7, 0, false)
	contentFlex.SetBackgroundColor(ui.colors.background)

	contentWithPadding := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 4, 0, false).
		AddItem(contentFlex, 0, 1, false).
		AddItem(nil, 4, 0, false)
	contentWithPadding.SetBackgroundColor(ui.colors.background)

	return contentWithPadding
}

type PlayingSpinner struct {
	Frames []string
	FPS    time.Duration
}

func NewPlayingSpinner() *PlayingSpinner {
	return &PlayingSpinner{
		Frames: []string{"⣾ ", "⣽ ", "⣻ ", "⢿ ", "⡿ ", "⣟ ", "⣯ ", "⣷ "},
		FPS:    time.Second / 10,
	}
}

func (ui *UI) getPlayingIndicator() string {
	if ui.playingSpinner == nil {
		ui.playingSpinner = NewPlayingSpinner()
	}

	frameIndex := ui.animationFrame % len(ui.playingSpinner.Frames)
	return ui.playingSpinner.Frames[frameIndex]
}

func (ui *UI) startPlayingAnimation() {
	if ui.playingSpinner == nil {
		ui.playingSpinner = NewPlayingSpinner()
	}

	go func() {
		animationTicker := time.NewTicker(ui.playingSpinner.FPS)
		trackUpdateTicker := time.NewTicker(5 * time.Second)
		defer animationTicker.Stop()
		defer trackUpdateTicker.Stop()

		for {
			select {
			case <-ui.stopUpdates:
				return
			case <-animationTicker.C:
				ui.mu.Lock()
				ui.animationFrame++
				ui.mu.Unlock()

				ui.statusRenderer.AdvanceAnimation()

				ui.app.QueueUpdateDraw(func() {
					ui.updateStationListPlayingIndicator()
				})
			case <-trackUpdateTicker.C:
				ui.app.QueueUpdateDraw(func() {
					ui.updateTrackInfo()
				})
			}
		}
	}()
}

func (ui *UI) updateTrackInfo() {
	if ui.currentTrackView == nil || !ui.player.IsPlaying() {
		return
	}

	trackInfo := ui.player.GetCurrentTrack()
	ui.currentTrackView.SetText(fmt.Sprintf(" [%s]%s[-]",
		ui.colors.highlight.String(),
		trackInfo))
}

func (ui *UI) onStationsRefreshed(stations []station.Station) {
	ui.app.QueueUpdateDraw(func() {
		ui.refreshStationTable()
	})
}

func (ui *UI) globalInputHandler(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyRune:
		switch event.Rune() {
		case 'q', 'Q':
			ui.stop()
			return nil
		case ' ':
			if ui.player.IsPlaying() || ui.player.IsPaused() {
				ui.player.TogglePause()
				ui.updateStationListPlayingIndicator()
			} else {
				row, _ := ui.stationList.GetSelection()
				if row > 0 && row <= ui.stationService.StationCount() {
					ui.onStationSelected(row - 1)
				}
			}
			return nil
		case '>':
			ui.nextStation()
			return nil
		case '<':
			ui.prevStation()
			return nil
		case 'r', 'R':
			ui.randomStation()
			return nil
		case 'f', 'F':
			ui.toggleFavorite()
			return nil
		case '+', '=':
			ui.adjustVolume(VolumeStep)
			return nil
		case '-', '_':
			ui.adjustVolume(-VolumeStep)
			return nil
		case 'm', 'M':
			ui.toggleMute()
			return nil
		case '?':
			ui.showHelpModal()
			return nil
		case 'a', 'A':
			ui.showAboutModal()
			return nil
		}
	case tcell.KeyEnter:
		row, _ := ui.stationList.GetSelection()
		if row > 0 && row <= ui.stationService.StationCount() {
			ui.onStationSelected(row - 1)
		}
		return nil
	case tcell.KeyEscape:
		ui.stop()
		return nil
	case tcell.KeyRight:
		// Right arrow - volume up (hidden shortcut)
		ui.adjustVolume(VolumeStep)
		return nil
	case tcell.KeyLeft:
		// Left arrow - volume down (hidden shortcut)
		ui.adjustVolume(-VolumeStep)
		return nil
	}
	return event
}
