// Package app contém a lógica principal da aplicação, orquestrando
// captura de áudio, transcrição e interface do usuário.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"transcriva/internal/audio"
	"transcriva/internal/transcription"
	"transcriva/internal/ui"
	"transcriva/internal/web"
	"transcriva/internal/whisper"
)

const (
	transcriptionsDir = "transcriptions"
	recordingsDir     = "recordings"
	modelsDir         = "models"
	binDir            = "bin"
)

// App é o ponto central da aplicação
type App struct {
	capturer    *audio.Capturer
	transcriber *whisper.Transcriber
	webServer   *web.Server
	terminal    *ui.TerminalUI
	session     *transcription.Session
}

// New inicializa a aplicação, carregando todos os componentes necessários
func New() (*App, error) {
	// Inicializa o capturador WASAPI
	capturer, err := audio.New()
	if err != nil {
		return nil, fmt.Errorf("captura de áudio indisponível:\n  %w\n\n"+
			"  Verifique se há dispositivos de saída habilitados no Windows.\n"+
			"  Painel de Controle → Som → Reprodução", err)
	}

	exeDir := getExeDir()
	binPath := resolvePath(exeDir, binDir, "whisper-cli.exe")
	modelPath := resolveModel(exeDir)

	transcriber, err := whisper.New(whisper.Config{
		BinPath:   binPath,
		ModelPath: modelPath,
		Language:  "pt",
		Threads:   runtime.NumCPU(),
	})
	if err != nil {
		return nil, fmt.Errorf("transcritor Whisper indisponível:\n  %w", err)
	}

	app := &App{
		capturer:    capturer,
		transcriber: transcriber,
		webServer:   web.NewServer(8080),
		terminal:    ui.New(),
	}

	// Injeta os callbacks da UI no servidor Web
	app.webServer.GetDeviceInfo = func() string {
		return app.capturer.DeviceInfo().Name
	}
	app.webServer.GetHistory = app.cleanupAndGetHistory
	app.webServer.OnDeleteHistory = app.deleteHistory
	app.webServer.OnStart = app.StartTranscriptionGUI
	app.webServer.OnStop = app.StopTranscriptionGUI

	return app, nil
}

// deleteHistory remove os arquivos de uma transcrição específica
func (a *App) deleteHistory(id string) {
	// Limpeza básica do ID para evitar directory traversal
	id = filepath.Base(id)
	
	txtPath := filepath.Join(transcriptionsDir, id+".txt")
	wavPath := filepath.Join(recordingsDir, id+".wav")
	
	os.Remove(txtPath)
	os.Remove(wavPath)
	
	a.webServer.BroadcastHistory(a.cleanupAndGetHistory())
}

// cleanupAndGetHistory varre os diretórios, apaga arquivos além dos 5 mais recentes e retorna o histórico
func (a *App) cleanupAndGetHistory() []web.HistoryItem {
	files, err := os.ReadDir(transcriptionsDir)
	if err != nil {
		return nil
	}

	type fileInfo struct {
		name    string
		modTime time.Time
	}
	
	var txtFiles []fileInfo
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".txt" {
			info, err := f.Info()
			if err == nil {
				txtFiles = append(txtFiles, fileInfo{f.Name(), info.ModTime()})
			}
		}
	}

	// Ordena do mais recente para o mais antigo
	sort.Slice(txtFiles, func(i, j int) bool {
		return txtFiles[i].modTime.After(txtFiles[j].modTime)
	})

	var history []web.HistoryItem
	maxItems := 5

	for i, f := range txtFiles {
		sessionID := strings.TrimSuffix(f.name, ".txt")
		txtPath := filepath.Join(transcriptionsDir, f.name)
		wavPath := filepath.Join(recordingsDir, sessionID+".wav")
		
		// Apaga arquivos excedentes
		if i >= maxItems {
			os.Remove(txtPath)
			os.Remove(wavPath)
			continue
		}
		
		hasWav := false
		if _, err := os.Stat(wavPath); err == nil {
			hasWav = true
		}

		item := web.HistoryItem{
			ID:      sessionID,
			Date:    f.modTime.Format("02/01/2006 15:04"),
			TxtPath: txtPath,
		}
		if hasWav {
			item.WavPath = wavPath
		}
		history = append(history, item)
	}
	return history
}

