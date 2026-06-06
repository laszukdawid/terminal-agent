package voice

type AudioFormat string

const (
	AudioFormatPCM16LE AudioFormat = "pcm_s16le"
	AudioFormatWAV     AudioFormat = "wav"
)

type Recording struct {
	Data       []byte
	Format     AudioFormat
	MIMEType   string
	SampleRate int
	Channels   int
}

type Transcript struct {
	Text string
}
