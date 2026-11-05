package dlna

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StreamSource provides audio data or a remote URL for a track being cast to DLNA.
type StreamSource struct {
	// RemoteURL is the original remote audio stream URL, if any.
	RemoteURL string
	// Headers are extra HTTP headers required by the remote server (e.g. Auth).
	Headers http.Header
	// LocalFile is the path to a fully cached local file, or "".
	LocalFile string
	// ContentType is the MIME type (e.g. "audio/flac", "audio/mpeg").
	ContentType string
	// ContentLength is the file size in bytes, or 0 if unknown.
	ContentLength int64
}

// Proxy is a lightweight local HTTP streaming proxy for DLNA media renderers.
// DLNA devices need a direct, non-authenticated LAN URL supporting HTTP Range
// requests to play tracks and seek within them.
type Proxy struct {
	mu            sync.RWMutex
	listener      net.Listener
	server        *http.Server
	lanIP         string
	port          int
	sources       map[string]StreamSource
	covers        map[string]string
	activeCancels map[string]context.CancelFunc
	activeBodies  map[string]io.Closer
	client        *http.Client
}

// NewProxy creates and starts a local DLNA media streaming proxy on an available port.
func NewProxy() (*Proxy, error) {
	// Bind to all interfaces on an ephemeral port.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("dlna proxy: listen: %w", err)
	}

	lanIP := detectLANIP()
	p := &Proxy{
		listener:      l,
		lanIP:         lanIP,
		port:          l.Addr().(*net.TCPAddr).Port,
		sources:       make(map[string]StreamSource),
		covers:        make(map[string]string),
		activeCancels: make(map[string]context.CancelFunc),
		activeBodies:  make(map[string]io.Closer),
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   15 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stream/", p.handleStream)
	mux.HandleFunc("/cover/", p.handleCover)

	p.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Streaming responses can last for the entire song
	}

	go func() {
		_ = p.server.Serve(l)
	}()

	return p, nil
}