// RunGUI executa o servidor web principal (Interface Gráfica)
func (a *App) RunGUI() error {
	defer func() {
		if a.capturer != nil {
			a.capturer.Close()
		}
	}()
	return a.webServer.Start()
}

// RunCLI executa a interface de terminal
func (a *App) RunCLI() error {
	defer func() {
		if a.capturer != nil {
			a.capturer.Close()
		}
	}()

	saveWAV := true
	deviceName := "⚠️ Nenhum dispositivo encontrado"
	if a.capturer != nil {
		deviceName = a.capturer.DeviceInfo().Name
	}
	a.terminal.ShowWelcome(deviceName, "pt")

	for {
		key, err := ui.ReadKey()
		if err != nil {
			return fmt.Errorf("erro ao ler entrada: %w", err)
		}

		switch {
		case key == '\r' || key == '\n':
			saveWAV = true
			if err := a.startSessionCLI(saveWAV); err != nil {
				return err
			}
		case key == 'w' || key == 'W':
			saveWAV = false
			if err := a.startSessionCLI(saveWAV); err != nil {
				return err
			}
		case key == 'q' || key == 'Q' || key == 3: // 3 = Ctrl+C
			fmt.Println("\nSaindo...")
			return nil
		}
	}
}

// startSessionCLI inicia a transcrição no modo CLI
func (a *App) startSessionCLI(saveWAV bool) error {
	if a.capturer == nil {
		newCap, err := audio.New()
		if err != nil {
			return fmt.Errorf("não há dispositivos de áudio disponíveis para gravar. Conecte um fone/caixa de som e tente novamente")
		}
		a.capturer = newCap
	}

	os.MkdirAll(transcriptionsDir, 0755)
	if saveWAV {
		os.MkdirAll(recordingsDir, 0755)
	}

	a.session = transcription.New(transcription.Config{
		Capturer:          a.capturer,
		Transcriber:       a.transcriber,
		TranscriptionsDir: transcriptionsDir,
		RecordingsDir:     recordingsDir,
		SaveWAV:           saveWAV,
	})

	if err := a.session.Start(); err != nil {
		return fmt.Errorf("falha ao iniciar sessão: %w", err)
	}

	startTime := time.Now()
	a.terminal.SetRecording(true)
	a.terminal.StartTimer()
	a.terminal.ShowRecording()

	go func() {
		for seg := range a.session.SegmentChan() {
			a.terminal.AddSegment(seg.RelativeMs, seg.Text)
			a.terminal.ShowRecording()
		}
	}()

	// Aguarda input para parar
	for {
		key, err := ui.ReadKey()
		if err != nil {
			break
		}
		if key == '\r' || key == '\n' || key == 'q' || key == 'Q' || key == 3 {
			break
		}
	}

	a.terminal.SetRecording(false)
	a.terminal.StopTimer()
	a.terminal.ShowStopping()

	a.session.Stop()
	a.session.Wait()

	elapsed := time.Since(startTime)
	segments := a.session.Segments()
	txtPath, wavPath := a.session.OutputPaths()

	a.terminal.ShowFinalSummary(txtPath, wavPath, len(segments), elapsed)
	
	fmt.Println("  Pressione qualquer tecla para voltar ao menu...")
	ui.ReadKey()

	if a.capturer != nil {
		a.capturer.Close()
	}
	newCapturer, err := audio.New()
	if err == nil {
		a.capturer = newCapturer
	} else {
		a.capturer = nil
	}
	
	deviceName := "⚠️ Nenhum dispositivo encontrado"
	if a.capturer != nil {
		deviceName = a.capturer.DeviceInfo().Name
	}
	a.terminal = ui.New()
	a.terminal.ShowWelcome(deviceName, "pt")
	return nil
}

