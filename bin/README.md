# Binários do Whisper

Este diretório contém o executável `whisper-cli.exe` do projeto whisper.cpp.

## Download automático

```bat
scripts\download_whisper.bat
```

## Download manual

1. Acesse: https://github.com/ggml-org/whisper.cpp/releases
2. Baixe o arquivo: `whisper-bin-x64-Release.zip`
3. Extraia e copie `whisper-cli.exe` para esta pasta

## DLLs necessárias

O build padrão do whisper.cpp para Windows é **standalone** (sem DLLs externas).

Se você baixar uma versão com suporte a GPU (CUDA/Vulkan), pode ser necessário incluir as DLLs correspondentes no mesmo diretório do `system-transcriber.exe`.

## Versões

| Variante | Velocidade | Requisito |
|---------|-----------|-----------|
| whisper-cli.exe (CPU) | Moderada | Apenas CPU |
| whisper-cli.exe (CUDA) | Rápida | NVIDIA GPU + CUDA |
| whisper-cli.exe (Vulkan) | Rápida | AMD/NVIDIA GPU |

Para uso geral, a versão CPU é suficiente.
