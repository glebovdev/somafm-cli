package player

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/glebovdev/somafm-cli/internal/config"
	"github.com/glebovdev/somafm-cli/internal/station"
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
	"github.com/rs/zerolog/log"
)

const (
	DefaultSampleRate   = beep.SampleRate(44100)
	SpeakerBufferSize   = time.Millisecond * 250
	NetworkReadSize     = 4096
	SampleChannelSize   = 8192
	MaxRetries          = 3
	RetryDelay          = time.Second * 2
	VolumeCurveExponent = 0.5
	MinVolumeDB         = -10.0
	ReadTimeout         = 10 * time.Second
	MaxErrorsToKeep     = 10 // Limit error accumulation during retries
)

type PlayerState int

const (
	StateIdle PlayerState = iota
	StateBuffering
	StatePlaying
	StatePaused
	StateReconnecting
	StateError
)

func (s PlayerState) String() string {
	switch s {
	case StateIdle:
		return "IDLE"
	case StateBuffering:
		return "BUFFERING"
	case StatePlaying:
		return "LIVE"
	case StatePaused:
		return "PAUSED"
	case StateReconnecting:
		return "RECONNECTING"
	case StateError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// StreamInfo contains metadata about the current audio stream.
type StreamInfo struct {
	Format     string
	Quality    string
	Bitrate    int
	SampleRate int
}

// contextReader wraps a reader with context-aware timeout detection.
// When a read blocks longer than the timeout, it returns an error
// without leaking goroutines (relies on context cancellation for cleanup).
type contextReader struct {
	reader  io.Reader
	ctx     context.Context
	timeout time.Duration
}

func (cr *contextReader) Read(p []byte) (n int, err error) {
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
	}

	timer := time.NewTimer(cr.timeout)
	defer timer.Stop()

	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)

	go func() {
		n, err := cr.reader.Read(p)
		select {
		case done <- result{n, err}:
		case <-cr.ctx.Done():
		}
	}()

	select {
	case res := <-done:
		return res.n, res.err
	case <-timer.C:
		return 0, fmt.Errorf("read timeout: no data received for %v", cr.timeout)
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	}
}

// Player handles audio streaming and playback for SomaFM radio stations.
// It manages the audio pipeline including network streaming, decoding, buffering,
// and volume control.
type Player struct {
	format        beep.Format
	volume        *effects.Volume
	ctrl          *beep.Ctrl
	mu            sync.Mutex
	cancelFunc    context.CancelFunc
	isPaused      bool
	isPlaying     bool
	speakerInit   bool
	volumePercent int
	httpClient    *http.Client // Reused for all stream connections

	buffer         [][2]float64
	bufferSize     int
	writeIdx       int64
	readBackOffset int
	bufferMu       sync.Mutex
	sampleCh       chan [2]float64
	wg             sync.WaitGroup
	streamDone     chan struct{}
	streamDoneOnce sync.Once // Prevents double-close panic on streamDone
	streamErr      chan error

	currentTrack string
	trackMu      sync.RWMutex

	state        PlayerState
	streamInfo   StreamInfo
	retryAttempt int
	maxRetries   int
	sessionStart time.Time
	lastError    string
	stateMu      sync.RWMutex

	currentStation *station.Station
	streamAlive    bool
	streamAliveMu  sync.RWMutex
}

// closeStreamDone safely closes the streamDone channel exactly once.
// This prevents panics from double-close when multiple goroutines try to signal completion.
func (p *Player) closeStreamDone() {
	p.streamDoneOnce.Do(func() {
		if p.streamDone != nil {
			close(p.streamDone)
		}
	})
}

// NewPlayer creates a new Player with the specified buffer size in seconds.
func NewPlayer(bufferSeconds int) *Player {
	var buffer [][2]float64
	if bufferSeconds > 0 {
		bufferLen := int(DefaultSampleRate) * bufferSeconds
		buffer = make([][2]float64, bufferLen)
		log.Debug().Msgf("Initialized circular buffer: %d seconds (%d samples, ~%.2f MB)",
			bufferSeconds, bufferLen, float64(bufferLen*2*8)/1000000)
	}

	// Create a reusable HTTP client with appropriate settings for streaming
	httpClient := &http.Client{
		Timeout: 0, // No timeout for streaming connections
		Transport: &http.Transport{
			DisableKeepAlives:  false,
			MaxIdleConns:       10,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: true, // Audio streams are already compressed
		},
	}

	return &Player{
		format: beep.Format{
			SampleRate:  DefaultSampleRate,
			NumChannels: 2,
			Precision:   2,
		},
		speakerInit:   false,
		isPaused:      false,
		isPlaying:     false,
		volumePercent: -1,
		httpClient:    httpClient,
		buffer:        buffer,
		bufferSize:    bufferSeconds,
		currentTrack:  "",
	}
}

