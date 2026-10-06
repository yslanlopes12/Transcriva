// Package transcription orquestra o pipeline completo:
// captura de áudio -> buffer -> Whisper -> exibição -> salvamento
package transcription

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"transcriva/internal/audio"
	"transcriva/internal/buffer"
	"transcriva/internal/output"
	"transcriva/internal/whisper"
)

const (
	// blockDurationSec: duração base de cada bloco
	blockDurationSec = 8
	// silenceThreshold: threshold de energia RMS para detectar silêncio
	silenceThreshold = 0.001
)

// Segment representa um trecho transcrito
type Segment struct {
	RelativeMs int64
	Text       string
}

// Session representa uma sessão de transcrição
type Session struct {
	mu          sync.Mutex
	capturer    *audio.Capturer
	transcriber *whisper.Transcriber
	writer      *output.Writer

	isRunning         bool
	startTime         time.Time
	segments          []Segment
	sessionID         string
	segmentChan       chan Segment
	stopChan          chan struct{}
	doneChan          chan struct{}
	fullBuffer        *buffer.AudioBuffer
	transcriptionsDir string
	recordingsDir     string
	saveWAV           bool
}

type Config struct {
	Capturer          *audio.Capturer
	Transcriber       *whisper.Transcriber
	TranscriptionsDir string
	RecordingsDir     string
	SaveWAV           bool
}

func New(cfg Config) *Session {
	return &Session{
		capturer:          cfg.Capturer,
		transcriber:       cfg.Transcriber,
		sessionID:         time.Now().Format("reuniao_2006-01-02_15-04-05"),
		segmentChan:       make(chan Segment, 50),
		stopChan:          make(chan struct{}),
		doneChan:          make(chan struct{}),
		saveWAV:           cfg.SaveWAV,
		transcriptionsDir: cfg.TranscriptionsDir,
		recordingsDir:     cfg.RecordingsDir,
	}
}

func (s *Session) SegmentChan() <-chan Segment {
	return s.segmentChan
}

func (s *Session) ElapsedTime() time.Duration {
	if s.startTime.IsZero() {
		return 0
	}
	return time.Since(s.startTime)
}

func (s *Session) Segments() []Segment {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Segment, len(s.segments))
	copy(result, s.segments)
	return result
}

func (s *Session) Start() error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("sessão já está em andamento")
	}
	s.isRunning = true
	s.startTime = time.Now()
	s.mu.Unlock()

	s.fullBuffer = buffer.New(s.capturer.SampleRate(), s.capturer.Channels())

	if err := s.capturer.Start(); err != nil {
		s.isRunning = false
		return fmt.Errorf("falha ao iniciar captura: %w", err)
	}

	go s.pipeline()

	return nil
}

type workJob struct {
	samples  []float32
	offsetMs int64
}

type transcribeResult struct {
	segments []whisper.Segment
	offsetMs int64
	err      error
}

func (s *Session) pipeline() {
	defer close(s.doneChan)

	workBuffer := buffer.New(s.capturer.SampleRate(), s.capturer.Channels())
	blockDuration := time.Duration(blockDurationSec) * time.Second

	// Canal de jobs: buffer de 1. Otimização extrema para não acumular Whisper na CPU.
	jobChan := make(chan workJob, 1)
	resultChan := make(chan transcribeResult, 10)

	// Single-Worker para o Whisper (Garante que só 1 processo use a CPU por vez)
	go func() {
		sr := s.capturer.SampleRate()
		ch := s.capturer.Channels()
		tr := s.transcriber
		for job := range jobChan {
			res := transcribeBlock(job.samples, sr, ch, tr, job.offsetMs)
			resultChan <- res
		}
	}()

	// Goroutine que processa resultados
	go func() {
		for res := range resultChan {
			if res.err != nil {
				fmt.Fprintf(os.Stderr, "\n[AVISO] Falha na transcrição: %v\n", res.err)
				continue
			}
			for _, seg := range res.segments {
				segment := Segment{
					RelativeMs: res.offsetMs + seg.StartMs,
					Text:       seg.Text,
				}
				s.mu.Lock()
				s.segments = append(s.segments, segment)
				s.mu.Unlock()

				select {
				case s.segmentChan <- segment:
				default:
				}
			}
		}
	}()

	ticker := time.NewTicker(blockDuration)
	defer ticker.Stop()

	blockOffset := int64(0)

	for {
		select {
		case <-s.stopChan:
			s.capturer.Stop()

			// Drenagem final
			drainTimeout := time.After(500 * time.Millisecond)
		drain:
			for {
				select {
				case samples, ok := <-s.capturer.DataChan():
					if !ok {
						break drain
					}
					workBuffer.Append(samples)
					s.fullBuffer.Append(samples)
				case <-drainTimeout:
					break drain
				}
			}

			// Transcreve o restinho final de áudio se for significativo
			if workBuffer.DurationMs() > 500 {
				jobChan <- workJob{
					samples:  workBuffer.Clone(),
					offsetMs: blockOffset,
				}
			}
			close(jobChan) // Finaliza o worker do Whisper

			// Espera um tempinho para o worker terminar o último bloco
			time.Sleep(300 * time.Millisecond)
			s.saveOutputFiles()
			return

		case samples, ok := <-s.capturer.DataChan():
			if !ok {
				return
			}
			workBuffer.Append(samples)
			s.fullBuffer.Append(samples)

		case <-ticker.C:
			// Menos de 1s de áudio? Ignora.
			if workBuffer.DurationMs() < 1000 {
				continue
			}

			elapsedMs := s.ElapsedTime().Milliseconds()
			currentOffset := elapsedMs - workBuffer.DurationMs()
			if currentOffset < 0 {
				currentOffset = 0
			}

			// Tenta enviar para o Worker.
			// Se ele estiver ocupado (por causa de um modelo pesado), a gente NÃO ENBIA,
			// simplesmente deixamos o áudio acumular no workBuffer. O Whisper lida de boa com até 30s.
			select {
			case jobChan <- workJob{samples: workBuffer.Clone(), offsetMs: currentOffset}:
				// Conseguiu enviar! O worker estava livre. Podemos limpar o buffer.
				workBuffer.Reset()
				blockOffset = elapsedMs
			default:
				// Worker ocupado processando modelo pesado!
				// Apenas ignora e continua acumulando áudio. Zero travamentos!
			}
		}
	}
}

