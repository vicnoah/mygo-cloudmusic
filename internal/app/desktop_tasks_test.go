package app

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/protocol"

	"waygu-music/internal/player"
	"waygu-music/internal/player/dlna"
)

func desktopWorkerPosts(t *testing.T) chan func() {
	t.Helper()
	posts := make(chan func(), 16)
	post := postDesktopTask
	postDesktopTask = func(_ *app, fn func()) { posts <- fn }
	t.Cleanup(func() { postDesktopTask = post })
	return posts
}

func desktopTestReceive[T any](t *testing.T, queue <-chan T) T {
	t.Helper()
	select {
	case result := <-queue:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("desktop task did not report back")
		var zero T
		return zero
	}
}

func desktopTestHost(t *testing.T, a *app) func(string, uint64) (protocol.DesktopTask, error) {
	t.Helper()
	scope := a.pluginActivations.Prepare("desktop-test")
	if !a.pluginActivations.Commit(scope) {
		t.Fatal("could not activate desktop task fixture")
	}
	reverse := pluginReverse{app: a, connection: scope.Connection}
	return func(action string, id uint64) (protocol.DesktopTask, error) {
		var reply PluginHostReply
		in := PluginHostCall{Scope: scope, Action: action, TaskID: id}
		err := reverse.apply(bridge.LocalHostCall(in.Wire()), &reply)
		wire := reply.Wire()
		if wire.Task == nil {
			return protocol.DesktopTask{}, err
		}
		return *wire.Task, err
	}
}

// Progress and completion from a canceled libmpv download cannot change its replacement.
func TestDesktopDownloadIgnoresCanceledWorker(t *testing.T) {
	a := newTestApp()
	use := desktopTestHost(t, a)
	posts := desktopWorkerPosts(t)
	type runningDownload struct {
		ctx      context.Context
		progress func(int64, int64)
		finish   chan error
	}
	workers := make(chan runningDownload, 2)
	shutdown := make(chan struct{})
	download, load := downloadMPV, loadMPV
	defer func() {
		close(shutdown)
		downloadMPV, loadMPV = download, load
	}()
	downloadMPV = func(ctx context.Context, progress func(int64, int64)) error {
		finish := make(chan error, 1)
		workers <- runningDownload{ctx: ctx, progress: progress, finish: finish}
		select {
		case err := <-finish:
			return err
		case <-shutdown:
			return context.Canceled
		}
	}
	var loads int
	loadMPV = func() error {
		loads++
		return nil
	}

	old, err := use(protocol.TaskDecoderDownload, 0)
	if err != nil || !old.Running() {
		t.Fatalf("initial download was not accepted: %+v, %v", old, err)
	}
	oldWorker := desktopTestReceive(t, workers)
	stopped, err := use("decoder-cancel", old.ID)
	if err != nil || stopped.Phase != protocol.TaskCanceled || oldWorker.ctx.Err() != context.Canceled {
		t.Fatalf("cancellation did not stop the initial task: %+v, %v", stopped, err)
	}
	current, err := use(protocol.TaskDecoderDownload, 0)
	if err != nil || !current.Running() || current.ID == old.ID {
		t.Fatalf("retry did not get a fresh task: %+v, %v", current, err)
	}
	worker := desktopTestReceive(t, workers)
	worker.progress(24, 100)
	desktopTestReceive(t, posts)()
	oldWorker.progress(97, 100)
	desktopTestReceive(t, posts)()
	oldWorker.finish <- nil
	desktopTestReceive(t, posts)()
	status := (platformHost{a: a}).DecoderStatus()
	if !status.Downloading || status.DownloadTask.ID != current.ID || status.Done != 24 || loads != 0 {
		t.Fatalf("late worker changed the active download: %+v, loads=%d", status, loads)
	}
	if _, err := use("decoder-cancel", old.ID); err == nil || worker.ctx.Err() != nil {
		t.Fatalf("late cancel affected its replacement: %v", err)
	}
	worker.finish <- nil
	desktopTestReceive(t, posts)()
	status = bridge.LocalSnapshot(bridge.WireSnapshot(capturePluginSnapshot(a, nil))).Decoder
	if status.Downloading || status.DownloadTask.Phase != protocol.TaskSucceeded || loads != 1 {
		t.Fatalf("completed retry was not exposed as success: %+v, loads=%d", status, loads)
	}
	oldResult, err := use(protocol.HostDesktopTask, old.ID)
	if err != nil || oldResult != stopped {
		t.Fatalf("old terminal result was lost after retry: %+v, %v", oldResult, err)
	}
}