func (p *Player) initSpeaker(sampleRate beep.SampleRate) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.speakerInit || sampleRate != p.format.SampleRate {
		err := speaker.Init(sampleRate, sampleRate.N(SpeakerBufferSize))
		if err != nil {
			return fmt.Errorf("failed to initialize speaker: %w", err)
		}
		p.format.SampleRate = sampleRate
		p.speakerInit = true
		log.Debug().Msgf("Speaker initialized with sample rate: %d Hz, buffer: %v", sampleRate, SpeakerBufferSize)
	}
	return nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
	}

	speaker.Clear()
	p.isPlaying = false
	p.isPaused = false

	p.streamAliveMu.Lock()
	p.streamAlive = false
	p.streamAliveMu.Unlock()

	p.stateMu.Lock()
	p.state = StateIdle
	p.sessionStart = time.Time{}
	p.streamInfo = StreamInfo{}
	p.stateMu.Unlock()

	log.Debug().Msg("Playback stopped")
}

func (p *Player) TogglePause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl == nil || !p.isPlaying {
		return
	}

	speaker.Lock()
	p.ctrl.Paused = !p.ctrl.Paused
	p.isPaused = p.ctrl.Paused
	speaker.Unlock()

	if p.isPaused {
		p.stateMu.Lock()
		p.state = StatePaused
		p.stateMu.Unlock()
		log.Debug().Msg("Playback paused")
	} else {
		p.stateMu.Lock()
		p.state = StatePlaying
		p.stateMu.Unlock()
		log.Debug().Msg("Playback resumed")
	}
}

func (p *Player) SetVolume(volumePercent int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.volumePercent = volumePercent

	if p.volume == nil {
		log.Debug().Msgf("Volume stored as %d%% (will be applied when playback starts)", volumePercent)
		return
	}

	volumeLevel := percentToExponent(float64(volumePercent))

	speaker.Lock()
	p.volume.Volume = volumeLevel
	p.volume.Silent = volumePercent == 0
	speaker.Unlock()

	log.Debug().Msgf("Volume set to %d%% (%.2f dB)", volumePercent, volumeLevel)
}

func percentToExponent(p float64) float64 {
	if p <= 0 {
		return MinVolumeDB
	}
	if p >= 100 {
		return 0
	}

	normalized := p / 100.0
	adjusted := math.Pow(normalized, VolumeCurveExponent)
	return (1.0 - adjusted) * MinVolumeDB
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPlaying && !p.isPaused
}

func (p *Player) IsPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPaused
}

func (p *Player) GetCurrentTrack() string {
	p.trackMu.RLock()
	defer p.trackMu.RUnlock()

	if p.currentTrack == "" {
		return "Waiting for track info..."
	}
	return p.currentTrack
}

func (p *Player) GetCurrentStation() *station.Station {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentStation
}

func (p *Player) setCurrentTrack(track string) {
	p.trackMu.Lock()
	defer p.trackMu.Unlock()

	if track != p.currentTrack {
		p.currentTrack = track
		log.Debug().Msgf("Now playing: %s", track)
	}
}

func (p *Player) SetInitialTrack(track string) {
	p.trackMu.Lock()
	defer p.trackMu.Unlock()

	// Don't overwrite ICY metadata if already set
	if p.currentTrack == "" {
		p.currentTrack = track
		log.Debug().Msgf("Initial track set from songs API: %s", track)
	}
}

func (p *Player) GetState() PlayerState {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.state
}

func (p *Player) setState(state PlayerState) {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.state != state {
		log.Debug().Msgf("Player state: %s -> %s", p.state.String(), state.String())
		p.state = state
	}
}

func (p *Player) GetStreamInfo() StreamInfo {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.streamInfo
}

func (p *Player) setStreamInfo(info StreamInfo) {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	p.streamInfo = info
	log.Debug().Msgf("Stream info: %s %dk %dHz", info.Format, info.Bitrate, info.SampleRate)
}

