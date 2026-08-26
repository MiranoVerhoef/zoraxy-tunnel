package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"zoraxy-tunnel/wire"
)

func newUpstreamClient(skipTLSVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: skipTLSVerify}
	return &http.Client{
		Timeout:   0, // requests can be long-lived; tunnel handles its own liveness
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // never rewrite — the browser sees the real 3xx
		},
	}
}

var (
	upstreamClient         = newUpstreamClient(false)
	insecureUpstreamClient = newUpstreamClient(true)
)

// handleStream is spawned per yamux stream the server opens. It reads the
// request head and dispatches to the HTTP or websocket relay.
func handleStream(stream io.ReadWriteCloser) {
	defer stream.Close()
	var head wire.RequestHead
	if err := wire.ReadJSON(stream, &head); err != nil {
		return
	}
	if head.IsWebSocket {
		relayWebsocket(stream, head)
		return
	}
	relayHTTP(stream, head)
}

func relayHTTP(stream io.ReadWriteCloser, head wire.RequestHead) {
	reqURL := head.Target + head.URL
	req, err := http.NewRequest(head.Method, reqURL, &frameBodyReader{r: stream})
	if err != nil {
		writeErr(stream, http.StatusBadGateway, err.Error())
		return
	}
	req.Host = head.Host
	for k, v := range head.Headers {
		if strings.EqualFold(k, "Host") {
			continue
		}
		req.Header.Set(k, v)
	}

	client := upstreamClient
	if head.SkipTLSVerify {
		client = insecureUpstreamClient
	}
	resp, err := client.Do(req)
	if err != nil {
		writeErr(stream, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	_ = wire.WriteJSON(stream, wire.ResponseHead{
		Status:  resp.StatusCode,
		Headers: headerMap(resp.Header),
	})
	_ = wire.WriteBody(stream, resp.Body)
}

// relayWebsocket dials the upstream as a raw socket, replays the upgrade
// request, then bridges the socket against the framed yamux stream.
func relayWebsocket(stream io.ReadWriteCloser, head wire.RequestHead) {
	u, err := url.Parse(head.Target)
	if err != nil {
		writeErr(stream, http.StatusBadGateway, "bad target")
		return
	}
	host := u.Host
	if _, _, e := net.SplitHostPort(host); e != nil {
		if strings.EqualFold(u.Scheme, "https") {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	var conn net.Conn
	if strings.EqualFold(u.Scheme, "https") {
		conn, err = tls.Dial("tcp", host, &tls.Config{InsecureSkipVerify: head.SkipTLSVerify})
	} else {
		conn, err = net.DialTimeout("tcp", host, 10*time.Second)
	}
	if err != nil {
		writeErr(stream, http.StatusBadGateway, err.Error())
		return
	}
	defer conn.Close()

	// replay the browser's upgrade request verbatim to the upstream
	fmt.Fprintf(conn, "%s %s HTTP/1.1\r\n", head.Method, head.URL)
	for k, v := range head.Headers {
		fmt.Fprintf(conn, "%s: %s\r\n", k, v)
	}
	if _, ok := headerHas(head.Headers, "Host"); !ok {
		fmt.Fprintf(conn, "Host: %s\r\n", head.Host)
	}
	fmt.Fprint(conn, "\r\n")

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		writeErr(stream, http.StatusBadGateway, "upstream closed")
		return
	}
	respHeaders := map[string]string{}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			writeErr(stream, http.StatusBadGateway, "truncated response")
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if i := strings.Index(line, ":"); i > 0 {
			respHeaders[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
		}
	}
	status := parseStatus(statusLine)
	_ = wire.WriteJSON(stream, wire.ResponseHead{Status: status, Headers: respHeaders})

	// anything the bufio.Reader already pulled past the headers (e.g. an early
	// server frame) must still reach the browser — but we must NOT block on the
	// live socket, so only drain what is buffered, nothing more.
	var prebuffered []byte
	if n := br.Buffered(); n > 0 {
		peek, _ := br.Peek(n)
		prebuffered = append(prebuffered, peek...)
		_, _ = br.Discard(n)
	}
	bridge := newBiPipe(conn, prebuffered, stream)
	bridge.run()
}

// writeErr pushes a synthetic error response back to the server so the end
// user sees a real status instead of a dead stream.
func writeErr(stream io.Writer, status int, msg string) {
	_ = wire.WriteJSON(stream, wire.ResponseHead{
		Status: status,
		Headers: map[string]string{
			"Content-Type": "text/plain; charset=utf-8",
		},
	})
	body := []byte(msg + "\n")
	_ = wire.WriteFrame(stream, body)
	_ = wire.WriteFrame(stream, nil) // end marker
}

type frameBodyReader struct {
	r   io.Reader
	buf []byte
	eof bool
}

func (f *frameBodyReader) Read(p []byte) (int, error) {
	if len(f.buf) > 0 {
		n := copy(p, f.buf)
		f.buf = f.buf[n:]
		return n, nil
	}
	if f.eof {
		return 0, io.EOF
	}
	frame, err := wire.ReadFrame(f.r)
	if err != nil {
		return 0, err
	}
	if frame == nil {
		f.eof = true
		return 0, io.EOF
	}
	f.buf = frame
	n := copy(p, f.buf)
	f.buf = f.buf[n:]
	return n, nil
}

func headerMap(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		out[k] = strings.Join(vs, ", ")
	}
	return out
}

func headerHas(h map[string]string, key string) (string, bool) {
	for k, v := range h {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

func parseStatus(line string) int {
	parts := strings.SplitN(strings.TrimSpace(line), " ", 3)
	if len(parts) < 2 {
		return 200
	}
	var code int
	fmt.Sscanf(parts[1], "%d", &code)
	if code == 0 {
		return 200
	}
	return code
}

// biPipe shuttles bytes between the upstream socket and the framed stream.
// The server side already de-frames, so here we frame toward the server and
// de-frame toward the socket.
type biPipe struct {
	conn    net.Conn
	initial []byte
	stream  io.ReadWriteCloser
}

func newBiPipe(conn net.Conn, initial []byte, stream io.ReadWriteCloser) *biPipe {
	return &biPipe{conn: conn, initial: initial, stream: stream}
}

func (b *biPipe) run() {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if len(b.initial) > 0 {
			_ = wire.WriteFrame(b.stream, b.initial)
		}
		_ = wire.PumpRawToFrames(b.stream, b.conn) // upstream -> browser
	}()
	go func() {
		defer wg.Done()
		_ = wire.PumpFramesToRaw(b.conn, b.stream) // browser -> upstream
	}()
	wg.Wait()
}
