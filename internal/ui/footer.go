package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/glebovdev/somafm-cli/internal/player"
	"github.com/rivo/tview"
)

type StatusRenderer struct {
	player        *player.Player
	isMuted       bool
	animFrame     int
	maxAnimFrame  int
	tickCount     int
	ticksPerFrame int

	bufferHealth         int
	bufferTickCount      int
	bufferTicksPerUpdate int

	primaryColor string
}

func NewStatusRenderer(p *player.Player) *StatusRenderer {
	return &StatusRenderer{
		player:               p,
		maxAnimFrame:         4,
		ticksPerFrame:        8,  // Slow down animation (8 ticks per frame)
		bufferTicksPerUpdate: 16, // Update buffer ~1 per second (16 * 60ms ≈ 960ms)
	}
}

func (s *StatusRenderer) SetMuted(muted bool) {
	s.isMuted = muted
}

func (s *StatusRenderer) SetPrimaryColor(color string) {
	s.primaryColor = color
}

func (s *StatusRenderer) AdvanceAnimation() {
	s.tickCount++
	if s.tickCount >= s.ticksPerFrame {
		s.tickCount = 0
		s.animFrame = (s.animFrame + 1) % s.maxAnimFrame
	}

	s.bufferTickCount++
	if s.bufferTickCount >= s.bufferTicksPerUpdate {
		s.bufferTickCount = 0
		if s.player != nil {
			s.bufferHealth = s.player.GetBufferHealth()
		}
	}
}

func (s *StatusRenderer) Render() string {
	if s.player == nil {
		return s.renderIdle()
	}

	state := s.player.GetState()

	switch state {
	case player.StateIdle:
		return s.renderIdle()
	case player.StateBuffering:
		return s.renderBuffering()
	case player.StatePlaying:
		return s.renderPlaying()
	case player.StatePaused:
		return s.renderPaused()
	case player.StateReconnecting:
		return s.renderReconnecting()
	case player.StateError:
		return s.renderError()
	default:
		return s.renderIdle()
	}
}

func (s *StatusRenderer) renderIdle() string {
	if s.isMuted {
		return "○ IDLE │ [red]MUTED[-] │ Select a station"
	}
	return "○ IDLE │ Select a station"
}

func (s *StatusRenderer) renderBuffering() string {
	circles := []string{"◐", "◓", "◑", "◒"}
	return fmt.Sprintf("%s BUFFERING", circles[s.animFrame])
}

func (s *StatusRenderer) renderPlaying() string {
	dots := []string{"●", "◉", "○", "◉"}
	dot := dots[s.animFrame]

	if s.primaryColor != "" {
		dot = fmt.Sprintf("[%s]%s[-]", s.primaryColor, dot)
	}

	parts := []string{dot + " LIVE"}

	if s.isMuted {
		parts = append(parts, "[red]MUTED[-]")
	}

	streamInfo := s.player.GetStreamInfo()
	if streamInfo.Format != "" {
		sampleRateKHz := float64(streamInfo.SampleRate) / 1000.0
		parts = append(parts, fmt.Sprintf("%s %s %dk %.1fkHz",
			streamInfo.Format,
			qualityShort(streamInfo.Quality),
			streamInfo.Bitrate,
			sampleRateKHz))
	}

	parts = append(parts, s.formatBufferHealth(s.bufferHealth))

	return joinParts(parts)
}

func (s *StatusRenderer) renderPaused() string {
	parts := []string{PauseIcon + " PAUSED"}

	if s.isMuted {
		parts = append(parts, "[red]MUTED[-]")
	}

	streamInfo := s.player.GetStreamInfo()
	if streamInfo.Format != "" {
		sampleRateKHz := float64(streamInfo.SampleRate) / 1000.0
		parts = append(parts, fmt.Sprintf("%s %s %dk %.1fkHz",
			streamInfo.Format,
			qualityShort(streamInfo.Quality),
			streamInfo.Bitrate,
			sampleRateKHz))
	}

	return joinParts(parts)
}

func (s *StatusRenderer) renderReconnecting() string {
	current, max := s.player.GetRetryInfo()
	return fmt.Sprintf("↻ RETRY %d/%d", current, max)
}

func (s *StatusRenderer) renderError() string {
	errMsg := s.player.GetLastError()
	if errMsg == "" {
		errMsg = "ERROR"
	}
	return fmt.Sprintf("✗ %s", errMsg)
}

func (s *StatusRenderer) formatBufferHealth(percent int) string {
	signalBars := []string{"▁", "▂", "▃", "▅", "▇"}
	const numBars = 5

	filled := (percent * numBars) / 100
	if filled > numBars {
		filled = numBars
	}

	bar := ""
	for i := 0; i < numBars; i++ {
		if i < filled {
			bar += signalBars[i]
		} else {
			bar += "▁"
		}
	}

	return bar
}

