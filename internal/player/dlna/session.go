package dlna

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/supersonic-app/go-upnpcast/services/avtransport"
)

// CastStatus is the current renderer session, including its last confirmed state.
type CastStatus struct {
	ID                            string
	Playing, Loading, Want, Ready bool
	Ended                         bool
	Position, Duration            time.Duration
	Err                           error
}

var errNoAudioRequest = errors.New("设备没有从本机读取音频")
var ErrRendererDidNotStart = errors.New("设备已读取音频，但未能开始播放")

type castSession struct {
	id                            string
	ctx                           context.Context
	device                        *Device
	item                          *avtransport.MediaItem
	telemetry                     *streamTelemetry
	want, sent, loaded, loading   bool
	confirmed, seekOnStart, ended bool
	eof                           bool
	revision, seekRevision        uint64
	seekPending, userSeek         bool
	onEnd                         func()
	pauseAt                       time.Time
	positionSeen                  bool
	handoffRetryAt                time.Time
	lastPosition                  time.Duration
	target                        time.Duration
	err                           error
	startedAt, lastReport         time.Time
}

const castStartupLimit = 12 * time.Second

// PrepareTrack records a track without telling the renderer to load or play it.
// Some renderers start playing as soon as they receive the media URI.
func (c *Controller) PrepareTrack(ctx context.Context, meta TrackMeta, pos time.Duration) error {
	if meta.StreamURL == "" && meta.LocalFile == "" {
		return errors.New("没有可投播的音源")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeDevice == nil || ctx.Err() != nil {
		return errors.New("投播会话已取消")
	}
	c.stopDeviceLocked(c.activeDevice)
	c.clearTracksLocked()
	meta.telemetry = &streamTelemetry{}
	c.currentID = meta.ID
	item := c.registerTrack(meta, c.activeDevice)
	pos = max(0, pos)
	if meta.Duration > 0 {
		pos = min(pos, meta.Duration)
	}
	c.position, c.duration = pos, meta.Duration
	c.session = &castSession{
		id: meta.ID, ctx: ctx, device: c.activeDevice, item: item, telemetry: meta.telemetry,
		target: pos, seekOnStart: pos > 0, onEnd: c.onEnd,
	}
	return nil
}

// PlayTrack prepares and starts a track, for callers that explicitly want to play.
func (c *Controller) PlayTrack(ctx context.Context, meta TrackMeta, pos time.Duration) error {
	if err := c.PrepareTrack(ctx, meta, pos); err != nil {
		return err
	}
	s, rev := c.playIntent(ctx)
	if s == nil {
		return context.Canceled
	}
	return c.start(s, rev)
}

func (c *Controller) CastStatus() CastStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.session
	if s == nil {
		return CastStatus{}
	}
	return CastStatus{
		ID: s.id, Playing: c.playing, Loading: s.loading, Want: s.want,
		Ended:    s.eof,
		Ready:    !s.ended && (s.err == nil || s.loaded),
		Position: c.positionLocked(), Duration: c.duration, Err: s.err,
	}
}

func (c *Controller) positionLocked() time.Duration {
	pos := c.position

	if c.duration > 0 {
		pos = min(c.duration, pos)
	}
	return pos
}

func (c *Controller) currentLocked(s *castSession, rev uint64) bool {
	return c.session == s && s.revision == rev && s.ctx.Err() == nil
}

func (c *Controller) playIntent(ctx context.Context) (*castSession, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.session
	if s == nil || s.ctx.Err() != nil || ctx.Err() != nil {
		return nil, 0
	}
	if s.ended || s.err != nil && !s.loaded {
		return nil, 0
	}
	s.want, s.loading, s.err = true, true, nil
	s.pauseAt = time.Time{}
	s.positionSeen = false
	s.startedAt = time.Time{}
	s.handoffRetryAt = time.Time{}
	s.revision++
	return s, s.revision
}

// Resume starts a prepared track or resumes one already loaded on the renderer.
func (c *Controller) Resume(ctx context.Context) error {
	s, rev := c.playIntent(ctx)
	if s != nil {
		c.notify()
		go func() { _ = c.start(s, rev) }()
	}
	return ctx.Err()
}

