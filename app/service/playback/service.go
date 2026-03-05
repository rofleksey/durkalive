package playback

import (
	"context"
	"durkalive/app/config"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/samber/do"
)

const (
	pageHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>TTS Playback</title>
<style>
body { margin: 0; padding: 0; background: #00ff00; }
</style>
</head>
<body>
<script>
(function() {
	var ws = new WebSocket((location.protocol === 'https:' ? 'wss:' : 'ws:') + '//' + location.host + '/ws');
	ws.binaryType = 'arraybuffer';
	ws.onmessage = function(ev) {
		if (ev.data instanceof ArrayBuffer && ev.data.byteLength > 0) {
			var blob = new Blob([ev.data], { type: 'audio/wav' });
			var url = URL.createObjectURL(blob);
			var audio = new Audio(url);
			audio.onended = function() { URL.revokeObjectURL(url); };
			audio.play();
		}
	};
	ws.onclose = function() { setTimeout(function() { location.reload(); }, 2000); };
})();
</script>
</body>
</html>
`
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Service struct {
	cfg     *config.Config
	clients map[*websocket.Conn]struct{}
	mu      sync.RWMutex
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)
	return &Service{
		cfg:     cfg,
		clients: make(map[*websocket.Conn]struct{}),
	}, nil
}

func (s *Service) BroadcastWAV(wav []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for conn := range s.clients {
		if err := conn.WriteMessage(websocket.BinaryMessage, wav); err != nil {
			slog.Debug("playback write error", "error", err)
		}
	}
}

func (s *Service) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.servePage)
	mux.HandleFunc("/ws", s.serveWS)

	srv := &http.Server{Addr: s.cfg.Webserver.Listen, Handler: mux}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	slog.Info("Playback server listening", "addr", s.cfg.Webserver.Listen)
	return srv.ListenAndServe()
}

func (s *Service) servePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/tts" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(pageHTML))
}

func (s *Service) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("websocket upgrade failed", "error", err)
		return
	}
	s.mu.Lock()
	s.clients[conn] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
	}()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