func qualityShort(quality string) string {
	switch quality {
	case "highest", "high":
		return "HQ"
	case "medium":
		return "MQ"
	case "low":
		return "LQ"
	default:
		return ""
	}
}

func joinParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += " │ " + parts[i]
	}
	return result
}

func (ui *UI) getPlaybackHint(keyColor string) string {
	state := ui.player.GetState()

	switch state {
	case player.StatePaused:
		return fmt.Sprintf("[%s]Enter[-] play  [%s]Space[-] resume", keyColor, keyColor)
	case player.StatePlaying, player.StateBuffering, player.StateReconnecting:
		return fmt.Sprintf("[%s]Enter[-] play  [%s]Space[-] pause", keyColor, keyColor)
	default:
		return fmt.Sprintf("[%s]Space[-] play", keyColor)
	}
}

func (ui *UI) getHelpText() string {
	keyColor := ui.colors.helpHotkey.String()
	playbackHint := ui.getPlaybackHint(keyColor)

	muteText := "mute"
	if ui.isMuted {
		muteText = "unmute"
	}

	return fmt.Sprintf(" %s  [%s]+/-[-] vol  [%s]m[-] %s  [%s]?[-] help  [%s]a[-] about  [%s]q[-] quit ",
		playbackHint, keyColor, keyColor, muteText, keyColor, keyColor, keyColor)
}

func (ui *UI) handleFooterResize(width int) {
	isWide := width >= FooterBreakpoint
	wasWide := ui.lastFooterWidth >= FooterBreakpoint

	if ui.lastFooterWidth > 0 && isWide != wasWide && ui.contentLayout != nil {
		newHeight := FooterHeightWide
		if !isWide {
			newHeight = FooterHeightNarrow
		}
		ui.contentLayout.ResizeItem(ui.helpPanel, newHeight, 0)
	}
	ui.lastFooterWidth = width
}

func (ui *UI) drawWideFooter(screen tcell.Screen, x, y, width, height int, helpText, statusText string) {
	helpWidth := width / 2
	statusWidth := width - helpWidth

	for row := y; row < y+height; row++ {
		for col := x; col < x+helpWidth; col++ {
			screen.SetContent(col, row, ' ', nil, tcell.StyleDefault.Background(ui.colors.helpBackground))
		}
	}

	for row := y; row < y+height; row++ {
		for col := x + helpWidth; col < x+width; col++ {
			screen.SetContent(col, row, ' ', nil, tcell.StyleDefault.Background(ui.colors.background))
		}
	}

	centerY := y + height/2
	tview.Print(screen, helpText, x, centerY, helpWidth, tview.AlignCenter, ui.colors.helpForeground)
	tview.Print(screen, statusText, x+helpWidth, centerY, statusWidth-2, tview.AlignRight, ui.colors.foreground)
}

func (ui *UI) drawNarrowFooter(screen tcell.Screen, x, y, width, height int, helpText, statusText string) {
	helpHeight := height / 2
	if helpHeight < 1 {
		helpHeight = 1
	}
	statusHeight := height - helpHeight
	helpBoxEnd := y + helpHeight

	for row := y; row < helpBoxEnd; row++ {
		for col := x; col < x+width; col++ {
			screen.SetContent(col, row, ' ', nil, tcell.StyleDefault.Background(ui.colors.helpBackground))
		}
	}

	for row := helpBoxEnd; row < y+height; row++ {
		for col := x; col < x+width; col++ {
			screen.SetContent(col, row, ' ', nil, tcell.StyleDefault.Background(ui.colors.background))
		}
	}

	helpTextY := y + helpHeight/2
	tview.Print(screen, helpText, x, helpTextY, width, tview.AlignCenter, ui.colors.helpForeground)

	if statusHeight > 0 {
		statusTextY := helpBoxEnd + statusHeight/2
		tview.Print(screen, statusText, x, statusTextY, width-2, tview.AlignRight, ui.colors.foreground)
	}
}

func (ui *UI) createFooter() *tview.Box {
	box := tview.NewBox().SetBackgroundColor(ui.colors.background)

	box.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		ui.handleFooterResize(width)

		helpText := ui.getHelpText()
		statusText := " " + ui.statusRenderer.Render() + " "

		isWide := width >= FooterBreakpoint
		usedHeight := height
		if isWide && height > FooterHeightWide {
			usedHeight = FooterHeightWide
		}

		if isWide {
			ui.drawWideFooter(screen, x, y, width, usedHeight, helpText, statusText)
		} else {
			ui.drawNarrowFooter(screen, x, y, width, height, helpText, statusText)
		}

		return x, y, width, height
	})

	return box
}
