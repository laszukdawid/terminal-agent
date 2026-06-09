package gui

import "strings"

type transcriptBlockKind uint8

const (
	transcriptBlockProse transcriptBlockKind = iota
	transcriptBlockToolOutput
	transcriptBlockFinal
)

type transcriptBlock struct {
	Kind   transcriptBlockKind
	Text   string
	Chunks []string
}

func cloneTranscriptBlocks(blocks []transcriptBlock) []transcriptBlock {
	if len(blocks) == 0 {
		return nil
	}
	cloned := make([]transcriptBlock, len(blocks))
	for i, block := range blocks {
		cloned[i] = block
		if len(block.Chunks) > 0 {
			cloned[i].Chunks = append([]string(nil), block.Chunks...)
		}
	}
	return cloned
}

func (b transcriptBlock) content() string {
	if len(b.Chunks) == 0 {
		return b.Text
	}
	var out strings.Builder
	for _, chunk := range b.Chunks {
		out.WriteString(chunk)
	}
	return out.String()
}

func serializeTranscript(blocks []transcriptBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	serializer := transcriptSerializer{}
	for _, block := range blocks {
		switch block.Kind {
		case transcriptBlockToolOutput:
			serializer.appendToolOutput(block)
		case transcriptBlockFinal:
			serializer.appendFinalBlock(block.Text)
		default:
			serializer.appendProseBlock(block.Text)
		}
	}
	return serializer.String()
}

type transcriptSerializer struct {
	b           strings.Builder
	endsNewline bool
}

func (s *transcriptSerializer) String() string {
	return s.b.String()
}

func (s *transcriptSerializer) writeString(text string) {
	s.b.WriteString(text)
	s.endsNewline = strings.HasSuffix(text, "\n")
}

func (s *transcriptSerializer) writeByte(ch byte) {
	s.b.WriteByte(ch)
	s.endsNewline = ch == '\n'
}

func (s *transcriptSerializer) ensureNewline() {
	if s.b.Len() > 0 && !s.endsNewline {
		s.writeByte('\n')
	}
}

func (s *transcriptSerializer) appendProseBlock(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	s.ensureNewline()
	s.writeString(text)
	s.writeByte('\n')
}

func (s *transcriptSerializer) appendToolOutput(block transcriptBlock) {
	if block.Text == "" && len(block.Chunks) == 0 {
		return
	}
	s.ensureNewline()
	s.writeString(taskToolFenceMarker)
	s.writeByte('\n')
	endsWithNewline := false
	if len(block.Chunks) == 0 {
		s.writeString(block.Text)
		endsWithNewline = strings.HasSuffix(block.Text, "\n")
	} else {
		for _, chunk := range block.Chunks {
			s.writeString(chunk)
			endsWithNewline = strings.HasSuffix(chunk, "\n")
		}
	}
	if !endsWithNewline {
		s.writeByte('\n')
	}
	s.writeString(taskToolFenceMarker)
	s.writeByte('\n')
}

func (s *transcriptSerializer) appendFinalBlock(text string) {
	if text == "" {
		return
	}
	if s.b.Len() > 0 {
		s.ensureNewline()
		s.writeString("\n---\n\n")
	}
	s.writeString(text)
}