func (c *Controller) start(s *castSession, rev uint64) error {
	for {
		c.mu.RLock()
		stop := c.remoteStops[s.device.ID]
		c.mu.RUnlock()
		if stop == nil {
			break
		}
		select {
		case <-stop:
		case <-s.ctx.Done():
			return s.ctx.Err()
		}
	}
	c.remoteMu.Lock()
	defer c.remoteMu.Unlock()
	c.mu.Lock()
	if !c.currentLocked(s, rev) {
		c.mu.Unlock()
		return context.Canceled
	}
	loaded := s.loaded
	s.sent = true
	if !loaded {
		s.startedAt = time.Now()
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(s.ctx, 6*time.Second)
	defer cancel()
	if !loaded {
		if err := s.device.SetMedia(ctx, s.item); err != nil {
			return c.fail(s, rev, fmt.Errorf("设备无法加载音源: %w", err))
		}
		c.mu.Lock()
		if c.session == s {
			s.loaded = true
			c.startPollingLocked(s)
		}
		c.mu.Unlock()
	}
	c.mu.RLock()
	current := c.currentLocked(s, rev) && s.want
	c.mu.RUnlock()
	if !current {
		return context.Canceled
	}
	if err := s.device.Play(ctx); err != nil {
		var fault *transportFault
		if errors.As(err, &fault) && (fault.code == "701" || fault.code == "714" || fault.code == "704") {
			if streamErr := s.startError(); streamErr != errNoAudioRequest {
				return c.fail(s, rev, streamErr)
			}
		}
		return c.fail(s, rev, fmt.Errorf("设备拒绝播放: %w", err))
	}
	c.mu.Lock()
	if c.currentLocked(s, rev) {
		s.startedAt = time.Now()
	}

	c.mu.Unlock()
	c.notify()
	return nil
}

func (c *Controller) startPollingLocked(s *castSession) {
	if s.ctx.Err() == nil && c.cancelPoll == nil {
		pollCtx, cancelPoll := context.WithCancel(s.ctx)
		c.cancelPoll = cancelPoll
		go c.poll(pollCtx, s)
	}
}

func (c *Controller) fail(s *castSession, rev uint64, err error) error {
	c.mu.Lock()
	if !c.currentLocked(s, rev) {
		c.mu.Unlock()
		return err
	}
	c.stopDeviceLocked(s.device)
	c.revokeTracksLocked()
	if c.cancelPoll != nil {
		c.cancelPoll()
		c.cancelPoll = nil
	}
	s.loading, s.want, s.err = false, false, err
	s.loaded, s.sent = false, false
	c.playing = false
	c.mu.Unlock()
	c.notify()
	return err
}

// Pause records the latest intent before sending the command to the renderer.
func (c *Controller) Pause(ctx context.Context) error {
	c.mu.Lock()
	s := c.session
	if s == nil || ctx.Err() != nil {
		c.mu.Unlock()
		return ctx.Err()
	}
	s.want, s.loading = false, false
	if s.sent {
		s.pauseAt = time.Now()
	}
	s.revision++
	rev, sent := s.revision, s.sent
	c.mu.Unlock()
	c.notify()
	if !sent {
		return nil
	}
	go func() {
		c.remoteMu.Lock()
		defer c.remoteMu.Unlock()
		c.mu.RLock()
		current := c.currentLocked(s, rev)
		c.mu.RUnlock()
		if !current {
			return
		}
		req, cancel := context.WithTimeout(s.ctx, 2*time.Second)
		defer cancel()
		if err := s.device.Pause(req); err != nil {
			c.mu.Lock()
			if c.currentLocked(s, rev) {
				s.err = fmt.Errorf("设备未确认暂停: %w", err)
				s.want = c.playing
				s.pauseAt = time.Time{}
			}
			c.mu.Unlock()
			c.notify()
			return
		}
		c.sample(s.ctx, s)
	}()
	return nil
}

// Seek preserves the position of an unstarted cast without waking the device.
func (c *Controller) Seek(ctx context.Context, pos time.Duration) error {
	c.mu.Lock()
	s := c.session
	if s == nil || ctx.Err() != nil {
		c.mu.Unlock()
		return ctx.Err()
	}
	pos = max(0, pos)
	if c.duration > 0 {
		pos = min(pos, c.duration)
	}
	s.target = pos
	if !s.sent || !s.confirmed {
		s.handoffRetryAt = time.Time{}
		s.seekOnStart = true
		c.position = pos
		c.mu.Unlock()
		c.notify()
		return nil
	}
	if !s.want {
		// Some devices acknowledge a paused seek but silently ignore it. Try it
		// now, and apply it on resume if the position never actually changes.
		s.seekOnStart = true
		s.handoffRetryAt = time.Time{}
	}
	s.seekRevision++
	s.seekPending, s.userSeek = true, true
	seekRev := s.seekRevision
	c.mu.Unlock()
	go func() {
		c.remoteMu.Lock()
		defer c.remoteMu.Unlock()
		c.mu.RLock()
		current := c.session == s && s.ctx.Err() == nil && s.seekRevision == seekRev && s.loaded
		c.mu.RUnlock()
		if !current {
			return
		}
		req, cancel := context.WithTimeout(s.ctx, 3*time.Second)
		defer cancel()
		err := s.device.Seek(req, int(pos.Seconds()))
		c.mu.Lock()
		if c.session == s && s.seekRevision == seekRev {
			s.seekPending = false
			if err != nil {
				s.err = fmt.Errorf("设备未能定位: %w", err)
			} else {
				s.err = nil
			}
		}
		c.mu.Unlock()
		c.sample(s.ctx, s)
	}()
	return nil
}

func (c *Controller) poll(ctx context.Context, s *castSession) {
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.remoteMu.Lock()
			c.sample(ctx, s)
			c.remoteMu.Unlock()
		}
	}
}

