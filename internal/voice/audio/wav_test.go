package audio

import (
	"encoding/binary"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/voice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodePCM16LEWAV(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	rec, err := EncodePCM16LEWAV(pcm, 16000, 1)
	require.NoError(t, err)

	assert.Equal(t, voice.AudioFormatWAV, rec.Format)
	assert.Equal(t, "audio/wav", rec.MIMEType)
	assert.Equal(t, 16000, rec.SampleRate)
	assert.Equal(t, 1, rec.Channels)
	require.Len(t, rec.Data, 48)
	assert.Equal(t, "RIFF", string(rec.Data[0:4]))
	assert.Equal(t, uint32(40), binary.LittleEndian.Uint32(rec.Data[4:8]))
	assert.Equal(t, "WAVE", string(rec.Data[8:12]))
	assert.Equal(t, "fmt ", string(rec.Data[12:16]))
	assert.Equal(t, uint16(1), binary.LittleEndian.Uint16(rec.Data[20:22]))
	assert.Equal(t, uint16(1), binary.LittleEndian.Uint16(rec.Data[22:24]))
	assert.Equal(t, uint32(16000), binary.LittleEndian.Uint32(rec.Data[24:28]))
	assert.Equal(t, uint32(32000), binary.LittleEndian.Uint32(rec.Data[28:32]))
	assert.Equal(t, uint16(2), binary.LittleEndian.Uint16(rec.Data[32:34]))
	assert.Equal(t, uint16(16), binary.LittleEndian.Uint16(rec.Data[34:36]))
	assert.Equal(t, "data", string(rec.Data[36:40]))
	assert.Equal(t, uint32(len(pcm)), binary.LittleEndian.Uint32(rec.Data[40:44]))
	assert.Equal(t, pcm, rec.Data[44:])
}

func TestEncodePCM16LEWAVRejectsInvalidFormat(t *testing.T) {
	tests := []struct {
		name       string
		pcm        []byte
		sampleRate int
		channels   int
	}{
		{name: "odd pcm length", pcm: []byte{0x01}, sampleRate: 16000, channels: 1},
		{name: "zero sample rate", pcm: []byte{0x01, 0x02}, channels: 1},
		{name: "zero channels", pcm: []byte{0x01, 0x02}, sampleRate: 16000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EncodePCM16LEWAV(tt.pcm, tt.sampleRate, tt.channels)
			require.Error(t, err)
		})
	}
}