// detectLANIP discovers the local network IP that can reach local devices.
func detectLANIP() string {
	// 1. Try public DNS endpoints to query the OS routing table for the primary physical gateway.
	for _, target := range []string{"223.5.5.5:80", "119.29.29.29:80", "8.8.8.8:80"} {
		conn, err := net.DialTimeout("udp", target, 400*time.Millisecond)
		if err == nil {
			localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
			conn.Close()
			if ok && !localAddr.IP.IsLoopback() && localAddr.IP.To4() != nil {
				return localAddr.IP.String()
			}
		}
	}

	// 2. Fallback to searching physical network interfaces (ignoring WSL / Hyper-V / VMware virtual adapters).
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			name := strings.ToLower(iface.Name)
			if strings.Contains(name, "vethernet") || strings.Contains(name, "wsl") ||
				strings.Contains(name, "vmware") || strings.Contains(name, "virtualbox") ||
				strings.Contains(name, "hyper-v") || strings.Contains(name, "docker") ||
				strings.Contains(name, "tailscale") || strings.Contains(name, "tun") ||
				strings.Contains(name, "tap") || strings.Contains(name, "wg") {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
					return ip.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

// RegisterCover associates an album cover with a track ID.
func (p *Proxy) RegisterCover(trackID string, coverURL string) {
	if trackID == "" || coverURL == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.covers[trackID] = coverURL
}

// LANCoverURLFor returns the LAN cover URL reachable by targetURL.
func (p *Proxy) LANCoverURLFor(trackID, targetURL string) string {
	ip := p.detectLANIPFor(targetURL)
	return fmt.Sprintf("http://%s:%d/cover/%s", ip, p.port, trackID)
}

// Register registers or updates a stream source for a track, returning its DLNA LAN URL.
func (p *Proxy) Register(trackID string, src StreamSource) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sources[trackID] = src
	return fmt.Sprintf("http://%s:%d/stream/%s", p.lanIP, p.port, trackID)
}

// LocalURL returns the 127.0.0.1 loopback URL for a registered track (ideal for local players like MPV).
func (p *Proxy) LocalURL(trackID string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/stream/%s", p.port, trackID)
}

// LANURLFor returns the LAN streaming URL using the specific interface IP reachable by targetURL.
// Optional ext appends a file extension (e.g. "mp3" or "flac") which many DLNA devices require.
func (p *Proxy) LANURLFor(trackID, targetURL string, ext ...string) string {
	ip := p.detectLANIPFor(targetURL)
	fileExt := ""
	if len(ext) > 0 && ext[0] != "" {
		fileExt = "." + strings.TrimPrefix(ext[0], ".")
	}
	return fmt.Sprintf("http://%s:%d/stream/%s%s", ip, p.port, trackID, fileExt)
}

func (p *Proxy) detectLANIPFor(targetURL string) string {
	if u, err := url.Parse(targetURL); err == nil && u.Host != "" {
		host := u.Host
		if !strings.Contains(host, ":") {
			host += ":80"
		}
		conn, err := net.DialTimeout("udp", host, 500*time.Millisecond)
		if err == nil {
			defer conn.Close()
			if localAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok && !localAddr.IP.IsLoopback() && localAddr.IP.To4() != nil {
				return localAddr.IP.String()
			}
		}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lanIP
}

// Unregister removes a stream source.
func (p *Proxy) Unregister(trackID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sources, trackID)
	if closer, ok := p.activeBodies[trackID]; ok && closer != nil {
		_ = closer.Close()
		delete(p.activeBodies, trackID)
	}
	if cancel, ok := p.activeCancels[trackID]; ok && cancel != nil {
		cancel()
		delete(p.activeCancels, trackID)
	}
}

func buildContentFeatures(mimeType string) string {
	var cf strings.Builder
	switch mimeType {
	case "audio/mpeg":
		cf.WriteString("DLNA.ORG_PN=MP3;")
	}
	cf.WriteString("DLNA.ORG_OP=01;")
	cf.WriteString("DLNA.ORG_CI=0;")
	cf.WriteString("DLNA.ORG_FLAGS=01700000000000000000000000000000")
	return cf.String()
}

func (p *Proxy) handleStream(w http.ResponseWriter, r *http.Request) {
	rawID := r.URL.Path[len("/stream/"):]
	p.mu.RLock()
	src, ok := p.sources[rawID]
	key := rawID
	if !ok {
		idx := strings.LastIndex(rawID, ".")
		if idx > 0 {
			key = rawID[:idx]
			src, ok = p.sources[key]
		}
	}
	p.mu.RUnlock()

	if !ok {
		http.Error(w, "stream not found", http.StatusNotFound)
		return
	}

	// Always emit DLNA compliance headers on all responses (GET, HEAD, 200, 206)
	w.Header().Set("Accept-Ranges", "bytes")
	if cf := buildContentFeatures(src.ContentType); cf != "" {
		w.Header().Set("contentFeatures.dlna.org", cf)
	}
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("Connection", "close")

	// 1. If we have a local cached file, serve it directly with range support.
	if src.LocalFile != "" {
		if src.ContentType != "" {
			w.Header().Set("Content-Type", src.ContentType)
		}
		http.ServeFile(w, r, src.LocalFile)
		return
	}

	// 2. Otherwise proxy the remote stream, forwarding auth headers and range requests.
	if src.RemoteURL == "" {
		http.Error(w, "no source available", http.StatusNotFound)
		return
	}

	client := p.client
	if client == nil {
		client = http.DefaultClient
	}

	// Handle HEAD requests specifically to prevent 403 / 405 rejections from CDNs
	if r.Method == http.MethodHead {
		headReq, _ := http.NewRequestWithContext(r.Context(), http.MethodHead, src.RemoteURL, nil)
		for k, vv := range src.Headers {
			for _, v := range vv {
				headReq.Header.Add(k, v)
			}
		}
		headResp, err := client.Do(headReq)
		if err == nil && headResp.StatusCode >= 200 && headResp.StatusCode < 300 {
			defer headResp.Body.Close()
			for k, vv := range headResp.Header {
				for _, v := range vv {
					if !strings.EqualFold(k, "Content-Type") && !strings.EqualFold(k, "Connection") {
						w.Header().Add(k, v)
					}
				}
			}
			if src.ContentType != "" {
				w.Header().Set("Content-Type", src.ContentType)
			}
			w.WriteHeader(headResp.StatusCode)
			return
		}
		if headResp != nil {
			_ = headResp.Body.Close()
		}

		// Fallback probe for CDNs rejecting HEAD: send Range: 0-0 GET to learn content length
		probeReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, src.RemoteURL, nil)
		for k, vv := range src.Headers {
			for _, v := range vv {
				probeReq.Header.Add(k, v)
			}
		}
		probeReq.Header.Set("Range", "bytes=0-0")
		probeResp, err := client.Do(probeReq)
		if err == nil {
			defer probeResp.Body.Close()
			if cr := probeResp.Header.Get("Content-Range"); cr != "" {
				if _, total, ok := strings.Cut(cr, "/"); ok && total != "*" {
					w.Header().Set("Content-Length", total)
				}
			} else if cl := probeResp.Header.Get("Content-Length"); cl != "" {
				w.Header().Set("Content-Length", cl)
			}
			if src.ContentType != "" {
				w.Header().Set("Content-Type", src.ContentType)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Cancel and close any previous in-flight upstream connection for this track.
	p.mu.Lock()
	if oldCloser, exists := p.activeBodies[key]; exists && oldCloser != nil {
		_ = oldCloser.Close()
		delete(p.activeBodies, key)
	}
	if oldCancel, exists := p.activeCancels[key]; exists && oldCancel != nil {
		oldCancel()
		delete(p.activeCancels, key)
	}

	ctx, cancel := context.WithCancel(r.Context())
	p.activeCancels[key] = cancel
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		if p.activeCancels[key] != nil {
			delete(p.activeCancels, key)
		}
		if p.activeBodies[key] != nil {
			delete(p.activeBodies, key)
		}
		p.mu.Unlock()
		cancel()
	}()

	req, err := http.NewRequestWithContext(ctx, r.Method, src.RemoteURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for k, vv := range src.Headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	p.mu.Lock()
	p.activeBodies[key] = resp.Body
	p.mu.Unlock()

	for k, vv := range resp.Header {
		for _, v := range vv {
			if strings.EqualFold(k, "Content-Type") && src.ContentType != "" {
				continue
			}
			if strings.EqualFold(k, "Connection") {
				continue
			}
			w.Header().Add(k, v)
		}
	}
	if src.ContentType != "" {
		w.Header().Set("Content-Type", src.ContentType)
	}
	w.WriteHeader(resp.StatusCode)

	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, resp.Body)
	}
}

// Close shuts down the proxy server.
func (p *Proxy) Close() error {
	p.mu.Lock()
	for _, closer := range p.activeBodies {
		if closer != nil {
			_ = closer.Close()
		}
	}
	for _, cancel := range p.activeCancels {
		if cancel != nil {
			cancel()
		}
	}
	p.activeBodies = make(map[string]io.Closer)
	p.activeCancels = make(map[string]context.CancelFunc)
	p.mu.Unlock()
	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return p.server.Shutdown(ctx)
	}
	return nil
}