func (p *Player) GetBufferFillPercent() int {
	if len(p.buffer) == 0 {
		return 0
	}

	p.bufferMu.Lock()
	defer p.bufferMu.Unlock()

	bufferLen := int64(len(p.buffer))
	fillLevel := p.writeIdx
	if fillLevel > bufferLen {
		fillLevel = bufferLen
	}

	return int((fillLevel * 100) / bufferLen)
}

// GetBufferHealth returns the current buffer fill level as a percentage (0-100).
func (p *Player) GetBufferHealth() int {
	p.mu.Lock()
	ch := p.sampleCh
	p.mu.Unlock()

	if ch == nil {
		return 0
	}

	channelLen := len(ch)
	channelCap := cap(ch)

	if channelCap == 0 {
		return 0
	}

	return (channelLen * 100) / channelCap
}

func (p *Player) GetRetryInfo() (current, max int) {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.retryAttempt, p.maxRetries
}

func (p *Player) setRetryInfo(current, max int) {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	p.retryAttempt = current
	p.maxRetries = max
}

func (p *Player) GetSessionDuration() time.Duration {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()

	if p.sessionStart.IsZero() {
		return 0
	}
	return time.Since(p.sessionStart)
}

func (p *Player) startSession() {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	p.sessionStart = time.Now()
}

func (p *Player) GetLastError() string {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.lastError
}

func (p *Player) setLastError(err string) {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	p.lastError = err
}

func (p *Player) Play(s *station.Station) error {
	return p.PlayWithRetry(s, MaxRetries)
}

func (p *Player) PlayWithRetry(s *station.Station, maxRetries int) error {
	playlistURLs := s.GetAllPlaylistURLs()
	if len(playlistURLs) == 0 {
		p.setState(StateError)
		p.setLastError("No playlists available")
		return fmt.Errorf("no playlists available for station: %s", s.Title)
	}

	p.setState(StateBuffering)
	p.setRetryInfo(0, maxRetries)

	allErrors := make([]string, 0, MaxErrorsToKeep)
	totalAttempts := 0

	addError := func(msg string) {
		if len(allErrors) < MaxErrorsToKeep {
			allErrors = append(allErrors, msg)
		}
	}

	for playlistIdx, playlistURL := range playlistURLs {
		log.Debug().Msgf("Trying playlist %d/%d: %s", playlistIdx+1, len(playlistURLs), playlistURL)

		streamInfo := parseStreamInfoFromURL(playlistURL)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		streamURLs, err := p.fetchAndParsePLS(ctx, playlistURL)
		cancel()

		if err != nil {
			log.Warn().Err(err).Msgf("Failed to fetch playlist: %s", playlistURL)
			addError(fmt.Sprintf("playlist %s: %v", playlistURL, err))
			continue
		}

		log.Debug().Msgf("Found %d stream URLs in playlist", len(streamURLs))

		for urlIdx, streamURL := range streamURLs {
			for attempt := 1; attempt <= maxRetries; attempt++ {
				totalAttempts++

				if attempt > 1 {
					p.setState(StateReconnecting)
					p.setRetryInfo(attempt, maxRetries)
				}

				log.Debug().Msgf("Trying stream %d/%d (attempt %d/%d): %s",
					urlIdx+1, len(streamURLs), attempt, maxRetries, streamURL)

				ctx, cancel := context.WithCancel(context.Background())

				p.mu.Lock()
				if p.cancelFunc != nil {
					p.cancelFunc()
				}
				p.cancelFunc = cancel
				p.mu.Unlock()

				p.setStreamInfo(streamInfo)

				err := p.playStreamURL(ctx, s, streamURL)
				if err == nil {
					return nil
				}

				if errors.Is(err, context.Canceled) {
					return context.Canceled
				}

				if isNonRetryableError(err) {
					log.Warn().Err(err).Msgf("Non-retryable error for %s, moving to next URL", streamURL)
					addError(fmt.Sprintf("%s: %v", streamURL, err))
					break
				}

				if isNetworkDownError(err) {
					log.Warn().Err(err).Msg("Network appears to be down, stopping retries")
					p.setState(StateError)
					p.setLastError("Network connection lost")
					return fmt.Errorf("network connection lost: %w", err)
				}

				addError(fmt.Sprintf("%s (attempt %d): %v", streamURL, attempt, err))

				if attempt < maxRetries {
					log.Warn().Err(err).Msgf("Stream failed, retrying in %v...", RetryDelay)
					time.Sleep(RetryDelay)
				}
			}
		}
	}

	p.setState(StateError)
	p.setLastError("Connection failed")
	return fmt.Errorf("playback failed after %d total attempts across all streams. Errors: %s",
		totalAttempts, strings.Join(allErrors, "; "))
}

