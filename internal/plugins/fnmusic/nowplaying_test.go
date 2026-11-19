package fnmusic

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/internal/plugintest"
)

// Feiniu Now Playing omits the NetEase comments pane because Feiniu has no comments API.
func TestFeiniuNowPlayingHidesComments(t *testing.T) {
	h := plugintest.Start(t, Registration())
	h.Engine.Cue([]pluginapi.Track{{ID: 1, Source: "fn", Name: "飞牛歌曲"}})
	tt := ui.NewTester(h.Windows[pluginapi.MainWindow].Root, 1200, 800)
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("关闭"); !ok {
		t.Fatal("Now Playing did not open")
	}
	if _, ok := tt.Find("评论"); ok {
		t.Fatal("Feiniu exposed the comments pane")
	}
}
