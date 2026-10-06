package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"transcriva/internal/app"
	"transcriva/internal/setup"
)

func pauseAndExit(code int) {
	fmt.Println("\nPressione [ENTER] para fechar esta janela...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	os.Exit(code)
}

func main() {
	mode := flag.String("mode", "gui", "Modo de execução: 'gui' (Interface Web/Navegador) ou 'cli' (Terminal)")
	flag.Parse()

	// Verifica e baixa dependências (modelo, whisper-cli) se não existirem
	if err := setup.EnsureDependencies(); err != nil {
		fmt.Fprintf(os.Stderr, "\n[ERRO CRÍTICO] Falha ao configurar dependências:\n  %v\n", err)
		pauseAndExit(1)
	}

	// Captura sinais do SO para encerramento limpo
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[ERRO] Falha ao inicializar o programa:\n  %v\n\n", err)
		fmt.Fprintf(os.Stderr, "Detalhes do erro podem ser permissões de áudio ou falta do dispositivo de gravação.\n")
		pauseAndExit(1)
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
			pauseAndExit(1)
		}
	} else {
		if err := application.RunGUI(); err != nil {
			fmt.Fprintf(os.Stderr, "\n[ERRO GUI] %v\n", err)
			pauseAndExit(1)
		}
	}
}