func isNonRetryableError(err error) bool {
	errStr := err.Error()
	// HTTP errors that won't change with retry on this specific URL
	return strings.Contains(errStr, "status 401") ||
		strings.Contains(errStr, "status 403") ||
		strings.Contains(errStr, "status 404") ||
		strings.Contains(errStr, "status 410")
}

func isNetworkDownError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no route to host") ||
		strings.Contains(errStr, "network is down") ||
		strings.Contains(errStr, "DNS lookup failed") ||
		strings.Contains(errStr, "read timeout")
}

func (p *Player) playStreamURL(ctx context.Context, s *station.Station, streamURL string) error {
	speaker.Clear()
	p.setCurrentTrack("")

	log.Debug().Msgf("Connecting to stream: %s", streamURL)

	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", fmt.Sprintf("SomaFM-CLI/%s", config.AppVersion))
	req.Header.Set("Icy-MetaData", "1")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch MP3 stream: %w", err)
	}

	log.Debug().Msgf("Stream response status: %d, Content-Type: %s", resp.StatusCode, resp.Header.Get("Content-Type"))

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("stream returned status %d: %s", resp.StatusCode, resp.Status)
	}

	var icyMetaint int
	if val := resp.Header.Get("icy-metaint"); val != "" {
		_, _ = fmt.Sscanf(val, "%d", &icyMetaint)
		log.Debug().Msgf("ICY metadata interval: %d bytes", icyMetaint)
	}

	pipeReader, pipeWriter := io.Pipe()

	p.mu.Lock()
	p.sampleCh = make(chan [2]float64, SampleChannelSize)
	p.streamDone = make(chan struct{})
	p.streamDoneOnce = sync.Once{} // Reset for new stream
	p.streamErr = make(chan error, 1)
	p.mu.Unlock()

	if len(p.buffer) > 0 {
		p.bufferMu.Lock()
		for i := range p.buffer {
			p.buffer[i] = [2]float64{0, 0}
		}
		p.writeIdx = 0
		p.readBackOffset = 0
		p.bufferMu.Unlock()
	}

	// Use context-aware reader for timeout detection
	// The goroutine in contextReader will exit when context is cancelled
	timeoutBody := &contextReader{
		reader:  resp.Body,
		ctx:     ctx,
		timeout: ReadTimeout,
	}

	p.wg.Add(1)
	go p.readNetworkStream(ctx, resp.Body, timeoutBody, pipeWriter, icyMetaint)

	log.Debug().Msg("Decoding MP3 stream...")
	streamer, format, err := mp3.Decode(pipeReader)
	if err != nil {
		pipeReader.Close()
		pipeWriter.Close()
		resp.Body.Close()
		return fmt.Errorf("failed to decode MP3 stream: %w", err)
	}

	log.Debug().Msgf("Initializing audio output (sample rate: %d Hz)...", format.SampleRate)
	if err := p.initSpeaker(format.SampleRate); err != nil {
		streamer.Close()
		pipeReader.Close()
		pipeWriter.Close()
		resp.Body.Close()
		return fmt.Errorf("failed to initialize audio output: %w", err)
	}

	p.mu.Lock()
	p.format = format
	p.mu.Unlock()

	p.streamAliveMu.Lock()
	p.streamAlive = true
	p.streamAliveMu.Unlock()

	p.mu.Lock()
	p.currentStation = s
	p.mu.Unlock()

	p.wg.Add(1)
	go p.decodeAndBuffer(ctx, streamer, pipeReader, resp.Body)

	p.mu.Lock()
	volumePercent := p.volumePercent
	if volumePercent < 0 {
		volumePercent = config.DefaultVolume
	}
	volumeLevel := percentToExponent(float64(volumePercent))

	bufferedStreamer := &bufferedStreamerWrapper{player: p}

	p.volume = &effects.Volume{
		Streamer: bufferedStreamer,
		Base:     2,
		Volume:   volumeLevel,
		Silent:   volumePercent == 0,
	}

	p.ctrl = &beep.Ctrl{
		Streamer: p.volume,
		Paused:   false,
	}
	p.isPlaying = true
	p.isPaused = false
	p.mu.Unlock()

	speaker.Play(p.ctrl)

	p.setState(StatePlaying)
	p.startSession()

	p.stateMu.Lock()
	p.streamInfo.SampleRate = int(format.SampleRate)
	p.stateMu.Unlock()

	log.Debug().Msgf("Now playing: %s (buffer: %ds)", s.Title, p.bufferSize)

	select {
	case <-ctx.Done():
		speaker.Clear()
		p.mu.Lock()
		p.isPlaying = false
		p.isPaused = false
		p.mu.Unlock()

		p.closeStreamDone()
		p.wg.Wait()

		return ctx.Err()
	case err := <-p.streamErr:
		speaker.Clear()
		p.mu.Lock()
		p.isPlaying = false
		p.isPaused = false
		p.mu.Unlock()

		p.closeStreamDone()
		p.wg.Wait()

		return fmt.Errorf("stream error: %w", err)
	case <-p.streamDone:
		p.mu.Lock()
		p.isPlaying = false
		p.isPaused = false
		p.mu.Unlock()
		return fmt.Errorf("stream ended unexpectedly")
	}
}