// Removing the decoder replaces a pending download and blocks installation until deletion finishes.
func TestDesktopDeleteReplacesPendingInstall(t *testing.T) {
	a := newTestApp()
	use := desktopTestHost(t, a)
	posts := desktopWorkerPosts(t)
	started := make(chan context.Context, 1)
	removing := make(chan struct{})
	deleted := make(chan error, 1)
	stop := make(chan struct{})
	download, load, remove := downloadMPV, loadMPV, removeMPV
	defer func() {
		close(stop)
		downloadMPV, loadMPV, removeMPV = download, load, remove
	}()
	downloadMPV = func(ctx context.Context, _ func(int64, int64)) error {
		started <- ctx
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			return context.Canceled
		}
	}
	loadMPV = func() error {
		t.Error("a replaced download tried to load the decoder")
		return nil
	}
	removeMPV = func() error {
		close(removing)
		select {
		case err := <-deleted:
			return err
		case <-stop:
			return context.Canceled
		}
	}
	install, err := use(protocol.TaskDecoderDownload, 0)
	if err != nil || !install.Running() {
		t.Fatalf("install was not accepted: %+v, %v", install, err)
	}
	active := desktopTestReceive(t, started)
	removeTask, err := use(protocol.TaskDecoderDelete, 0)
	if err != nil || !removeTask.Running() || active.Err() != context.Canceled {
		t.Fatalf("delete did not replace the download: %+v, %v", removeTask, err)
	}
	desktopTestReceive(t, removing)
	desktopTestReceive(t, posts)()
	replaced, err := use(protocol.HostDesktopTask, install.ID)
	if err != nil || replaced.Phase != protocol.TaskReplaced {
		t.Fatalf("superseded install did not retain its terminal status: %+v, %v", replaced, err)
	}
	if _, err := use(protocol.TaskDecoderDownload, 0); err != (protocol.Failure{Code: "decoder-busy"}) {
		t.Fatalf("download was accepted during deletion: %v", err)
	}
	deleted <- errors.New("fixture deletion failure")
	desktopTestReceive(t, posts)()
	final, err := use(protocol.HostDesktopTask, removeTask.ID)
	if err != nil || final.Phase != protocol.TaskFailed || final.Message == "" {
		t.Fatalf("deletion failure was not exposed: %+v, %v", final, err)
	}
}

