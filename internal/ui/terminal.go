// Package ui gerencia a interface de terminal do transcriva.
package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

const (
	separator = "----------------------------------------"
	header    = "========================================"
)

// TerminalUI gerencia o display e a entrada do terminal
type TerminalUI struct {
	mu           sync.Mutex
	isRecording  bool
	lines        []string
	startTime    time.Time
	stopChan     chan struct{}
}

// New cria um novo TerminalUI
func New() *TerminalUI {
	return &TerminalUI{
		lines:    make([]string, 0),
		stopChan: make(chan struct{}),
	}
}

// ShowWelcome exibe a tela inicial
func (ui *TerminalUI) ShowWelcome(deviceName, language string) {
	clearScreen()
	fmt.Println(header)
	fmt.Println("       TRANSCRIVA")
	fmt.Println(header)
	fmt.Println()
	fmt.Printf("Dispositivo de saída:\n  %s\n\n", deviceName)
	fmt.Printf("Idioma:\n  %s\n\n", formatLanguage(language))
	fmt.Println("Status:")
	fmt.Println("  PARADO")
	fmt.Println()
	fmt.Println(separator)
	fmt.Println()
	fmt.Println("  [ENTER] Iniciar transcrição")
	fmt.Println("  [Q]     Sair")
	fmt.Println("  [W]     Iniciar sem salvar WAV")
	fmt.Println()
}

// ShowRecording exibe a tela de gravação
func (ui *TerminalUI) ShowRecording() {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	clearScreen()
	elapsed := time.Since(ui.startTime)
	h := int(elapsed.Hours())
	m := int(elapsed.Minutes()) % 60
	s := int(elapsed.Seconds()) % 60

	fmt.Println(header)
	fmt.Println("       TRANSCRIVA")
	fmt.Println(header)
	fmt.Println()
	fmt.Printf("Status: 🔴 TRANSCRIBINDO\n")
	fmt.Printf("Tempo:  %02d:%02d:%02d\n", h, m, s)
	fmt.Println()
	fmt.Println(separator)
	fmt.Println("TRANSCRIÇÃO")
	fmt.Println(separator)
	fmt.Println()

	maxLines := 15
	start := 0
	if len(ui.lines) > maxLines {
		start = len(ui.lines) - maxLines
	}
	for _, line := range ui.lines[start:] {
		fmt.Println(line)
	}

	fmt.Println()
	fmt.Println(separator)
	fmt.Println()
	fmt.Println("  [ENTER] Parar e salvar")
	fmt.Println()
}

// StartTimer inicia o timer
func (ui *TerminalUI) StartTimer() {
	ui.startTime = time.Now()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ui.stopChan:
				return
			case <-ticker.C:
				if ui.isRecording {
					ui.ShowRecording()
				}
			}
		}
	}()
}

// StopTimer para o timer
func (ui *TerminalUI) StopTimer() {
	select {
	case <-ui.stopChan:
	default:
		close(ui.stopChan)
	}
}

// AddSegment adiciona uma transcrição à tela
func (ui *TerminalUI) AddSegment(relativeMs int64, text string) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	timestamp := formatRelativeTs(relativeMs)
	ui.lines = append(ui.lines, fmt.Sprintf("[%s]", timestamp))

	words := strings.Fields(text)
	var currentLine strings.Builder
	for _, word := range words {
		if currentLine.Len()+len(word)+1 > 60 && currentLine.Len() > 0 {
			ui.lines = append(ui.lines, currentLine.String())
			currentLine.Reset()
		}
		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}
	if currentLine.Len() > 0 {
		ui.lines = append(ui.lines, currentLine.String())
	}
	ui.lines = append(ui.lines, "")
}

// SetRecording muda estado
func (ui *TerminalUI) SetRecording(v bool) {
	ui.mu.Lock()
	ui.isRecording = v
	ui.mu.Unlock()
}

// ShowStopping mostra tela de encerramento
func (ui *TerminalUI) ShowStopping() {
	fmt.Println()
	fmt.Println(separator)
	fmt.Println("  Finalizando transcrição...")
	fmt.Println("  Aguarde o processamento do último bloco de áudio.")
	fmt.Println(separator)
}

// ShowFinalSummary exibe o final
func (ui *TerminalUI) ShowFinalSummary(txtPath, wavPath string, segmentCount int, elapsed time.Duration) {
	fmt.Println()
	fmt.Println(header)
	fmt.Println("       TRANSCRIÇÃO CONCLUÍDA")
	fmt.Println(header)
	fmt.Println()
	fmt.Printf("  Duração total:    %s\n", formatDuration(elapsed))
	fmt.Printf("  Segmentos:        %d\n", segmentCount)
	fmt.Println()
	if txtPath != "" {
		fmt.Printf("  📄 Transcrição:   %s\n", txtPath)
	}
	if wavPath != "" {
		fmt.Printf("  🔊 Gravação WAV:  %s\n", wavPath)
	}
	fmt.Println()
	fmt.Println(header)
	fmt.Println()
}

// ReadKey lê uma tecla
func ReadKey() (byte, error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		var b [1]byte
		_, err := os.Stdin.Read(b[:])
		return b[0], err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	var b [1]byte
	_, err = os.Stdin.Read(b[:])
	return b[0], err
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func formatLanguage(lang string) string {
	if lang == "pt" {
		return "Português (Brasil)"
	}
	return lang
}

func formatRelativeTs(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}
