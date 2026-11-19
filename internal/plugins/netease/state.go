package netease

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/eq"
	"waygu-music/internal/plugins/netease/internal/client"
)

type pageKind int

const (
	pageHome pageKind = iota
	pageDaily
	pageCharts
	pagePlaylist
	pageSearch
	pageFM
	pageArtist
	pageAlbum
	pageUser
	pagePodcast
	pagePodcasts
)

type route struct {
	page pageKind
	// id is the playlist, artist, album, user or podcast the page shows.
	id int64
	// query and kind are what a search page searched for.
	query string
	kind  netease.SearchType
}

// app is the state both windows show. Everything in it is read and changed
// on the main thread: by the views, and by goroutines through post.
type app struct {
	notch            *notch
	host             pluginapi.Services
	pl               *playback
	pal              palette
	preferences      *platformPreferences
	savePreferences  func()
	platformMenuOpen bool
	scope            context.Context
	cancelScope      context.CancelFunc

	desktopLyricsTrackID                          int64
	desktopLyricsShownAt, desktopLyricsPreviousAt int
	desktopLyricsScrollGeneration                 int
	desktopLyrics                                 pluginapi.DesktopLyricsState
	trayLockItem                                  *mygo.MenuItem

	route   route
	history []route
	forward []route // the pages gone back from, the last first to go to

	profile      *netease.Profile
	mine, saved  []netease.Playlist // the listener's playlists: made, and saved
	savingPl     map[int64]bool     // the playlists whose saving or its end is on its way
	recPlaylists []netease.Playlist // the day's playlists picked for the listener
	login        loginState
	accountOpen  bool           // the dropdown of the account row shows
	naming       nameDialog     // the name of a playlist asked for
	deleting     deleteDialog   // whether to delete a playlist, asked
	liked        map[int64]bool // the songs the listener likes
	toast        string         // a message to show once
	picked       pickedUp       // the song a login picked up, waiting to play

	// The episodes the listener likes, and doesn't, as far as NetEase has
	// answered, and those asked about: see ui_like.go.
	likedPrograms, askedPrograms map[int64]bool

	daily       []netease.Track
	dailySpin   spinState // the record on the daily card
	dailyBars   barState  // the bars on its play button
	featured    []netease.Playlist
	charts      []netease.Playlist
	homeErr     string
	homeLoading bool
	homeAt      time.Time // when what the home page shows was asked for

	playlists  map[int64]*netease.Playlist
	loadingPl  map[int64]bool
	plErr      map[int64]string
	artists    map[int64]*artistData
	artistTab  int // what an artist's page lists: artistHot, artistLatest, artistAlbums or artistEvents
	albums     map[int64]*albumData
	users      map[int64]*userData
	podcasts   map[int64]*podcastData
	podcastTab *podcastTab // the podcasts tab's lists
	trackList  ui.ListState
	// Where the page and the sidebar are scrolled, kept while Now Playing
	// covers them, unbuilt.
	pageScroll ui.ScrollState
	navScroll  ui.ScrollState
	scrolled   route // the page trackList and pageScroll are of

	mineCollapsed, savedCollapsed bool

	query       string
	searchBox   ui.Handle
	searches    map[route]*searchData // the results of each search, by its page
	searched    route                 // the search last shown, which the sidebar goes back to
	suggestions suggestState          // what the search field's query may be after

	nowPlaying  bool
	npOpen      bool      // Now Playing as it last showed: open or not
	npChangedAt time.Time // when it last opened or closed
	npPane      int       // paneLyrics, paneQueue, paneStage, paneComments, paneProgram or paneCaptions
	npPaneTop   float32   // how far the cover beside the pane starts below its top
	lyricScroll ui.ScrollState
	lyricGlide  lyricGlide
	lyricAt     int
	lyricTrack  int64
	queueList   ui.ListState
	queueAt     int // the row of the queue pane last scrolled to, -1 while it is hidden
	comments    commentState
	tabCount    rollState // the count on the comments' tab, as it rolls

	fmMode        int    // the FM mode chosen, in fmModes
	radioStarting string // the radio asked for its first songs, or ""

	scrub      float64
	scrubbing  bool
	volume     float64
	volumeOpen bool // the volume popover of the player bar shows
	mutedFrom  float64

	rowBars barState
	// underTop and underBottom say whether the page has content under its
	// bars, which show their scroll edges then.
	underTop, underBottom bool
	barScrub              scrubState
	npScrub               scrubState
	backdropCache         backdropCache // the backdrop of Now Playing, as last painted
	npArt                 artwork       // the cover whose colors Now Playing shows
	npCoverState          coverState    // the cover of Now Playing, as it is tilted and flicked
	wallState             wallState     // the wall of the queue's covers, as it is dragged
	quiet                 quietState    // the quiet parts of the song playing, for its hot comments
	captions              captionState  // the episode playing's captions, and the model that writes them
	castOpen              bool
	eqOpen                bool
	eqUI                  eqUIState
	settings              settings
}

