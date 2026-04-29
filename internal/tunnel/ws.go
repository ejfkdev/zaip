package tunnel

import (
	"sync"

	"github.com/gorilla/websocket"
)

// wsStream wraps a WebSocket connection as io.ReadWriteCloser for smux.
// Each Write sends a WebSocket binary message; Read returns bytes from buffered messages.
type wsStream struct {
	conn    *websocket.Conn
	readBuf []byte
	readOff int
	wmu     sync.Mutex
}

func newWSStream(conn *websocket.Conn) *wsStream {
	return &wsStream{conn: conn}
}

func (w *wsStream) Read(p []byte) (n int, err error) {
	if w.readOff < len(w.readBuf) {
		n = copy(p, w.readBuf[w.readOff:])
		w.readOff += n
		return n, nil
	}
	_, msg, err := w.conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	w.readBuf = msg
	w.readOff = 0
	n = copy(p, w.readBuf)
	w.readOff = n
	return n, nil
}

func (w *wsStream) Write(p []byte) (n int, err error) {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	err = w.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *wsStream) Close() error {
	return w.conn.Close()
}
