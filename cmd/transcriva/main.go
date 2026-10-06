package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"transcriva/internal/app"
	"transcriva/internal/setup"
)

func main() {
	mode := flag.String("mode", "gui", "Modo de execução: 'gui' (Interface Web/Navegador) ou 'cli' (Terminal)")
	flag.Parse()

	// Verifica e baixa dependências (modelo, whisper-cli) se não existirem
	if err := setup.EnsureDependencies(); err != nil {
		fmt.Fprintf(os.Stderr, "\n[ERRO CRÍTICO] Falha ao configurar dependências:\n  %v\n", err)
		os.Exit(1)
	}

	// Captura sinais do SO para encerramento limpo
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[ERRO] Falha ao inicializar o programa:\n  %v\n\n", err)
		fmt.Fprintf(os.Stderr, "Verifique se o whisper-cli.exe está em ./bin/ e o modelo em ./models/\n")
		os.Exit(1)
	}

	// Goroutine para encerramento por sinal do SO
	go func() {
		<-sigChan
		fmt.Println("\n\n[SINAL] Encerrando...")
		application.Stop()
		os.Exit(0)
	}()

	if *mode == "cli" {
		if err := application.RunCLI(); err != nil {
			fmt.Fprintf(os.Stderr, "\n[ERRO CLI] %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := application.RunGUI(); err != nil {
			fmt.Fprintf(os.Stderr, "\n[ERRO GUI] %v\n", err)
			os.Exit(1)
		}
	}
}
