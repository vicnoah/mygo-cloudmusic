package app

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/protocol"

	"waygu-music/internal/player"
)

// newSongCache returns the cache of songs played, in the app's cache
// directory, or nil without one.
func newSongCache(mb int) *player.Cache {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return nil
	}
	return player.NewCache(filepath.Join(dir, "songs"), int64(mb)<<20)
}

// applyCacheLimit gives the cache the limit the settings say.
func (a *app) applyCacheLimit() {
	if cache := a.pl.Cache; cache != nil {
		limit := int64(a.settings.CacheMB) << 20
		revision := a.cacheLimitRev.Add(1)
		go func() {
			a.cacheLimitMu.Lock()
			if a.cacheLimitRev.Load() == revision {
				cache.SetLimit(limit)
			}
			a.cacheLimitMu.Unlock()
			postDesktopTask(a, a.measureCache)
		}()
	}
}

// measureCache finds how much the cache takes, on a goroutine, for the
// settings to show.
func (a *app) measureCache() {
	cache := a.pl.Cache
	if cache == nil || a.desktopTasks.latest[protocol.TaskCacheClear].Phase == protocol.TaskRunning {
		return
	}
	a.cacheMeasure++
	measure := a.cacheMeasure
	go func() {
		n, err := cache.Measure()
		postDesktopTask(a, func() {
			if err == nil && a.cacheMeasure == measure {
				a.cacheUsed = n
				a.desktopTasksChanged()
			}
		})
	}()
}

func (a *app) clearCache() (protocol.DesktopTask, error) {
	cache := a.pl.Cache
	if cache == nil {
		return protocol.DesktopTask{}, protocol.Failure{Code: "cache-unavailable"}
	}
	if task := a.desktopTasks.latest[protocol.TaskCacheClear]; task.Phase == protocol.TaskRunning {
		return task, nil
	}
	a.cacheMeasure++
	task := a.beginDesktopTask(protocol.TaskCacheClear)
	go func() {
		err := cache.Clear()
		n, measurementErr := cache.Measure()
		postDesktopTask(a, func() {
			if measurementErr == nil {
				a.cacheUsed = n
			} else if err == nil {
				err = measurementErr
			}
			if err != nil {
				err = fmt.Errorf("清除歌曲缓存失败（%w）", err)
			}
			a.completeDesktopTask(task, err)
		})
	}()
	return task, nil
}