func (p *Player) readNetworkStream(ctx context.Context, respBody io.ReadCloser, bodyReader io.Reader, pipeWriter *io.PipeWriter, icyMetaint int) {
	defer func() {
		respBody.Close()
		pipeWriter.Close()
		p.wg.Done()
		log.Debug().Msg("Network stream reader stopped")
	}()

	reportError := func(err error) {
		select {
		case p.streamErr <- err:
		default:
			// Channel full or closed, error already reported
		}
	}

	chunkSize := int64(icyMetaint)
	if chunkSize == 0 {
		chunkSize = NetworkReadSize
	}

	bufReader := bufio.NewReader(bodyReader)

	for {
		select {
		case <-ctx.Done():
			log.Debug().Msg("Network reader context cancelled")
			return
		case <-p.streamDone:
			return
		default:
			_, err := io.CopyN(pipeWriter, bufReader, chunkSize)
			if err != nil {
				// Don't log errors during intentional shutdown (station switch)
				if ctx.Err() != nil || errors.Is(err, io.ErrClosedPipe) || strings.Contains(err.Error(), "closed pipe") {
					return
				}
				if err != io.EOF {
					log.Error().Err(err).Msg("Error reading audio data from stream")
					reportError(fmt.Errorf("network read error: %w", err))
				}
				return
			}

			if icyMetaint > 0 {
				metaLenByte, err := bufReader.ReadByte()
				if err != nil {
					if ctx.Err() != nil || err == io.EOF {
						return
					}
					log.Error().Err(err).Msg("Error reading metadata length")
					reportError(fmt.Errorf("metadata read error: %w", err))
					return
				}

				metaLen := int(metaLenByte) * 16
				if metaLen > 0 {
					metaData := make([]byte, metaLen)
					n, err := io.ReadFull(bufReader, metaData)
					if err != nil {
						if ctx.Err() != nil {
							return
						}
						log.Error().Err(err).Msg("Error reading metadata content")
						reportError(fmt.Errorf("metadata content error: %w", err))
						return
					}

					metaStr := string(metaData[:n])
					if strings.Contains(metaStr, "StreamTitle='") {
						start := strings.Index(metaStr, "StreamTitle='") + len("StreamTitle='")
						end := strings.Index(metaStr[start:], "';")
						if end > 0 {
							title := metaStr[start : start+end]
							p.setCurrentTrack(title)
						}
					}
				}
			}
		}
	}
}