// Clearing cached songs reports completed deletion or a filesystem failure, without older size results replacing it.
func TestDesktopCacheClearReportsActualResult(t *testing.T) {
	a := newTestApp()
	use := desktopTestHost(t, a)
	posts := desktopWorkerPosts(t)
	dir := t.TempDir()
	song := filepath.Join(dir, "test.mp3")
	if err := os.WriteFile(song, []byte("cached song"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.pl.Cache = player.NewCache(dir, 1<<20)
	a.cacheUsed = 11
	a.measureCache()
	oldMeasurement := desktopTestReceive(t, posts)

	task, err := use(protocol.TaskCacheClear, 0)
	if err != nil || !task.Running() || a.cacheUsed != 11 {
		t.Fatalf("clear claimed an early result: %+v, usage %d, %v", task, a.cacheUsed, err)
	}
	desktopTestReceive(t, posts)()
	oldMeasurement()
	got := bridge.LocalSnapshot(bridge.WireSnapshot(capturePluginSnapshot(a, nil))).Desktop
	if got.CacheUsed != 0 || got.CacheTask.ID != task.ID || got.CacheTask.Phase != protocol.TaskSucceeded {
		t.Fatalf("new cache state was overwritten: %+v", got)
	}
	if _, err := os.Stat(song); !os.IsNotExist(err) {
		t.Fatalf("completed task left cached audio on disk: %v", err)
	}

	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.pl.Cache = player.NewCache(blocked, 1<<20)
	a.cacheUsed = 16
	failed, err := use(protocol.TaskCacheClear, 0)
	if err != nil || !failed.Running() {
		t.Fatalf("cache operation failed before it was accepted: %+v, %v", failed, err)
	}
	desktopTestReceive(t, posts)()
	result, err := use(protocol.HostDesktopTask, failed.ID)
	if err != nil || result.Phase != protocol.TaskFailed || result.Message == "" || a.cacheUsed != 16 {
		t.Fatalf("cache filesystem failure had no terminal result: %+v, %v", result, err)
	}
}

// A failed discovery ends scanning and can be retried, and selecting a missing renderer reports failure.
func TestDesktopCastingTaskFailures(t *testing.T) {
	a := newTestApp()
	use := desktopTestHost(t, a)
	posts := desktopWorkerPosts(t)
	refresh := refreshDLNA
	defer func() { refreshDLNA = refresh }()
	failures := make(chan error, 2)
	failures <- errors.New("fixture network failure")
	failures <- nil
	refreshDLNA = func(*dlna.Controller, context.Context) ([]*dlna.Device, error) {
		return nil, <-failures
	}
	failed, err := use(protocol.TaskCastingScan, 0)
	if err != nil || !failed.Running() {
		t.Fatalf("scan was not accepted: %+v, %v", failed, err)
	}
	desktopTestReceive(t, posts)()
	state := (platformHost{a: a}).Casting()
	if state.Scanning || state.ScanTask.ID != failed.ID || state.ScanTask.Phase != protocol.TaskFailed {
		t.Fatalf("failed scan stayed in progress: %+v", state)
	}
	retry, err := use(protocol.TaskCastingScan, 0)
	if err != nil || !retry.Running() || retry.ID == failed.ID {
		t.Fatalf("failed scan could not be retried: %+v, %v", retry, err)
	}
	desktopTestReceive(t, posts)()
	if got := (platformHost{a: a}).Casting().ScanTask; got.Phase != protocol.TaskSucceeded {
		t.Fatalf("recovered scan did not report completion: %+v", got)
	}
	task, err := a.switchPlaybackOutput("missing-renderer")
	if err != nil || task.Phase != protocol.TaskFailed || a.pl.IsCasting() {
		t.Fatalf("missing renderer was reported as selected: %+v, %v", task, err)
	}
}

// A delayed desktop patch clears explicit zero values without overwriting unrelated host changes.
func TestDesktopPreferencePatchKeepsNewerHostChoices(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Appearance = "dark"
	a.settings.CacheMB = 1024
	a.settings.MPVExclusive = true
	a.settings.MPVAudioDevice = "device-now-set-on-host"
	a.settings.Fog = true
	base := protocol.DesktopPreferences{CacheMB: 1024, MPVExclusive: true, MPVAudioDevice: "device-before"}
	patch := base.Changes(protocol.DesktopPreferences{})
	scope := a.pluginActivations.Prepare("patch-test")
	a.pluginActivations.Commit(scope)
	request := protocol.PluginHostCall{Scope: scope, Action: protocol.HostPatchDesktopPreferences, DesktopPatch: patch}
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(request); err != nil {
		t.Fatal(err)
	}
	var wire protocol.PluginHostCall
	if err := gob.NewDecoder(&buffer).Decode(&wire); err != nil {
		t.Fatal(err)
	}
	reverse := pluginReverse{app: a, connection: scope.Connection}
	if err := reverse.apply(bridge.LocalHostCall(wire), &PluginHostReply{}); err != nil {
		t.Fatal(err)
	}
	if a.settings.CacheMB != 0 || a.settings.MPVExclusive || a.settings.MPVAudioDevice != "" ||
		a.settings.Appearance != "dark" || !a.settings.Fog {
		t.Fatalf("desktop patch did not preserve host changes while clearing values: %+v", a.settings)
	}
}
