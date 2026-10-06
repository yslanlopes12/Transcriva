// Package output gerencia a escrita da transcrição em arquivo TXT.
package output

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

// Writer escreve a transcrição em um arquivo TXT formatado
type Writer struct {
	file   *os.File
	writer *bufio.Writer
}

// NewFileWriter cria um novo Writer para o arquivo especificado
func NewFileWriter(path string) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar arquivo de transcrição em %s: %w", path, err)
	}

	return &Writer{
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

// WriteHeader escreve o cabeçalho do arquivo de transcrição
func (w *Writer) WriteHeader(startTime time.Time) error {
	_, err := fmt.Fprintf(w.writer,
		"Transcrição\n"+
			"Data: %s\n"+
			"Horário: %s\n\n",
		startTime.Format("02/01/2006"),
		startTime.Format("15:04"),
	)
	if err != nil {
		return fmt.Errorf("falha ao escrever cabeçalho: %w", err)
	}
	return w.writer.Flush()
}

// WriteSegment escreve um segmento de transcrição formatado
// relativeMs é o tempo em milissegundos relativo ao início da sessão
func (w *Writer) WriteSegment(relativeMs int64, text string) error {
	timestamp := formatRelativeTimestamp(relativeMs)
	_, err := fmt.Fprintf(w.writer, "[%s]\n%s\n\n", timestamp, text)
	if err != nil {
		return fmt.Errorf("falha ao escrever segmento: %w", err)
	}
	return w.writer.Flush()
}

// Close fecha o arquivo de transcrição
func (w *Writer) Close() error {
	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.file.Close()
}

// formatRelativeTimestamp converte milissegundos para HH:MM:SS
func formatRelativeTimestamp(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// FormatAbsoluteTimestamp formata um tempo absoluto para exibição
func FormatAbsoluteTimestamp(t time.Time) string {
	return t.Format("15:04:05")
}
