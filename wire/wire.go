// wire is the shared framing protocol between the tunnel server (plugin)
// and the tunnel client. Everything that crosses a yamux stream is framed:
// 4-byte big-endian length, then the payload. JSON headers ride on top of
// frames; bodies are streamed as frames with a single empty frame as EOF.
package wire

import (
	"encoding/binary"
	"encoding/json"
	"io"
)

const maxFrame = 1 << 20 // 1 MiB cap so a misbehaving peer can't OOM us

// WriteFrame writes one length-prefixed chunk. An empty payload is the
// end-of-stream marker.
func WriteFrame(w io.Writer, p []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(p)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(p) == 0 {
		return nil
	}
	_, err := w.Write(p)
	return err
}

// ReadFrame reads one chunk. A nil result is the end-of-stream marker.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 {
		return nil, nil
	}
	if n > maxFrame {
		return nil, io.ErrUnexpectedEOF
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func WriteJSON(w io.Writer, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return WriteFrame(w, raw)
}

func ReadJSON(r io.Reader, v any) error {
	raw, err := ReadFrame(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// WriteBody streams a reader as frames, ending with the empty-frame marker.
func WriteBody(w io.Writer, r io.Reader) error {
	buf := make([]byte, 32<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if e := WriteFrame(w, buf[:n]); e != nil {
				return e
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return WriteFrame(w, nil)
}

// ReadBody drains framed chunks into w until the empty-frame marker.
func ReadBody(w io.Writer, r io.Reader) error {
	for {
		frame, err := ReadFrame(r)
		if err != nil {
			return err
		}
		if frame == nil {
			return nil
		}
		if _, err := w.Write(frame); err != nil {
			return err
		}
	}
}

// PumpRawToFrames reads raw bytes and re-emits them as frames. Used for
// websockets, where there is no natural EOF marker.
func PumpRawToFrames(dst io.Writer, src io.Reader) error {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if e := WriteFrame(dst, buf[:n]); e != nil {
				return e
			}
		}
		if err != nil {
			return err
		}
	}
}

func PumpFramesToRaw(dst io.Writer, src io.Reader) error {
	for {
		frame, err := ReadFrame(src)
		if err != nil {
			return err
		}
		if frame == nil {
			return nil
		}
		if _, err := dst.Write(frame); err != nil {
			return err
		}
	}
}

// AuthReq is sent by the client on the first stream right after yamux is up.
type AuthReq struct {
	Token string `json:"token"`
}

// AuthResp tells the client whether the token was accepted.
type AuthResp struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	TunnelID string `json:"tunnel_id,omitempty"`
}

// RequestHead is what the server pushes down a fresh data stream for each
// incoming HTTP request the ingress receives.
type RequestHead struct {
	Target      string            `json:"target"`        // client-local upstream, e.g. http://127.0.0.1:3000
	Method      string            `json:"method"`
	URL         string            `json:"url"`           // path + query
	Host        string            `json:"host"`
	Headers     map[string]string `json:"headers"`
	IsWebSocket bool              `json:"is_websocket"`
}

// ResponseHead is the client's status line + headers back to the server.
type ResponseHead struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
}
