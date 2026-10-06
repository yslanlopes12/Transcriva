// Package buffer gerencia o acúmulo de amostras de áudio float32 e sua
// conversão para WAV 16kHz mono, adequado para o Whisper.
package buffer

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// AudioBuffer acumula amostras de áudio e permite exportar para WAV
type AudioBuffer struct {
	samples    []float32
	sampleRate uint32
	channels   uint16
}

// New cria um novo AudioBuffer
func New(sampleRate uint32, channels uint16) *AudioBuffer {
	return &AudioBuffer{
		samples:    make([]float32, 0, int(sampleRate)*30), // pré-aloca 30s
		sampleRate: sampleRate,
		channels:   channels,
	}
}

// Append adiciona amostras ao buffer
func (b *AudioBuffer) Append(samples []float32) {
	b.samples = append(b.samples, samples...)
}

// DurationMs retorna a duração atual do buffer em milissegundos
func (b *AudioBuffer) DurationMs() int64 {
	if b.sampleRate == 0 || b.channels == 0 {
		return 0
	}
	totalFrames := len(b.samples) / int(b.channels)
	return int64(totalFrames) * 1000 / int64(b.sampleRate)
}

// Len retorna o número de amostras no buffer
func (b *AudioBuffer) Len() int {
	return len(b.samples)
}

// Reset limpa o buffer (reutiliza memória)
func (b *AudioBuffer) Reset() {
	b.samples = b.samples[:0]
}

// Clone retorna uma cópia das amostras atuais
func (b *AudioBuffer) Clone() []float32 {
	result := make([]float32, len(b.samples))
	copy(result, b.samples)
	return result
}

// SaveWAV salva o conteúdo atual como arquivo WAV PCM 16-bit mono 16kHz
// O Whisper requer 16kHz mono, então fazemos o downsample/downmix aqui.
func (b *AudioBuffer) SaveWAV(path string, originalSamples []float32) error {
	if len(originalSamples) == 0 {
		return fmt.Errorf("sem amostras para salvar")
	}

	// Converte para mono se necessário (downmix de canais)
	monoSamples := downmixToMono(originalSamples, b.channels)

	// Faz resample para 16kHz se necessário
	targetRate := uint32(16000)
	if b.sampleRate != targetRate {
		monoSamples = resample(monoSamples, b.sampleRate, targetRate)
	}

	// Salva como WAV PCM 16-bit
	return writeWAV(path, monoSamples, targetRate)
}

// SaveWAVCurrentBuffer salva o buffer atual como WAV
func (b *AudioBuffer) SaveWAVCurrentBuffer(path string) error {
	return b.SaveWAV(path, b.Clone())
}

// downmixToMono converte amostras multi-canal para mono fazendo média dos canais
func downmixToMono(samples []float32, channels uint16) []float32 {
	if channels == 1 {
		return samples
	}

	ch := int(channels)
	frames := len(samples) / ch
	mono := make([]float32, frames)

	for i := 0; i < frames; i++ {
		var sum float32
		for c := 0; c < ch; c++ {
			idx := i*ch + c
			if idx < len(samples) {
				sum += samples[idx]
			}
		}
		mono[i] = sum / float32(ch)
	}

	return mono
}

// resample faz interpolação linear simples para mudar a taxa de amostragem
func resample(samples []float32, fromRate, toRate uint32) []float32 {
	if fromRate == toRate {
		return samples
	}

	ratio := float64(fromRate) / float64(toRate)
	outputLen := int(float64(len(samples)) / ratio)
	output := make([]float32, outputLen)

	for i := 0; i < outputLen; i++ {
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx+1 < len(samples) {
			// Interpolação linear
			output[i] = float32(float64(samples[srcIdx])*(1-frac) + float64(samples[srcIdx+1])*frac)
		} else if srcIdx < len(samples) {
			output[i] = samples[srcIdx]
		}
	}

	return output
}

// writeWAV escreve um arquivo WAV PCM 16-bit
func writeWAV(path string, samples []float32, sampleRate uint32) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("falha ao criar arquivo WAV: %w", err)
	}
	defer f.Close()

	channels := uint16(1)
	bitsPerSample := uint16(16)
	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample) / 8
	blockAlign := channels * bitsPerSample / 8
	dataSize := uint32(len(samples)) * uint32(bitsPerSample) / 8
	chunkSize := 36 + dataSize

	// RIFF header
	f.WriteString("RIFF")
	binary.Write(f, binary.LittleEndian, chunkSize)
	f.WriteString("WAVE")

	// fmt chunk
	f.WriteString("fmt ")
	binary.Write(f, binary.LittleEndian, uint32(16))       // chunk size
	binary.Write(f, binary.LittleEndian, uint16(1))        // PCM
	binary.Write(f, binary.LittleEndian, channels)
	binary.Write(f, binary.LittleEndian, sampleRate)
	binary.Write(f, binary.LittleEndian, byteRate)
	binary.Write(f, binary.LittleEndian, blockAlign)
	binary.Write(f, binary.LittleEndian, bitsPerSample)

	// data chunk
	f.WriteString("data")
	binary.Write(f, binary.LittleEndian, dataSize)

	// Amostras PCM 16-bit
	for _, s := range samples {
		// Clamp para evitar overflow
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		pcm := int16(math.Round(float64(s) * 32767.0))
		binary.Write(f, binary.LittleEndian, pcm)
	}

	return nil
}
