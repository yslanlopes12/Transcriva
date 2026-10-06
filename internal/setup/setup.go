package setup

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	whisperZipURL = "https://github.com/ggml-org/whisper.cpp/releases/download/b5454/whisper-bin-x64.zip"
	modelURL      = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin"
	defaultModel  = "ggml-medium.bin"
)

// EnsureDependencies verifica se os binários e modelos existem, e baixa automaticamente se não existirem
func EnsureDependencies() error {
	exePath, err := os.Executable()
	if err != nil {
		exePath = "."
	}
	baseDir := filepath.Dir(exePath)

	binDir := filepath.Join(baseDir, "bin")
	modelsDir := filepath.Join(baseDir, "models")

	os.MkdirAll(binDir, 0755)
	os.MkdirAll(modelsDir, 0755)

	cliPath := filepath.Join(binDir, "whisper-cli.exe")

	// Verifica se o whisper-cli.exe existe
	if _, err := os.Stat(cliPath); os.IsNotExist(err) {
		fmt.Println("\n[SETUP] Motor de transcrição (whisper.cpp) não encontrado.")
		fmt.Println("[SETUP] Baixando arquivos binários necessários...")
		if err := downloadAndExtractWhisper(binDir); err != nil {
			return fmt.Errorf("falha ao baixar motor: %w", err)
		}
	}

	// Verifica se existe QUALQUER modelo (.bin) na pasta models
	hasModel := false
	files, _ := os.ReadDir(modelsDir)
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".bin") {
			hasModel = true
			break
		}
	}

	if !hasModel {
		modelPath := filepath.Join(modelsDir, defaultModel)
		fmt.Println("\n[SETUP] Nenhum modelo de Inteligência Artificial encontrado.")
		fmt.Println("[SETUP] Baixando o modelo recomendado 'Medium' (~1.5 GB)...")
		fmt.Println("[SETUP] Isso pode demorar vários minutos, dependendo da sua internet.")
		if err := downloadFile(modelPath, modelURL); err != nil {
			return fmt.Errorf("falha ao baixar modelo: %w", err)
		}
		fmt.Println("[SETUP] Instalação concluída com sucesso!\n")
	}

	return nil
}

func downloadAndExtractWhisper(destDir string) error {
	zipPath := filepath.Join(destDir, "whisper-temp.zip")
	if err := downloadFile(zipPath, whisperZipURL); err != nil {
		return err
	}
	defer os.Remove(zipPath)

	fmt.Println("[SETUP] Extraindo arquivos...")
	return unzip(zipPath, destDir)
}

func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 32*1024)
	var downloaded int64
	total := resp.ContentLength

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			out.Write(buf[0:n])
			downloaded += int64(n)
			if total > 0 {
				percent := float64(downloaded) / float64(total) * 100
				fmt.Printf("\rBaixando... %.1f%% (%d / %d MB)", percent, downloaded/1024/1024, total/1024/1024)
			} else {
				fmt.Printf("\rBaixando... %d MB", downloaded/1024/1024)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	fmt.Println("\n[SETUP] Download concluído.")
	return nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Precisamos copiar o executável e as DLLs
		if !strings.HasSuffix(f.Name, ".exe") && !strings.HasSuffix(f.Name, ".dll") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outPath := filepath.Join(dest, filepath.Base(f.Name))
		outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
