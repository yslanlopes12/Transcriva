package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed static/*
var staticFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Permite conexões locais
	},
}

// HistoryItem representa uma gravação salva
type HistoryItem struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	TxtPath string `json:"txtPath"`
	WavPath string `json:"wavPath"`
}

// Server gerencia o servidor HTTP e WebSockets da UI
type Server struct {
	port          int
	mu            sync.Mutex
	clients       map[*websocket.Conn]bool
	
	// Handlers injetados pela App
	OnStart         func(saveWav bool) error
	OnStop          func()
	GetDeviceInfo   func() string
	GetHistory      func() []HistoryItem
	OnDeleteHistory func(id string)
	GetState        func() bool
}

// NewServer cria uma nova instância do servidor web UI
func NewServer(port int) *Server {
	return &Server{
		port:    port,
		clients: make(map[*websocket.Conn]bool),
	}
}

// Start inicia o servidor na porta configurada e abre o navegador
func (s *Server) Start() error {
	// Extrai a subpasta "static" do sistema de arquivos embutido
	subFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("falha ao carregar arquivos estáticos: %w", err)
	}

	mux := http.NewServeMux()
	
	// Serve os arquivos estáticos diretamente na raiz
	mux.Handle("/", http.FileServer(http.FS(subFS)))
	mux.HandleFunc("/ws", s.handleWebSocket)
	
	// Rota para download de arquivos
	mux.HandleFunc("/download", s.handleDownload)

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	fmt.Printf("\n[GUI] Interface gráfica rodando em: http://%s\n", addr)
	fmt.Println("[GUI] Abra o link no seu navegador ou aguarde ele abrir automaticamente.")

	// Abre o navegador automaticamente
	go s.openBrowser("http://" + addr)

	return http.ListenAndServe(addr, mux)
}

// handleDownload permite baixar os arquivos gerados (txt e wav)
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")
	if filePath == "" {
		http.Error(w, "Arquivo não especificado", http.StatusBadRequest)
		return
	}

	// Segurança básica para não permitir download de arquivos fora do esperado
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(filePath))
	http.ServeFile(w, r, filePath)
}

// handleWebSocket gerencia a conexão bidirecional com o frontend
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Falha no upgrade websocket:", err)
		return
	}

	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	// Envia informações iniciais (ex: nome do dispositivo)
	if s.GetDeviceInfo != nil {
		deviceName := s.GetDeviceInfo()
		s.sendJSON(conn, map[string]interface{}{
			"type": "device_info",
			"data": map[string]string{
				"name": deviceName,
			},
		})
	}

	// Envia histórico inicial
	if s.GetHistory != nil {
		s.sendJSON(conn, map[string]interface{}{
			"type": "history",
			"data": s.GetHistory(),
		})
	}

	// Sincroniza estado de gravação caso tenha reconectado no meio da reunião
	if s.GetState != nil {
		isRecording := s.GetState()
		s.sendJSON(conn, map[string]interface{}{
			"type": "status",
			"data": map[string]bool{"isRecording": isRecording},
		})
	}

	// Loop para escutar comandos do Frontend (Iniciar/Parar)
	for {
		_, msgData, err := conn.ReadMessage()
		if err != nil {
			break // cliente desconectou
		}

		var cmd struct {
			Action  string `json:"action"`
			SaveWav bool   `json:"saveWav"`
			ID      string `json:"id"`
		}

		if err := json.Unmarshal(msgData, &cmd); err != nil {
			continue
		}

		switch cmd.Action {
		case "start":
			if s.OnStart != nil {
				go func() {
					err := s.OnStart(cmd.SaveWav)
					if err != nil {
						s.BroadcastError(err.Error())
					}
				}()
			}
		case "stop":
			if s.OnStop != nil {
				go s.OnStop()
			}
		case "delete_history":
			if s.OnDeleteHistory != nil && cmd.ID != "" {
				go s.OnDeleteHistory(cmd.ID)
			}
		}
	}
}

// sendJSON envia JSON para uma conexão específica
func (s *Server) sendJSON(conn *websocket.Conn, v interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn.WriteJSON(v)
}

// BroadcastHistory atualiza a lista do frontend com o histórico recente
func (s *Server) BroadcastHistory(history []HistoryItem) {
	s.broadcast(map[string]interface{}{
		"type": "history",
		"data": history,
	})
}

// BroadcastState atualiza o frontend sobre o estado (rodando ou parado)
func (s *Server) BroadcastState(isRecording bool) {
	msg := map[string]interface{}{
		"type": "status",
		"data": map[string]bool{
			"isRecording": isRecording,
		},
	}
	s.broadcast(msg)
}

// BroadcastSegment envia um novo segmento transcrito para o frontend
func (s *Server) BroadcastSegment(relativeMs int64, text string) {
	msg := map[string]interface{}{
		"type": "segment",
		"data": map[string]interface{}{
			"timestamp": relativeMs,
			"text":      text,
		},
	}
	s.broadcast(msg)
}

// BroadcastResult envia o caminho dos arquivos gerados após finalizar
func (s *Server) BroadcastResult(txtPath, wavPath string) {
	msg := map[string]interface{}{
		"type": "result",
		"data": map[string]string{
			"txtPath": txtPath,
			"wavPath": wavPath,
		},
	}
	s.broadcast(msg)
}

// BroadcastError envia um erro para ser mostrado no frontend
func (s *Server) BroadcastError(errMessage string) {
	msg := map[string]interface{}{
		"type": "error",
		"data": map[string]string{
			"message": errMessage,
		},
	}
	s.broadcast(msg)
}

// broadcast envia a mensagem para todos os clientes websocket conectados
func (s *Server) broadcast(msg interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.clients {
		err := conn.WriteJSON(msg)
		if err != nil {
			conn.Close()
			delete(s.clients, conn)
		}
	}
}

// openBrowser abre a URL no navegador padrão do SO
func (s *Server) openBrowser(url string) {
	// Aguarda um instante pro server subir
	time.Sleep(1 * time.Second)

	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	default:
		err = fmt.Errorf("sistema operacional não suportado")
	}
	
	if err != nil {
		log.Printf("Aviso: não foi possível abrir o navegador automaticamente. Acesse %s manualmente.", url)
	}
}