func transcribeBlock(samples []float32, sampleRate uint32, channels uint16, tr *whisper.Transcriber, offsetMs int64) transcribeResult {
	if !hasAudioEnergy(samples) {
		return transcribeResult{nil, offsetMs, nil}
	}

	tmpFile, err := os.CreateTemp("", "transcriber_block_*.wav")
	if err != nil {
		return transcribeResult{nil, offsetMs, fmt.Errorf("falha temporário: %w", err)}
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	buf := buffer.New(sampleRate, channels)
	if err := buf.SaveWAV(tmpPath, samples); err != nil {
		return transcribeResult{nil, offsetMs, fmt.Errorf("falha ao salvar WAV tmp: %w", err)}
	}

	segments, err := tr.Transcribe(tmpPath)
	if err != nil {
		return transcribeResult{nil, offsetMs, err}
	}

	return transcribeResult{segments, offsetMs, nil}
}

func hasAudioEnergy(samples []float32) bool {
	if len(samples) == 0 {
		return false
	}
	var sumSq float64
	for _, s := range samples {
		sumSq += float64(s) * float64(s)
	}
	rms := sumSq / float64(len(samples))
	return rms > silenceThreshold*silenceThreshold
}

func (s *Session) Stop() {
	s.mu.Lock()
	if !s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = false
	s.mu.Unlock()

	close(s.stopChan)
}

func (s *Session) Wait() {
	<-s.doneChan
}

func (s *Session) saveOutputFiles() {
	s.mu.Lock()
	segments := make([]Segment, len(s.segments))
	copy(segments, s.segments)
	startTime := s.startTime
	s.mu.Unlock()

	os.MkdirAll(s.transcriptionsDir, 0755)
	if s.saveWAV {
		os.MkdirAll(s.recordingsDir, 0755)
	}

	txtPath := filepath.Join(s.transcriptionsDir, s.sessionID+".txt")
	w, err := output.NewFileWriter(txtPath)
	if err == nil {
		defer w.Close()
		w.WriteHeader(startTime)
		for _, seg := range segments {
			w.WriteSegment(seg.RelativeMs, seg.Text)
		}
		s.writer = w
		fmt.Printf("\n[OK] Transcrição salva em: %s\n", txtPath)
	}

	if s.saveWAV && s.fullBuffer != nil && s.fullBuffer.Len() > 0 {
		wavPath := filepath.Join(s.recordingsDir, s.sessionID+".wav")
		if err := s.fullBuffer.SaveWAVCurrentBuffer(wavPath); err == nil {
			fmt.Printf("[OK] Gravação salva em: %s\n", wavPath)
		}
	}
}

func (s *Session) OutputPaths() (txtPath, wavPath string) {
	txtPath = filepath.Join(s.transcriptionsDir, s.sessionID+".txt")
	if s.saveWAV {
		wavPath = filepath.Join(s.recordingsDir, s.sessionID+".wav")
	}
	return
}
