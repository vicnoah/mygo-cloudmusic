package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/waygu-music-plugin/sdk/protocol"

	"waygu-music/internal/player/dlna"
	"waygu-music/internal/player/mpv"
)

func (a *app) syncMPVSettings() {
	if a.pl == nil {
		return
	}
	a.settings.EQ.Normalize()
	a.pl.SetMPVConfig(mpv.Config{
		Exclusive:        a.settings.MPVExclusive,
		ReplayGain:       a.settings.MPVReplayGain,
		AudioDevice:      a.settings.MPVAudioDevice,
		Gapless:          a.settings.MPVGapless,
		EqualizerEnabled: a.settings.EQ.Enabled,
		EqualizerType:    a.settings.EQ.Type,
		EqualizerPreamp:  a.settings.EQ.Preamp,
		EqualizerBands:   a.settings.EQ.Bands,
	})
	a.pl.SetUseMPV(a.settings.UseMPV)
}

// eqAvailable reports whether the equalizer has an MPV engine to run in.
func (a *app) eqAvailable() bool {
	return a.pl != nil && a.pl.UsingMPV()
}

func (a *app) mpvReady() bool {
	return mpv.Ready()
}

var (
	downloadMPV = mpv.Download
	loadMPV     = mpv.Load
	removeMPV   = mpv.Delete
	refreshDLNA = (*dlna.Controller).RefreshDevices
)

func (a *app) startMPVDownload() (protocol.DesktopTask, error) {
	if a.desktopTasks.latest[protocol.TaskDecoderDelete].Phase == protocol.TaskRunning {
		return protocol.DesktopTask{}, protocol.Failure{Code: "decoder-busy"}
	}
	if task := a.desktopTasks.latest[protocol.TaskDecoderDownload]; task.Phase == protocol.TaskRunning {
		return task, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mpvCancel = cancel
	a.mpvDone, a.mpvTotal = 0, 0
	task := a.beginDesktopTask(protocol.TaskDecoderDownload)

	go func() {
		err := downloadMPV(ctx, func(done, total int64) {
			postDesktopTask(a, func() {
				if a.desktopTaskRunning(task) {
					a.mpvDone, a.mpvTotal = done, total
					if a.settingsWin != nil {
						a.settingsWin.Invalidate()
					}
				}
			})
		})
		postDesktopTask(a, func() {
			if !a.desktopTaskRunning(task) || ctx.Err() != nil {
				return
			}
			cancel()
			a.mpvCancel = nil
			if err == nil {
				err = loadMPV()
				if err == nil {
					a.syncMPVSettings()
				}
			}
			if err != nil {
				err = fmt.Errorf("解码组件安装失败（%w）", err)
			}
			a.completeDesktopTask(task, err)
		})
	}()
	return task, nil
}

func (a *app) cancelMPVDownload(id uint64, phase protocol.TaskPhase) (protocol.DesktopTask, error) {
	task := a.desktopTasks.latest[protocol.TaskDecoderDownload]
	if id != 0 && id != task.ID {
		return protocol.DesktopTask{}, protocol.Failure{Code: "stale-task"}
	}
	if a.mpvCancel != nil {
		a.mpvCancel()
		a.mpvCancel = nil
	}
	return a.finishDesktopTask(task, phase, nil), nil
}

func (a *app) deleteMPV() protocol.DesktopTask {
	if task := a.desktopTasks.latest[protocol.TaskDecoderDelete]; task.Phase == protocol.TaskRunning {
		return task
	}
	_, _ = a.cancelMPVDownload(0, protocol.TaskReplaced)
	if a.pl != nil {
		a.pl.SetUseMPV(false)
	}
	a.settings.UseMPV = false
	a.syncMPVSettings()
	saveErr := a.settings.save()
	task := a.beginDesktopTask(protocol.TaskDecoderDelete)
	go func() {
		err := errors.Join(saveErr, removeMPV())
		postDesktopTask(a, func() {
			if err != nil {
				err = fmt.Errorf("删除解码组件失败（%w）", err)
			}
			a.completeDesktopTask(task, err)
		})
	}()
	return task
}

func (a *app) scanDLNADevices() (protocol.DesktopTask, error) {
	if a.pl == nil || a.pl.DLNA() == nil {
		return protocol.DesktopTask{}, protocol.Failure{Code: "cast-unavailable"}
	}
	if task := a.desktopTasks.latest[protocol.TaskCastingScan]; task.Phase == protocol.TaskRunning {
		return task, nil
	}
	ctrl := a.pl.DLNA()
	task := a.beginDesktopTask(protocol.TaskCastingScan)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_, err := refreshDLNA(ctrl, ctx)
		postDesktopTask(a, func() {
			if err != nil {
				err = fmt.Errorf("搜索投播设备失败（%w）", err)
			}
			a.completeDesktopTask(task, err)
		})
	}()
	return task, nil
}

func (a *app) switchPlaybackOutput(id string) (protocol.DesktopTask, error) {
	if a.pl == nil || a.pl.DLNA() == nil {
		return protocol.DesktopTask{}, protocol.Failure{Code: "cast-unavailable"}
	}
	task := a.beginDesktopTask(protocol.TaskCastingOutput)
	err := a.pl.SwitchOutput(id)
	if err != nil {
		err = fmt.Errorf("切换播放输出失败（%w）", err)
	}
	return a.completeDesktopTask(task, err), nil
}
