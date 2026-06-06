package audio

import (
	"context"
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"
	"github.com/laszukdawid/terminal-agent/internal/voice"
)

const (
	defaultCaptureSampleRate = 16000
	defaultCaptureChannels   = 1
)

type MalgoRecorder struct {
	mu      sync.Mutex
	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	buffer  []byte
	started bool
}

func NewMalgoRecorder() *MalgoRecorder {
	return &MalgoRecorder{}
}

func (r *MalgoRecorder) Start(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return fmt.Errorf("voice recorder is already recording")
	}

	malgoCtx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return err
	}
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = defaultCaptureChannels
	deviceConfig.SampleRate = defaultCaptureSampleRate
	deviceConfig.Alsa.NoMMap = 1

	callbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, _ uint32) {
			if len(input) == 0 {
				return
			}
			r.mu.Lock()
			r.buffer = append(r.buffer, input...)
			r.mu.Unlock()
		},
	}
	device, err := malgo.InitDevice(malgoCtx.Context, deviceConfig, callbacks)
	if err != nil {
		_ = malgoCtx.Uninit()
		malgoCtx.Free()
		return err
	}
	if err := device.Start(); err != nil {
		device.Uninit()
		_ = malgoCtx.Uninit()
		malgoCtx.Free()
		return err
	}

	r.ctx = malgoCtx
	r.device = device
	r.buffer = nil
	r.started = true
	return nil
}

func (r *MalgoRecorder) Stop(context.Context) (voice.Recording, error) {
	pcm, err := r.finish(false)
	if err != nil {
		return voice.Recording{}, err
	}
	return EncodePCM16LEWAV(pcm, defaultCaptureSampleRate, defaultCaptureChannels)
}

func (r *MalgoRecorder) Cancel(context.Context) error {
	_, err := r.finish(true)
	return err
}

func (r *MalgoRecorder) finish(discard bool) ([]byte, error) {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return nil, fmt.Errorf("voice recorder is not recording")
	}
	device := r.device
	malgoCtx := r.ctx
	r.mu.Unlock()

	if device != nil {
		_ = device.Stop()
		device.Uninit()
	}

	r.mu.Lock()
	pcm := append([]byte(nil), r.buffer...)
	r.device = nil
	r.ctx = nil
	r.buffer = nil
	r.started = false
	r.mu.Unlock()

	if malgoCtx != nil {
		_ = malgoCtx.Uninit()
		malgoCtx.Free()
	}
	if discard {
		return nil, nil
	}
	return pcm, nil
}
