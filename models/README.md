# Modelos Whisper

Este diretório contém os modelos GGML usados pelo whisper.cpp.

## Modelo Recomendado

**`ggml-small.bin`** — melhor equilíbrio para português brasileiro:

| Modelo | RAM | Velocidade | Precisão PT |
|--------|-----|-----------|-------------|
| tiny   | ~75MB  | Muito rápido | Razoável |
| base   | ~140MB | Rápido       | Boa      |
| **small**  | **~460MB** | **Moderado** | **Ótima ✓** |
| medium | ~1.5GB | Lento        | Excelente |

## Como baixar

### Opção 1: Script automático (recomendado)

```bat
scripts\download_model.bat small
```

### Opção 2: Manual (PowerShell)

```powershell
# Modelo small (recomendado para PT-BR)
Invoke-WebRequest `
  -Uri "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin" `
  -OutFile "models\ggml-small.bin"
```

### Opção 3: Download direto

Acesse: https://huggingface.co/ggerganov/whisper.cpp/tree/main

Baixe o arquivo desejado e coloque aqui em `models/`.

## Modelos quantizados (menor tamanho, boa qualidade)

Para computadores com pouca RAM, use versões quantizadas:

```powershell
# small quantizado Q5 (~190MB)
Invoke-WebRequest `
  -Uri "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small-q5_1.bin" `
  -OutFile "models\ggml-small-q5_1.bin"
```

## Notas

- O programa detecta automaticamente o modelo presente neste diretório
- Prioridade: `small` > `base` > `tiny`
- Para trocar o modelo, basta baixar outro arquivo .bin aqui
