package fetch

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// filterProxy sits between Chromium and the network. Every connection the
// browser makes goes through it, including frames, preconnects and service
// workers, so a blocked domain is refused before any byte leaves.
type filterProxy struct {
	ln       net.Listener
	srv      *http.Server
	upstream *url.URL
	dialer   net.Dialer
	tr       *http.Transport
}

func startFilterProxy(upstream *url.URL) (*filterProxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start filter proxy: %w", err)
	}
	p := &filterProxy{ln: ln, upstream: upstream, dialer: net.Dialer{Timeout: 15 * time.Second}}
	p.tr = &http.Transport{Proxy: http.ProxyURL(upstream), DialContext: p.dialer.DialContext, ResponseHeaderTimeout: 30 * time.Second}
	if upstream == nil {
		p.tr.Proxy = nil
	}
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second}
	go p.srv.Serve(ln)
	return p, nil
}

func (p *filterProxy) URL() string { return "http://" + p.ln.Addr().String() }

func (p *filterProxy) Close() {
	p.srv.Close()
	p.tr.CloseIdleConnections()
}

func (p *filterProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Hostname()
	if r.Method == http.MethodConnect {
		host, _, _ = net.SplitHostPort(r.Host)
	}
	if IsBlockedHost(host) {
		http.Error(w, "blocked by Speaker Trail", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodConnect {
		p.tunnel(w, r)
		return
	}
	p.forward(w, r)
}

func (p *filterProxy) tunnel(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	upstream, err := p.dialTunnel(ctx, r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "no hijacking", http.StatusInternalServerError)
		return
	}
	client, buf, err := hj.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	go func() {
		if buf.Reader.Buffered() > 0 {
			io.CopyN(upstream, buf, int64(buf.Reader.Buffered()))
		}
		io.Copy(upstream, client)
		upstream.Close()
	}()
	io.Copy(client, upstream)
	client.Close()
}

// dialTunnel opens a raw connection to addr, through the upstream proxy when
// there is one.
func (p *filterProxy) dialTunnel(ctx context.Context, addr string) (net.Conn, error) {
	if p.upstream == nil {
		return p.dialer.DialContext(ctx, "tcp", addr)
	}
	conn, err := p.dialer.DialContext(ctx, "tcp", p.upstream.Host)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr)
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("upstream proxy: %s", resp.Status)
	}
	return conn, nil
}

var hopHeaders = []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"}

func (p *filterProxy) forward(w http.ResponseWriter, r *http.Request) {
	out := r.Clone(r.Context())
	out.RequestURI = ""
	for _, h := range hopHeaders {
		out.Header.Del(h)
	}
	resp, err := p.tr.RoundTrip(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, h := range hopHeaders {
		resp.Header.Del(h)
	}
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