// StartTranscriptionGUI inicia o pipeline de transcrição para Web
func (a *App) StartTranscriptionGUI(saveWAV bool) error {
	if a.session != nil {
		return fmt.Errorf("sessão já está em andamento")
	}

	if a.capturer == nil {
		newCap, err := audio.New()
		if err != nil {
			return fmt.Errorf("Nenhum dispositivo de áudio conectado. Conecte um fone/alto-falante e tente novamente.")
		}
		a.capturer = newCap
		// Atualiza o nome na UI assim que conectar
		if a.webServer != nil {
			a.webServer.GetDeviceInfo = func() string { return a.capturer.DeviceInfo().Name }
		}
	}

	os.MkdirAll(transcriptionsDir, 0755)
	if saveWAV {
		os.MkdirAll(recordingsDir, 0755)
	}

	a.session = transcription.New(transcription.Config{
		Capturer:          a.capturer,
		Transcriber:       a.transcriber,
		TranscriptionsDir: transcriptionsDir,
		RecordingsDir:     recordingsDir,
		SaveWAV:           saveWAV,
	})

	if err := a.session.Start(); err != nil {
		a.session = nil
		return fmt.Errorf("falha ao iniciar sessão: %w", err)
	}

	a.webServer.BroadcastState(true)

	go func(currentSession *transcription.Session) {
		for seg := range currentSession.SegmentChan() {
			a.webServer.BroadcastSegment(seg.RelativeMs, seg.Text)
		}
	}(a.session)

	return nil
}

// StopTranscriptionGUI para a sessão e mostra resultado na Web
func (a *App) StopTranscriptionGUI() {
	if a.session == nil {
		return
	}

	currentSession := a.session
	a.webServer.BroadcastState(false)

	currentSession.Stop()
	currentSession.Wait()

	txtPath, wavPath := currentSession.OutputPaths()
	a.webServer.BroadcastResult(txtPath, wavPath)
	
	// Atualiza e envia o histórico novo
	a.webServer.BroadcastHistory(a.cleanupAndGetHistory())

	a.session = nil

	if a.capturer != nil {
		a.capturer.Close()
	}
	newCapturer, err := audio.New()
	if err == nil {
		a.capturer = newCapturer
	} else {
		a.capturer = nil
	}
}

// Stop encerra tudo
func (a *App) Stop() {
	if a.session != nil {
		a.session.Stop()
	}
	if a.capturer != nil {
		a.capturer.Close()
	}
}

func getExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func resolvePath(exeDir, subDir, filename string) string {
	candidates := []string{
		filepath.Join(subDir, filename),
		filepath.Join(".", subDir, filename),
		filepath.Join(exeDir, subDir, filename),
		filepath.Join(exeDir, filename),
		filename,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
			return c
		}
	}
	return ""
}

func resolveModel(exeDir string) string {
	modelNames := []string{
		"ggml-large-v3.bin",
		"ggml-large-v3-q5_1.bin",
		"ggml-large-v2.bin",
		"ggml-medium.bin",
		"ggml-medium-q5_1.bin",
		"ggml-small.bin",
		"ggml-small-q5_1.bin",
		"ggml-base.bin",
		"ggml-base-q5_1.bin",
		"ggml-tiny.bin",
		"ggml-tiny-q5_1.bin",
	}

	searchDirs := []string{
		modelsDir,
		filepath.Join(".", modelsDir),
		filepath.Join(exeDir, modelsDir),
	}

	for _, dir := range searchDirs {
		for _, name := range modelNames {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				abs, err := filepath.Abs(path)
				if err == nil {
					return abs
				}
				return path
			}
		}
	}
	return ""
}
