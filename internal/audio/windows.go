// Package audio fornece captura de áudio do sistema via WASAPI Loopback no Windows.
// Utiliza a biblioteca go-wca para acessar as interfaces COM do Windows Core Audio API.
package audio

import (
	"fmt"
	"math"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

const (
	// AUDCLNT_STREAMFLAGS_LOOPBACK captura o áudio que está sendo reproduzido
	AUDCLNT_STREAMFLAGS_LOOPBACK = uint32(0x00020000)
	// REFTIMES_PER_MILLISEC: 100ns intervals por millisegundo
	REFTIMES_PER_MILLISEC = wca.REFERENCE_TIME(10000)
	// Tamanho do buffer de captura: 100ms
	bufferDurationMs = 100
)

// DeviceInfo contém informações sobre o dispositivo de saída de áudio
type DeviceInfo struct {
	Name          string
	SampleRate    uint32
	Channels      uint16
	BitsPerSample uint16
}

// Capturer gerencia a captura de áudio via WASAPI Loopback
type Capturer struct {
	device        *wca.IMMDevice
	audioClient   *wca.IAudioClient
	captureClient *wca.IAudioCaptureClient
	waveFormat    *wca.WAVEFORMATEX
	deviceInfo    DeviceInfo
	isCapturing   bool
	stopChan      chan struct{}
	dataChan      chan []float32
}

// New cria um novo Capturer e inicializa o dispositivo de saída padrão
func New() (*Capturer, error) {
	// Inicializa COM (necessário para WASAPI)
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// HRESULT S_FALSE (1) significa que COM já foi inicializado — não é erro
		if oleErr, ok := err.(*ole.OleError); ok {
			if oleErr.Code() != 1 {
				return nil, fmt.Errorf("falha ao inicializar COM: %w", err)
			}
		}
	}

	// Obtém o enumerador de dispositivos de mídia
	var mmDeviceEnumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator,
		0,
		wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator,
		&mmDeviceEnumerator,
	); err != nil {
		return nil, fmt.Errorf("falha ao criar MMDeviceEnumerator: %w", err)
	}
	defer mmDeviceEnumerator.Release()

	// Obtém o dispositivo de saída (renderização) padrão
	var device *wca.IMMDevice
	if err := mmDeviceEnumerator.GetDefaultAudioEndpoint(
		wca.ERender,  // dispositivo de renderização (saída)
		wca.EConsole, // uso: console/multimídia
		&device,
	); err != nil {
		return nil, fmt.Errorf("nenhum dispositivo de saída de áudio encontrado: %w\n  Verifique se há alto-falantes ou fones conectados e habilitados", err)
	}

	// Obtém as propriedades do dispositivo para exibir o nome
	deviceName := getDeviceName(device)

	// Obtém o IAudioClient via ativação
	// A assinatura correta é: Activate(refIID, ctx, param, obj) error
	var audioClient *wca.IAudioClient
	if err := device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &audioClient); err != nil {
		device.Release()
		return nil, fmt.Errorf("falha ao ativar IAudioClient: %w", err)
	}

	// Obtém o formato de mix do dispositivo (formato nativo do Windows)
	var waveFormat *wca.WAVEFORMATEX
	if err := audioClient.GetMixFormat(&waveFormat); err != nil {
		audioClient.Release()
		device.Release()
		return nil, fmt.Errorf("falha ao obter formato de áudio: %w", err)
	}

	deviceInfo := DeviceInfo{
		Name:          deviceName,
		SampleRate:    waveFormat.NSamplesPerSec,
		Channels:      waveFormat.NChannels,
		BitsPerSample: waveFormat.WBitsPerSample,
	}

	return &Capturer{
		device:      device,
		audioClient: audioClient,
		waveFormat:  waveFormat,
		deviceInfo:  deviceInfo,
		stopChan:    make(chan struct{}),
		dataChan:    make(chan []float32, 100), // buffer generoso para evitar perda
	}, nil
}

// getDeviceName tenta obter o nome amigável do dispositivo
func getDeviceName(device *wca.IMMDevice) string {
	var propertyStore *wca.IPropertyStore
	if err := device.OpenPropertyStore(wca.STGM_READ, &propertyStore); err != nil {
		return "Dispositivo de Áudio Padrão"
	}
	defer propertyStore.Release()

	var propVariant wca.PROPVARIANT
	if err := propertyStore.GetValue(&wca.PKEY_Device_FriendlyName, &propVariant); err != nil {
		return "Dispositivo de Áudio Padrão"
	}

	name := propVariant.String()
	if name == "" {
		return "Dispositivo de Áudio Padrão"
	}
	return name
}

// DeviceInfo retorna informações sobre o dispositivo capturado
func (c *Capturer) DeviceInfo() DeviceInfo {
	return c.deviceInfo
}

// SampleRate retorna a taxa de amostragem do dispositivo
func (c *Capturer) SampleRate() uint32 {
	return c.waveFormat.NSamplesPerSec
}

// Channels retorna o número de canais do dispositivo
func (c *Capturer) Channels() uint16 {
	return c.waveFormat.NChannels
}

// DataChan retorna o canal de dados de áudio (samples float32 normalizados)
func (c *Capturer) DataChan() <-chan []float32 {
	return c.dataChan
}

