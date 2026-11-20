package app

import (
	"context"
	"io"
	"net"

	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/platformrpc"
	"github.com/waygu-music-plugin/sdk/protocol"
)

func (p *pluginProcess) callMediaStream(ctx context.Context, in PluginCall) (PluginMedia, error) {
	slots := p.media.ForAction(in.Action)
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return PluginMedia{}, ctx.Err()
	}
	accepted := make(chan struct{})
	defer func() {
		select {
		case <-accepted:
			<-slots
		default:
			go func() {
				<-accepted
				<-slots
			}()
		}
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	id, accept := p.transport.ReserveStream()
	in.MediaStream = id
	connections := make(chan net.Conn)
	go func() {
		defer close(accepted)
		defer close(connections)
		conn, err := accept()
		if err != nil {
			return
		}
		select {
		case connections <- conn:
		case <-ctx.Done():
			_ = conn.Close()
		}
	}()

	var wireReply protocol.PluginMedia
	if err := p.transport.Call(ctx, "Media", in.Wire(), &wireReply); err != nil {
		return PluginMedia{}, err
	}
	reply := bridge.LocalMedia(wireReply)
	if err := reply.Failure.Err(); err != nil {
		return reply, err
	}
	if reply.CoverSize > bridge.MaxPluginCoverBytes {
		return PluginMedia{}, platformrpc.Failure{Code: "invalid-media-stream"}
	}
	select {
	case conn, ok := <-connections:
		if !ok {
			return PluginMedia{}, platformrpc.Failure{Code: "missing-media-stream"}
		}
		defer conn.Close()
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()
		if reply.CoverSize > 0 {
			reply.Cover = make([]byte, reply.CoverSize)
			if _, err := io.ReadFull(conn, reply.Cover); err != nil {
				if ctx.Err() != nil {
					return PluginMedia{}, ctx.Err()
				}
				return PluginMedia{}, err
			}
		}
		return reply, ctx.Err()
	case <-ctx.Done():
		return PluginMedia{}, ctx.Err()
	}
}
