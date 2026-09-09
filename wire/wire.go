// wire is the shared framing protocol between the tunnel server (plugin)
// and the tunnel client.
package wire

import (
	"encoding/binary"
	"encoding/json"
	"io"
)

const maxFrame = 1 << 20

func WriteFrame(w io.Writer, p []byte) error { var hdr [4]byte; binary.BigEndian.PutUint32(hdr[:],uint32(len(p))); if _,err:=w.Write(hdr[:]);err!=nil{return err}; if len(p)==0{return nil}; _,err:=w.Write(p); return err }
func ReadFrame(r io.Reader)([]byte,error){var hdr [4]byte;if _,err:=io.ReadFull(r,hdr[:]);err!=nil{return nil,err};n:=binary.BigEndian.Uint32(hdr[:]);if n==0{return nil,nil};if n>maxFrame{return nil,io.ErrUnexpectedEOF};buf:=make([]byte,n);if _,err:=io.ReadFull(r,buf);err!=nil{return nil,err};return buf,nil}
func WriteJSON(w io.Writer,v any)error{raw,err:=json.Marshal(v);if err!=nil{return err};return WriteFrame(w,raw)}
func ReadJSON(r io.Reader,v any)error{raw,err:=ReadFrame(r);if err!=nil{return err};return json.Unmarshal(raw,v)}
func WriteBody(w io.Writer,r io.Reader)error{buf:=make([]byte,32<<10);for{n,err:=r.Read(buf);if n>0{if e:=WriteFrame(w,buf[:n]);e!=nil{return e}};if err==io.EOF{break};if err!=nil{return err}};return WriteFrame(w,nil)}
func ReadBody(w io.Writer,r io.Reader)error{for{frame,err:=ReadFrame(r);if err!=nil{return err};if frame==nil{return nil};if _,err:=w.Write(frame);err!=nil{return err}}}
func PumpRawToFrames(dst io.Writer,src io.Reader)error{buf:=make([]byte,32<<10);for{n,err:=src.Read(buf);if n>0{if e:=WriteFrame(dst,buf[:n]);e!=nil{return e}};if err!=nil{return err}}}
func PumpFramesToRaw(dst io.Writer,src io.Reader)error{for{frame,err:=ReadFrame(src);if err!=nil{return err};if frame==nil{return nil};if _,err:=dst.Write(frame);err!=nil{return err}}}

type AuthReq struct { Token string `json:"token"`; Version string `json:"version,omitempty"`; Hostname string `json:"hostname,omitempty"`; OS string `json:"os,omitempty"`; Arch string `json:"arch,omitempty"`; ConnectorID string `json:"connector_id,omitempty"` }
type AuthResp struct { OK bool `json:"ok"`; Error string `json:"error,omitempty"`; TunnelID string `json:"tunnel_id,omitempty"`; ConnectorID string `json:"connector_id,omitempty"` }
type RequestHead struct { Target string `json:"target"`; Method string `json:"method"`; URL string `json:"url"`; Host string `json:"host"`; Headers map[string]string `json:"headers"`; IsWebSocket bool `json:"is_websocket"`; SkipTLSVerify bool `json:"skip_tls_verify"` }
type ResponseHead struct { Status int `json:"status"`; Headers map[string]string `json:"headers"` }
