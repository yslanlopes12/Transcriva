@echo off
:: Script de build completo do projeto
setlocal

echo.
echo ================================================
echo   Build: Transcriva
echo ================================================
echo.

:: Verifica se o Go está instalado
where go >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo [ERRO] Go não encontrado. Instale em: https://golang.org/dl/
    exit /b 1
)

:: Informações da versão Go
for /f "tokens=3" %%v in ('go version') do set GO_VERSION=%%v
echo Go: %GO_VERSION%

:: Download das dependências
echo.
echo Baixando dependências...
go mod download
if %ERRORLEVEL% neq 0 (
    echo [ERRO] Falha ao baixar dependências
    exit /b 1
)

:: Build
echo.
echo Compilando...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64

go build -o transcriva.exe -ldflags="-s -w" ./cmd/transcriva
if %ERRORLEVEL% neq 0 (
    echo [ERRO] Falha na compilação
    exit /b 1
)

echo.
echo [OK] Build concluído: transcriva.exe
echo.
echo Para executar:
echo   transcriva.exe
echo.
echo Certifique-se de ter:
echo   bin\whisper-cli.exe
echo   models\ggml-small.bin (ou outro modelo)
echo.
pause
