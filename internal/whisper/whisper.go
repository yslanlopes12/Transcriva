// Package whisper fornece integração com o whisper-cli (whisper.cpp) via os/exec.
// Abordagem escolhida: subprocess (os/exec) em vez de CGO para máxima
// compatibilidade e simplicidade de build/distribuição no Windows.
package whisper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Config contém as configurações do transcritor Whisper
type Config struct {
	// BinPath: caminho para o whisper-cli.exe
	BinPath string
	// ModelPath: caminho para o arquivo .bin do modelo
	ModelPath string
	// Language: código do idioma (ex: "pt", "en", "es")
	Language string
	// Threads: número de threads CPU (0 = automático)
	Threads int
}

// Segment representa um trecho de texto transcrito com timestamp
type Segment struct {
	StartMs int64
	EndMs   int64
	Text    string
}

// Transcriber gerencia a transcrição de áudio via whisper-cli
type Transcriber struct {
	config Config
}

// whisperOutput representa a saída JSON do whisper-cli
type whisperOutput struct {
	Transcription []struct {
		Timestamps struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"timestamps"`
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

// New cria um novo Transcriber com validação de pré-requisitos
func New(cfg Config) (*Transcriber, error) {
	// Define número de threads automaticamente se não especificado
	if cfg.Threads <= 0 {
		cfg.Threads = runtime.NumCPU()
		if cfg.Threads > 8 {
			cfg.Threads = 8 // limita para não sobrecarregar
		}
	}

	// Valida o executável whisper-cli
	if cfg.BinPath == "" {
		cfg.BinPath = findWhisperBin()
	}

	if cfg.BinPath == "" {
		return nil, fmt.Errorf("whisper-cli.exe não encontrado\n" +
			"  Baixe em: https://github.com/ggml-org/whisper.cpp/releases\n" +
			"  Coloque em: ./bin/whisper-cli.exe")
	}

	if _, err := os.Stat(cfg.BinPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("whisper-cli.exe não encontrado em: %s\n"+
			"  Baixe em: https://github.com/ggml-org/whisper.cpp/releases", cfg.BinPath)
	}

	// Valida o modelo
	if cfg.ModelPath == "" {
		cfg.ModelPath = findModel()
	}

	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("modelo Whisper não encontrado\n" +
			"  Execute: scripts\\download_model.bat small\n" +
			"  Ou baixe manualmente em: https://huggingface.co/ggerganov/whisper.cpp/tree/main")
	}

	if _, err := os.Stat(cfg.ModelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("modelo não encontrado em: %s\n"+
			"  Execute: scripts\\download_model.bat small", cfg.ModelPath)
	}

	// Define idioma padrão
	if cfg.Language == "" {
		cfg.Language = "pt"
	}

	return &Transcriber{config: cfg}, nil
}

// findWhisperBin procura o executável whisper-cli em locais comuns
func findWhisperBin() string {
	candidates := []string{
		"bin/whisper-cli.exe",
		"./bin/whisper-cli.exe",
		"whisper-cli.exe",
		"./whisper-cli.exe",
	}

	// Verifica também o diretório do executável
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "bin", "whisper-cli.exe"),
			filepath.Join(dir, "whisper-cli.exe"),
		)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			abs, err := filepath.Abs(candidate)
			if err == nil {
				return abs
			}
			return candidate
		}
	}
	return ""
}

// findModel procura um arquivo de modelo .bin em locais comuns
func findModel() string {
	// Prioridade: small > base > tiny
	modelNames := []string{
		"ggml-small.bin",
		"ggml-small-q5_1.bin",
		"ggml-base.bin",
		"ggml-base-q5_1.bin",
		"ggml-tiny.bin",
		"ggml-tiny-q5_1.bin",
	}

	searchDirs := []string{"models", "./models"}

	if exe, err := os.Executable(); err == nil {
		searchDirs = append(searchDirs, filepath.Join(filepath.Dir(exe), "models"))
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

// Transcribe recebe um arquivo WAV e retorna os segmentos transcritos
// O arquivo WAV deve estar em 16kHz mono PCM para melhores resultados
func (t *Transcriber) Transcribe(wavPath string) ([]Segment, error) {
	if _, err := os.Stat(wavPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("arquivo WAV não encontrado: %s", wavPath)
	}

	// Cria arquivo de saída JSON temporário
	jsonPath := wavPath + ".json"
	defer os.Remove(jsonPath)

	// Monta os argumentos do whisper-cli
	args := []string{
		"--model", t.config.ModelPath,
		"--file", wavPath,
		"--language", t.config.Language,
		"--output-json",
		"--output-file", strings.TrimSuffix(wavPath, ".wav"),
		"--no-prints",           // suprime prints de progresso
		"--no-timestamps",       // não usar — queremos os timestamps
		"--threads", strconv.Itoa(t.config.Threads),
		"--beam-size", "5",      // melhora qualidade
		"--best-of", "5",
		"--word-thold", "0.01",  // threshold de confiança de palavras
	}

	// Remove --no-timestamps dos args (queremos os timestamps de segmento)
	args = removeArg(args, "--no-timestamps")

	cmd := exec.Command(t.config.BinPath, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("whisper-cli falhou: %w\n  Stderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("whisper-cli falhou: %w", err)
	}

	// Lê o arquivo JSON gerado
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		// Tenta o caminho alternativo (whisper-cli pode gerar com sufixo diferente)
		altPath := strings.TrimSuffix(wavPath, ".wav") + ".json"
		jsonData, err = os.ReadFile(altPath)
		if err != nil {
			return nil, fmt.Errorf("falha ao ler saída JSON do whisper: %w", err)
		}
		defer os.Remove(altPath)
	}

	// Parse do JSON
	var output whisperOutput
	if err := json.Unmarshal(jsonData, &output); err != nil {
		return nil, fmt.Errorf("falha ao parsear saída JSON: %w\n  JSON: %s", err, string(jsonData))
	}

	// Converte para Segments
	segments := make([]Segment, 0, len(output.Transcription))
	for _, item := range output.Transcription {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		// Filtra outputs de silêncio comuns do whisper
		if isNoiseText(text) {
			continue
		}
		segments = append(segments, Segment{
			StartMs: item.Offsets.From,
			EndMs:   item.Offsets.To,
			Text:    text,
		})
	}

	return segments, nil
}

// isNoiseText filtra textos que são artefatos comuns do Whisper em silêncio
func isNoiseText(text string) bool {
	noisePatterns := []string{
		"[BLANK_AUDIO]",
		"[ Silêncio ]",
		"[silêncio]",
		"[ Música ]",
		"[música]",
		"(silêncio)",
		"(música)",
	}
	lower := strings.ToLower(text)
	for _, p := range noisePatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// removeArg remove um argumento da lista (helper)
func removeArg(args []string, arg string) []string {
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == arg {
			continue
		}
		result = append(result, args[i])
	}
	return result
}

// FormatTimestamp formata milissegundos em formato HH:MM:SS
func FormatTimestamp(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// ModelInfo retorna informações sobre o modelo em uso
func (t *Transcriber) ModelInfo() string {
	return filepath.Base(t.config.ModelPath)
}

// Language retorna o idioma configurado
func (t *Transcriber) Language() string {
	return t.config.Language
}