func (p *Player) decodeAndBuffer(ctx context.Context, streamer beep.StreamSeekCloser, pipeReader *io.PipeReader, respBody io.ReadCloser) {
	defer func() {
		streamer.Close()
		pipeReader.Close()
		close(p.sampleCh)
		p.wg.Done()

		p.streamAliveMu.Lock()
		p.streamAlive = false
		p.streamAliveMu.Unlock()

		log.Debug().Msg("Decoder and buffer goroutine stopped")

		if ctx.Err() == nil {
			p.mu.Lock()
			station := p.currentStation
			stationID := ""
			if station != nil {
				stationID = station.ID
			}
			shouldReconnect := p.isPlaying && !p.isPaused
			p.mu.Unlock()

			if shouldReconnect && station != nil {
				log.Info().Msg("Stream ended unexpectedly, auto-reconnecting...")
				go func() {
					p.setState(StateReconnecting)
					p.Stop()

					// Verify station hasn't changed during reconnect
					p.mu.Lock()
					currentStation := p.currentStation
					stationChanged := currentStation == nil || currentStation.ID != stationID
					p.mu.Unlock()

					if stationChanged {
						log.Debug().Msg("Station changed during reconnect, aborting")
						return
					}

					if err := p.Play(station); err != nil {
						log.Error().Err(err).Msg("Auto-reconnect failed")
						p.setState(StateError)
						p.setLastError("Reconnection failed")
					}
				}()
			}
		}
	}()

	decodedSamples := make([][2]float64, 4096)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.streamDone:
			return
		default:
			n, ok := streamer.Stream(decodedSamples)
			if !ok {
				if err := streamer.Err(); err != nil {
					log.Error().Err(err).Msg("Stream decoding error")
				}
				return
			}

			for i := 0; i < n; i++ {
				sample := decodedSamples[i]

				select {
				case <-ctx.Done():
					return
				case <-p.streamDone:
					return
				case p.sampleCh <- sample:
					if len(p.buffer) > 0 {
						p.bufferMu.Lock()
						idx := p.writeIdx % int64(len(p.buffer))
						p.buffer[idx] = sample
						p.writeIdx++
						p.bufferMu.Unlock()
					}
				}
			}
		}
	}
}

type bufferedStreamerWrapper struct {
	player *Player
}

func (b *bufferedStreamerWrapper) Stream(samples [][2]float64) (n int, ok bool) {
	p := b.player
	i := 0

	if len(p.buffer) > 0 {
		i = b.readFromBuffer(samples)
		if i == len(samples) {
			return i, true
		}
	}

	return b.readFromChannel(samples, i)
}

func (b *bufferedStreamerWrapper) readFromBuffer(samples [][2]float64) int {
	p := b.player
	i := 0

	p.bufferMu.Lock()
	defer p.bufferMu.Unlock()

	for p.readBackOffset < 0 && i < len(samples) {
		bufLen := int64(len(p.buffer))
		idx := (p.writeIdx + int64(p.readBackOffset) + bufLen) % bufLen

		if idx >= p.writeIdx {
			break
		}

		samples[i] = p.buffer[idx]
		p.readBackOffset++
		i++
	}

	return i
}

func (b *bufferedStreamerWrapper) readFromChannel(samples [][2]float64, startIdx int) (n int, ok bool) {
	p := b.player
	i := startIdx

	for i < len(samples) {
		select {
		case sample, more := <-p.sampleCh:
			if !more {
				return i, i > 0
			}
			samples[i] = sample
			i++
		case <-p.streamDone:
			return i, i > 0
		}
	}

	return len(samples), len(samples) > 0
}

func (b *bufferedStreamerWrapper) Err() error {
	return nil
}

func (p *Player) fetchAndParsePLS(ctx context.Context, plsURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", plsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create PLS request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PLS file: %w", err)
	}
	defer resp.Body.Close()

	var urls []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "File") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				url := strings.TrimSpace(parts[1])
				if url != "" {
					urls = append(urls, url)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading PLS file: %w", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("no valid stream URL found in PLS file")
	}

	return urls, nil
}

// parseStreamInfoFromURL extracts format and bitrate from SomaFM playlist URLs.
// URL patterns: groovesalad130.pls (MP3 128k), groovesalad-aac.pls (AAC), etc.
func parseStreamInfoFromURL(url string) StreamInfo {
	info := StreamInfo{
		Format:     "MP3",
		Quality:    "high",
		Bitrate:    128,
		SampleRate: 44100,
	}

	urlLower := strings.ToLower(url)

	if strings.Contains(urlLower, "aac") || strings.Contains(urlLower, "aacp") {
		info.Format = "AAC"
	}

	bitrates := []int{320, 256, 192, 130, 128, 64, 32}
	for _, br := range bitrates {
		brStr := fmt.Sprintf("%d", br)
		if strings.Contains(url, brStr+".pls") || strings.Contains(url, brStr+".") {
			info.Bitrate = br
			if br == 130 { // SomaFM uses 130 for 128kbps streams
				info.Bitrate = 128
			}
			break
		}
	}

	switch {
	case info.Bitrate >= 256:
		info.Quality = "highest"
	case info.Bitrate >= 128:
		info.Quality = "high"
	case info.Bitrate >= 64:
		info.Quality = "medium"
	default:
		info.Quality = "low"
	}

	return info
}