// sample runs in the device command lane, so state queries cannot overtake Seek or Pause.
func (s *castSession) startError() error {
	if status := s.telemetry.failure.Load(); status != 0 {
		return fmt.Errorf("设备读取音频失败（HTTP %d）", status)
	}
	if s.telemetry.bytes.Load() != 0 {
		return ErrRendererDidNotStart
	}
	return errNoAudioRequest
}

func (c *Controller) sample(ctx context.Context, s *castSession) {
	c.mu.RLock()
	rev, seekRev := s.revision, s.seekRevision
	current := c.currentLocked(s, rev) && s.loaded && !s.seekPending && !s.ended
	c.mu.RUnlock()
	if !current {
		return
	}
	query, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	state, statusErr := s.device.TransportState(query)
	info, posErr := s.device.readPosition(query)

	c.mu.Lock()
	if !c.currentLocked(s, rev) || s.seekRevision != seekRev || s.ended {
		c.mu.Unlock()
		return
	}
	now := time.Now()
	expired := s.loading && !s.startedAt.IsZero() && now.Sub(s.startedAt) >= castStartupLimit
	if statusErr != nil || state == "" {
		if expired {
			c.mu.Unlock()
			_ = c.fail(s, rev, errors.New("无法连接设备或读取播放状态"))
			return
		}
		if s.confirmed && now.Sub(s.lastReport) >= 3*time.Second {
			c.playing = false
			s.err = errors.New("设备未响应，播放状态暂不可用")
		}
		c.mu.Unlock()
		c.notify()
		return
	}

	if !sameMedia(info.URI, s.item.URL) {
		if s.confirmed {
			// A different controller took over the renderer.
			s.loading, s.want, s.sent, s.loaded, s.ended = false, false, false, false, true
			c.playing = false
			c.revokeTracksLocked()
			if c.cancelPoll != nil {
				c.cancelPoll()
				c.cancelPoll = nil
			}
			c.mu.Unlock()
			c.notify()
			return
		}
		c.mu.Unlock()
		if expired {
			_ = c.fail(s, rev, errors.New("设备未加载当前投播音源"))
		}
		return
	}

	if !s.lastReport.IsZero() && now.Sub(s.lastReport) >= 3*time.Second {
		s.err = nil
	}
	s.lastReport = now
	if posErr == nil && info.Duration > 0 {
		c.duration = info.Duration
	}
	if state == "PLAYING" && !s.pauseAt.IsZero() {
		if now.Sub(s.pauseAt) < 3*time.Second {
			c.mu.Unlock()
			return
		}
		s.pauseAt = time.Time{}
		s.err = errors.New("设备未确认暂停")
		s.want = true
	}
	if info.Known && s.seekOnStart && !s.want && c.duration > 0 &&
		!s.seekPending && info.Position/time.Second == s.target/time.Second {
		s.seekOnStart = false
	}
	if state == "PLAYING" && s.want && s.seekOnStart && s.handoffRetryAt.IsZero() {
		// Start the decoder before asking it to seek. Receivers can ignore Seek
		// or stop fetching altogether if asked before the stream is running.
		hasAudio := info.Known && info.Position > 0 || s.telemetry.bytes.Load() > 0
		if hasAudio {
			target := s.target
			c.mu.Unlock()
			req, done := context.WithTimeout(ctx, 2*time.Second)
			err := s.device.Seek(req, int(target.Seconds()))
			done()
			c.mu.Lock()
			if c.currentLocked(s, rev) && s.target == target {
				s.positionSeen = false
				if err != nil {
					s.seekOnStart = false
					s.err = fmt.Errorf("设备未能接续到 %s: %w", target, err)
				} else {
					s.handoffRetryAt = time.Now()
				}
			}
			c.mu.Unlock()
			c.notify()
			return
		}
	}
	if state == "PLAYING" && s.want && s.seekOnStart && !s.handoffRetryAt.IsZero() {
		switch {
		case info.Known && info.Position >= s.target.Truncate(time.Second) && info.Position <= s.target+2*time.Second:
			s.seekOnStart = false
		case now.Sub(s.handoffRetryAt) >= 3*time.Second:
			s.seekOnStart = false
			s.err = fmt.Errorf("设备未确认接续到 %s", s.target)
		}
	}

	switch state {
	case "PLAYING":
		// Some receivers report PLAYING while their decoder is still stuck at zero.
		// Accept progress on the device clock, or actual delivery if it has no clock.
		progressed := info.Known && s.positionSeen && info.Position > s.lastPosition
		if info.Known {
			s.lastPosition, s.positionSeen = info.Position, true
		}
		if !s.loading || !s.seekOnStart && (progressed || (!info.Known && posErr == nil && s.telemetry.bytes.Load() > 0)) {
			c.playing, s.confirmed, s.loading, s.want = true, true, false, true
			if info.Known {
				c.position = info.Position
			}
		}
	case "PAUSED_PLAYBACK", "PAUSED_RECORDING", "STOPPED", "NO_MEDIA_PRESENT":
		if !s.loading || !s.want {
			prev := c.position
			if info.Known && (state != "STOPPED" || info.Position > 0) {
				c.position = info.Position
			}
			c.playing, s.loading, s.want = false, false, false
			s.pauseAt = time.Time{}
			last := max(prev, c.position)
			nearEnd := c.duration > 0 && last > 0 && last >= c.duration-min(c.duration/4, 2*time.Second)
			if state == "STOPPED" && s.confirmed && nearEnd && !s.userSeek {
				s.ended, s.eof = true, true
				if c.cancelPoll != nil {
					c.cancelPoll()
					c.cancelPoll = nil
				}
				cb := s.onEnd
				c.mu.Unlock()
				c.notify()
				if cb != nil {
					cb()
				}
				return
			}
		}
	case "TRANSITIONING":
		c.playing = false
		if s.want && !s.loading {
			s.startedAt = now
			s.positionSeen = false
			s.loading = true
		}
	}
	if c.playing {
		s.userSeek = false
	}
	if expired && s.loading {
		err := s.startError()
		c.mu.Unlock()
		_ = c.fail(s, rev, err)
		return
	}
	c.mu.Unlock()
	c.notify()
}

// UpdateCover attaches late artwork without delaying the renderer's audio setup.
func (c *Controller) UpdateCover(id string, data []byte) {
	if len(data) == 0 {
		return
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if id == c.currentID {
		c.proxy.RegisterCoverData(id, data)
	}
}
