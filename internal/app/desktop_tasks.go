package app

import (
	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/protocol"
)

const desktopTaskHistory = 32

// Desktop tasks have host lifetime and are changed only on its application thread.
type desktopTasks struct {
	next   uint64
	latest map[string]protocol.DesktopTask
	recent []protocol.DesktopTask
}

func (a *app) beginDesktopTask(kind string) protocol.DesktopTask {
	tasks := &a.desktopTasks
	tasks.next++
	task := protocol.DesktopTask{ID: tasks.next, Kind: kind, Phase: protocol.TaskRunning}
	if tasks.latest == nil {
		tasks.latest = make(map[string]protocol.DesktopTask)
	}
	tasks.latest[kind] = task
	a.desktopTasksChanged()
	return task
}

func (a *app) desktopTaskRunning(task protocol.DesktopTask) bool {
	current := a.desktopTasks.latest[task.Kind]
	return current.ID == task.ID && current.Phase == protocol.TaskRunning
}

func (a *app) finishDesktopTask(task protocol.DesktopTask, phase protocol.TaskPhase, err error) protocol.DesktopTask {
	if !a.desktopTaskRunning(task) {
		return task
	}
	tasks := &a.desktopTasks
	task.Phase = phase
	if err != nil {
		task.Message = err.Error()
	}
	tasks.latest[task.Kind] = task
	if len(tasks.recent) == desktopTaskHistory {
		copy(tasks.recent, tasks.recent[1:])
		tasks.recent = tasks.recent[:desktopTaskHistory-1]
	}
	tasks.recent = append(tasks.recent, task)
	if a.remoteActive() == nil {
		if message := bridge.TaskMessage(task); message != "" {
			a.toast = message
		}
	}
	a.desktopTasksChanged()
	return task
}

func (a *app) completeDesktopTask(task protocol.DesktopTask, err error) protocol.DesktopTask {
	if err != nil {
		return a.finishDesktopTask(task, protocol.TaskFailed, err)
	}
	return a.finishDesktopTask(task, protocol.TaskSucceeded, nil)
}

func (s *desktopTasks) find(id uint64) (protocol.DesktopTask, bool) {
	for _, task := range s.latest {
		if task.ID == id {
			return task, true
		}
	}
	for _, task := range s.recent {
		if task.ID == id {
			return task, true
		}
	}
	return protocol.DesktopTask{}, false
}

func (a *app) desktopTasksChanged() {
	if remote := a.remoteActive(); remote != nil {
		remote.publishPlayback(a, false)
	}
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
	a.redraw()
}

// Desktop workers post their results to the host application thread.
var postDesktopTask = (*app).post