type settings struct {
	pluginapi.DesktopPreferences
	LyricsStage, StageSparks         bool
	StageStyle                       string
	EQ                               eq.Settings
	Quality                          string
	Normalize, HotComments, Captions bool
}

type playback struct {
	pluginapi.PlaybackControls
	pluginapi.PlaybackContinuity
	port   pluginapi.PlayerPort
	volume func(float64)
}

func (p *playback) PlayQueue(tracks []pluginapi.Track, start int) { p.port.PlayQueue(tracks, start) }
func (p *playback) PlayRadio(name string, tracks []pluginapi.Track, feed func(func([]pluginapi.Track))) {
	p.port.PlayRadio(name, tracks, feed)
}
func (p *playback) SetVolume(v float64) { p.volume(v) }

func (a *app) showMain()     { a.host.ShowMain() }
func (a *app) openSettings() { a.host.OpenSettings() }
func (a *app) redraw()       { a.host.Invalidate() }

// scopedPost captures the activation owning a background result before
// the request starts. A previous activation can finish after a switch back.
func (a *app) scopedPost() func(func()) {
	ctx := a.scope
	return func(fn func()) {
		if ctx.Err() != nil {
			return
		}
		a.host.Post(func() {
			if ctx.Err() == nil {
				fn()
			}
		})
	}
}

func (a *app) restoreSession() {
	s, ok := a.pl.LoadSession()
	if !ok {
		return
	}
	a.pl.Restore(s)
	if s.Radio != "" && a.pl.Current() != nil {
		a.pl.AttachRadio(s.Radio, a.radioFeed(s.Radio))
	}
}

func (a *app) refreshSharedSettings() {
	a.settings.DesktopPreferences = a.host.DesktopSettings().Preferences
	view := a.host.Presentation()
	a.settings.LyricsStage, a.settings.StageStyle, a.settings.StageSparks = view.LyricsStage, view.StageStyle, view.StageSparks
	a.settings.EQ = a.host.EqualizerSettings()
	a.volume = a.host.Volume()
}

func (a *app) saveLyricsPresentation() {
	s := a.settings
	a.host.SetLyricsPresentation(s.LyricsStage, s.StageStyle, s.StageSparks)
}

func (a *app) savePlatformPreferences() {
	s := &a.settings
	*a.preferences = platformPreferences{Quality: s.Quality, Normalize: s.Normalize, HotComments: s.HotComments, Captions: s.Captions}
	a.savePreferences()
}

func (a *app) syncMPVSettings() {
	a.host.ApplyEqualizer(a.settings.EQ)
	a.closeUnavailableEqualizer()
}
func (a *app) eqAvailable() bool { return a.host.EqualizerAvailable() }

type artwork = pluginapi.DecodedArtwork

func (a *app) closeUnavailableEqualizer() {
	if !a.eqAvailable() {
		a.eqOpen, a.eqUI.saveOpen, a.eqUI.deleteOpen, a.eqUI.autoeqOpen = false, false, false, false
	}
}
