package audio

import (
	"encoding/binary"
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/voice"
)

const (
	wavHeaderSize    = 44
	wavPCMFormat     = 1
	wavBitsPerSample = 16
)

func EncodePCM16LEWAV(pcm []byte, sampleRate int, channels int) (voice.Recording, error) {
	if len(pcm)%2 != 0 {
		return voice.Recording{}, fmt.Errorf("pcm_s16le data length must be even")
	}
	if sampleRate <= 0 {
		return voice.Recording{}, fmt.Errorf("sample rate must be positive")
	}
	if channels <= 0 {
		return voice.Recording{}, fmt.Errorf("channels must be positive")
	}

	data := make([]byte, wavHeaderSize+len(pcm))
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(36+len(pcm)))
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], wavPCMFormat)
	binary.LittleEndian.PutUint16(data[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(data[24:28], uint32(sampleRate))
	byteRate := sampleRate * channels * wavBitsPerSample / 8
	binary.LittleEndian.PutUint32(data[28:32], uint32(byteRate))
	blockAlign := channels * wavBitsPerSample / 8
	binary.LittleEndian.PutUint16(data[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(data[34:36], wavBitsPerSample)
	copy(data[36:40], "data")
	binary.LittleEndian.PutUint32(data[40:44], uint32(len(pcm)))
	copy(data[44:], pcm)

	return voice.Recording{
		Data:       data,
		Format:     voice.AudioFormatWAV,
		MIMEType:   "audio/wav",
		SampleRate: sampleRate,
		Channels:   channels,
	}, nil
}