// Start inicia a captura de áudio em modo loopback
func (c *Capturer) Start() error {
	if c.isCapturing {
		return fmt.Errorf("captura já está em andamento")
	}

	// Inicializa o IAudioClient em modo compartilhado com flag LOOPBACK
	bufferDuration := REFTIMES_PER_MILLISEC * bufferDurationMs
	if err := c.audioClient.Initialize(
		wca.AUDCLNT_SHAREMODE_SHARED,
		AUDCLNT_STREAMFLAGS_LOOPBACK,
		bufferDuration,
		0,
		c.waveFormat,
		nil,
	); err != nil {
		return fmt.Errorf("falha ao inicializar audio client em modo loopback: %w\n  Verifique se WASAPI está disponível no seu Windows", err)
	}

	// Obtém o IAudioCaptureClient
	// A assinatura correta é: GetService(refIID, obj) error
	var captureClient *wca.IAudioCaptureClient
	if err := c.audioClient.GetService(wca.IID_IAudioCaptureClient, &captureClient); err != nil {
		return fmt.Errorf("falha ao obter IAudioCaptureClient: %w", err)
	}
	c.captureClient = captureClient

	// Inicia o stream de áudio
	if err := c.audioClient.Start(); err != nil {
		return fmt.Errorf("falha ao iniciar stream de áudio: %w", err)
	}

	c.isCapturing = true
	c.stopChan = make(chan struct{})

	// Goroutine de captura em loop
	go c.captureLoop()

	return nil
}

// captureLoop lê continuamente os buffers de áudio do WASAPI
func (c *Capturer) captureLoop() {
	// Intervalo de polling: metade do buffer para garantir captura sem lacunas
	pollInterval := time.Duration(bufferDurationMs/2) * time.Millisecond

	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		// Obtém o número de frames disponíveis
		var packetLength uint32
		if err := c.captureClient.GetNextPacketSize(&packetLength); err != nil {
			// Erro temporário, tenta novamente
			time.Sleep(pollInterval)
			continue
		}

		// Processa todos os pacotes disponíveis
		for packetLength > 0 {
			var data *byte
			var numFramesToRead uint32
			var flags uint32

			if err := c.captureClient.GetBuffer(
				&data,
				&numFramesToRead,
				&flags,
				nil,
				nil,
			); err != nil {
				break
			}

			if numFramesToRead == 0 {
				c.captureClient.ReleaseBuffer(0)
				break
			}

			// Converte os bytes raw para float32 normalizado
			samples := convertToFloat32(data, numFramesToRead, c.waveFormat)

			// Envia para o canal (não bloqueante para não perder frames)
			select {
			case c.dataChan <- samples:
			default:
				// Canal cheio: descarta o frame mais antigo e insere o novo
				select {
				case <-c.dataChan:
				default:
				}
				select {
				case c.dataChan <- samples:
				default:
				}
			}

			if err := c.captureClient.ReleaseBuffer(numFramesToRead); err != nil {
				break
			}

			if err := c.captureClient.GetNextPacketSize(&packetLength); err != nil {
				packetLength = 0
			}
		}

		time.Sleep(pollInterval)
	}
}

// convertToFloat32 converte os bytes raw do WASAPI para amostras float32 normalizadas [-1.0, 1.0]
// O WASAPI em modo compartilhado geralmente usa float32 (IEEE 754), mas pode usar int16 ou int24.
func convertToFloat32(data *byte, numFrames uint32, wf *wca.WAVEFORMATEX) []float32 {
	if data == nil || numFrames == 0 {
		return nil
	}

	channels := int(wf.NChannels)
	totalSamples := int(numFrames) * channels
	bytesPerSample := int(wf.WBitsPerSample) / 8
	totalBytes := totalSamples * bytesPerSample

	// Converte o ponteiro para slice de bytes
	rawBytes := unsafe.Slice(data, totalBytes)

	result := make([]float32, totalSamples)

	switch wf.WBitsPerSample {
	case 32:
		// Float32 (formato mais comum no Windows 10/11 WASAPI compartilhado)
		for i := 0; i < totalSamples; i++ {
			offset := i * 4
			if offset+4 > len(rawBytes) {
				break
			}
			bits := uint32(rawBytes[offset]) |
				uint32(rawBytes[offset+1])<<8 |
				uint32(rawBytes[offset+2])<<16 |
				uint32(rawBytes[offset+3])<<24
			result[i] = math.Float32frombits(bits)
		}
	case 16:
		// Int16 normalizado para float32
		for i := 0; i < totalSamples; i++ {
			offset := i * 2
			if offset+2 > len(rawBytes) {
				break
			}
			sample := int16(uint16(rawBytes[offset]) | uint16(rawBytes[offset+1])<<8)
			result[i] = float32(sample) / 32768.0
		}
	case 24:
		// Int24 normalizado para float32
		for i := 0; i < totalSamples; i++ {
			offset := i * 3
			if offset+3 > len(rawBytes) {
				break
			}
			sample := int32(rawBytes[offset]) |
				int32(rawBytes[offset+1])<<8 |
				int32(rawBytes[offset+2])<<16
			// Extensão de sinal para 32 bits
			if sample&0x800000 != 0 {
				sample |= ^int32(0xFFFFFF)
			}
			result[i] = float32(sample) / 8388608.0
		}
	}

	return result
}

// Stop para a captura de áudio
func (c *Capturer) Stop() {
	if !c.isCapturing {
		return
	}
	close(c.stopChan)
	c.isCapturing = false

	if c.audioClient != nil {
		c.audioClient.Stop()
	}
}

// Close libera todos os recursos COM
func (c *Capturer) Close() {
	c.Stop()

	if c.captureClient != nil {
		c.captureClient.Release()
		c.captureClient = nil
	}
	if c.audioClient != nil {
		c.audioClient.Release()
		c.audioClient = nil
	}
	if c.device != nil {
		c.device.Release()
		c.device = nil
	}

	ole.CoUninitialize()
}
