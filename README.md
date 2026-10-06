# Transcriva (Go)

Um aplicativo 100% offline e local para Windows que transcreve automaticamente **qualquer áudio reproduzido pelo sistema** (Teams, Discord, Meet, Zoom, YouTube, etc.) e salva o texto em formato legível.

Com interface gráfica moderna baseada em navegador (Web UI embutida) e também interface de terminal (CLI), é leve, não requer C++ compiler na sua máquina para montar a interface e mantém sua privacidade garantida (nada sai do seu PC).

## 🚀 Funcionalidades

- ✅ **Privacidade Total:** 100% offline, rodando localmente no seu computador.
- ✅ **Áudio do Sistema:** Captura áudio diretamente do Windows (WASAPI Loopback). Funciona em qualquer app!
- ✅ **Interface Gráfica (Web UI):** Interface moderna que abre automaticamente no seu navegador.
- ✅ **Modo Terminal (CLI):** Opção de interface via linha de comando para devs e power-users.
- ✅ **Auto-Setup Inteligente:** Baixa dependências e a IA automaticamente na primeira execução.
- ✅ **Copiar & Baixar:** Botões dedicados para baixar `.txt` / `.wav` ou copiar no formato de ata de reunião.
- ✅ **Histórico Recente (Cache):** Guarda e gerencia as 5 últimas gravações na interface.

---

## 👤 Para Usuários Finais (O jeito mais fácil)

Você não precisa saber programar nem configurar terminais para usar!

1. **Baixe** o arquivo `transcriva.exe` (na aba *Releases* deste repositório, ou o arquivo enviado a você).
2. Coloque-o em uma pasta vazia e dê um **duplo clique** para abrir.
3. **Na primeira execução:** O programa identificará que é a sua primeira vez e começará a baixar o "cérebro" da Inteligência Artificial (~1.5 GB). Uma tela de carregamento aparecerá. *Isso só acontece uma vez!*
4. **Pronto!** Uma aba se abrirá automaticamente no seu navegador com a Interface Gráfica pronta para iniciar a gravação. Nas próximas vezes, o aplicativo abrirá instantaneamente de forma 100% offline.

---

## 💻 Para Desenvolvedores (Build e CLI)

Se você é um desenvolvedor, quer usar a interface de terminal ou deseja explorar o código para adicionar novas *features*, siga os passos abaixo:

### Pré-requisitos
- **Windows 10 ou 11** (64-bit)
- **Go 1.22+**

### 1. Compilando o Projeto
Faça o clone do repositório, abra o terminal na raiz do projeto e rode o comando de build:

```powershell
go build -o transcriva.exe -ldflags="-s -w" ./cmd/transcriva
```
*(Nota: Graças ao pacote `setup` injetado no código, você não precisa se preocupar em baixar o `whisper.cpp` manualmente. O próprio executável montará o ambiente na primeira vez que rodar).*

### 2. Executando em Modo Terminal (CLI)
Se você prefere usar o sistema diretamente na tela preta (ideal para automações ou ambientes minimalistas), rode o binário com a flag de modo:

```powershell
.\transcriva.exe -mode cli
```
Isso abrirá um painel interativo no console. Pressione `[ENTER]` para iniciar ou parar a gravação.

### Estrutura do Projeto (Para adicionar Features)
- `internal/app`: Orquestrador que liga a UI ao Backend.
- `internal/transcription`: A mágica acontece aqui (Fila de single-worker otimizada para modelos pesados).
- `internal/web/static`: HTML/Tailwind/JS da interface gráfica.

---

## 🤖 Escolhendo o Modelo Ideal de IA (Avançado)

Por padrão, o auto-setup baixa o modelo `Medium`, pois oferece excelente precisão para o idioma Português.
O modelo dita a qualidade da transcrição e os requisitos de hardware da sua máquina.

| Modelo | Consumo de RAM | Precisão PT-BR | Velocidade |
|--------|----------------|----------------|------------|
| **Tiny** | ~75 MB | Razoável | 🚀 Ultra Rápido |
| **Small** | ~460 MB | Boa | ⚡ Rápido |
| **Medium** (Padrão) | ~1.5 GB | **Excelente** | 🐢 Moderado |
| **Large-v3** | ~3 GB | Perfeita | 🐌 Pesado |

**Como trocar?** 
Basta baixar o arquivo `.bin` desejado [nesta página oficial do HuggingFace](https://huggingface.co/ggerganov/whisper.cpp/tree/main), apagar o antigo e colar o novo dentro da pasta `models/` que o executável gerou. O aplicativo usará automaticamente o novo modelo!

---

## 🤝 Contribua (Open-Source)

Este projeto é **Open-Source** e adoraríamos ver a comunidade ajudando a melhorá-lo!

Teve uma ideia bacana? Encontrou um bug? Quer adicionar uma *feature* nova (como exportação para SRT, tradução simultânea, diarização de locutores)?
Siga o fluxo abaixo:

1. Faça um **Fork** deste repositório.
2. Crie uma branch para a sua feature (`git checkout -b feature/minha-feature-nova`).
3. Faça o commit das suas alterações (`git commit -m 'Adiciona feature X'`).
4. Faça o push para a sua branch (`git push origin feature/minha-feature-nova`).
5. Abra um **Pull Request (Merge Request)** detalhando o que você construiu.

Ficaremos muito felizes em analisar seu código e realizar o merge para o projeto principal! Licenciado sob MIT.