// Port returns the listening port.
func (p *Proxy) Port() int {
	return p.port
}

func (p *Proxy) handleCover(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/cover/")
	p.mu.RLock()
	coverURL := p.covers[key]
	p.mu.RUnlock()

	if coverURL == "" {
		http.NotFound(w, r)
		return
	}

	// 1. If coverURL is a local file on disk, serve it directly.
	if strings.HasPrefix(coverURL, "/") || (len(coverURL) > 3 && coverURL[1] == ':') {
		http.ServeFile(w, r, coverURL)
		return
	}

	// 2. If coverURL is a remote HTTP URL, proxy it with timeout and caching.
	req, err := http.NewRequestWithContext(r.Context(), "GET", coverURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "image/jpeg")
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// LANIP returns the discovered LAN IP.
func (p *Proxy) LANIP() string {
	return p.lanIP
}

// SetLANIP overrides the discovered LAN IP (useful for tests or custom routing).
func (p *Proxy) SetLANIP(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lanIP = ip
}

// FormatToMIME converts common audio format extensions to DLNA-friendly MIME types.
func FormatToMIME(format string) string {
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "flac":
		return "audio/flac"
	case "mp3":
		return "audio/mpeg"
	case "m4a", "aac":
		return "audio/mp4"
	case "wav":
		return "audio/wav"
	case "ogg", "vorbis":
		return "audio/ogg"
	case "opus":
		return "audio/opus"
	case "dsf", "dff", "dsd":
		return "audio/x-dsd"
	case "ape":
		return "audio/x-ape"
	default:
		return "audio/mpeg"
	}
}

func parseDuration(s string) time.Duration {
	// Parse UPnP RelTime or TrackDuration format: "H:MM:SS" or "H:MM:SS.mmm"
	var h, m int
	var sec float64
	_, err := fmt.Sscanf(s, "%d:%d:%f", &h, &m, &sec)
	if err == nil {
		return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec*float64(time.Second))
	}
	// Try fallback to integer seconds
	if secInt, err := strconv.Atoi(s); err == nil {
		return time.Duration(secInt) * time.Second
	}
	return 0
}
