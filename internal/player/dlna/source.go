package dlna

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// PrepareMP3 checks that the platform honored the requested format. A server
// that cannot serve byte ranges is staged so the renderer can seek.
func (p *Proxy) PrepareMP3(ctx context.Context, src StreamSource, dst io.Writer) (size int64, staged bool, err error) {
	resp, err := src.upstream().Open(ctx, p.client, http.MethodGet, http.Header{
		"Accept-Encoding": {"identity"},
		"Range":           {"bytes=0-4095"},
	})
	if err != nil {
		if ctx.Err() != nil {
			return 0, false, ctx.Err()
		}
		return 0, false, errors.New("无法连接平台的兼容音源")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, false, fmt.Errorf("平台音源返回 HTTP %d", resp.StatusCode)
	}
	body := bufio.NewReader(resp.Body)
	head, err := body.Peek(4)
	if err != nil {
		return 0, false, errors.New("平台未返回完整的音频数据")
	}
	isMP3 := string(head[:3]) == "ID3" ||
		head[0] == 0xff && head[1]&0xe0 == 0xe0 && head[1]&0x18 != 0x08 &&
			head[1]&0x06 == 0x02 && head[2]&0xf0 != 0xf0 && head[2]&0x0c != 0x0c
	if !isMP3 {
		return 0, false, errors.New("平台没有返回请求的 MP3 格式，请确认服务端已启用转码")
	}
	if resp.StatusCode == http.StatusPartialContent {
		if !strings.HasPrefix(resp.Header.Get("Content-Range"), "bytes 0-") {
			return 0, false, errors.New("平台返回了无效的音频范围")
		}
		_, total, _ := strings.Cut(resp.Header.Get("Content-Range"), "/")
		size, _ = strconv.ParseInt(total, 10, 64)
		return max(0, size), false, nil
	}
	const limit = 512 << 20
	size, err = io.Copy(dst, io.LimitReader(body, limit+1))
	if err != nil || size > limit || (resp.ContentLength >= 0 && size != resp.ContentLength) {
		return 0, false, errors.New("兼容音源下载不完整或超过体积上限")
	}
	return size, true, nil
}
