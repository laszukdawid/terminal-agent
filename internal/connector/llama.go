package connector

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/hybridgroup/yzma/pkg/llama"
	yzmamessage "github.com/hybridgroup/yzma/pkg/message"
	yzmatemplate "github.com/hybridgroup/yzma/pkg/template"
	"go.uber.org/zap"
)

const (
	LlamaProvider     = "llama"
	DefaultLlamaModel = "llama3.2"
)

type LlamaConnector struct {
	logger    zap.Logger
	modelID   string
	modelPath string
	config    config.Config
}

var llamaRuntimeInit sync.Once

func NewLlamaConnector(modelID *string, cfg config.Config) *LlamaConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewLlamaConnector")

	if modelID == nil || *modelID == "" {
		model := DefaultLlamaModel
		modelID = &model
	}

	connector := &LlamaConnector{
		logger:  logger,
		modelID: *modelID,
		config:  cfg,
	}

	modelPath, err := resolveLlamaModelPath(*modelID, cfg)
	if err == nil {
		connector.modelPath = modelPath
	} else {
		logger.Debug("llama model path not resolved during initialization", zap.String("model", *modelID), zap.Error(err))
	}

	return connector
}

func (lc *LlamaConnector) Query(ctx context.Context, params *QueryParams) (string, error) {
	modelPath, err := lc.requireModelPath()
	if err != nil {
		return "", err
	}

	if params == nil {
		return "", fmt.Errorf("llama query params cannot be nil")
	}

	if err := initializeLlamaRuntime(); err != nil {
		return "", err
	}

	model, err := llama.ModelLoadFromFile(modelPath, llama.ModelDefaultParams())
	if err != nil {
		return "", fmt.Errorf("failed to load llama model %q from %q: %w", lc.modelID, modelPath, err)
	}
	if model == 0 {
		return "", fmt.Errorf("failed to load llama model %q from %q", lc.modelID, modelPath)
	}
	defer llama.ModelFree(model)

	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = 4096
	ctxParams.NBatch = 2048
	ctxParams.NUbatch = 2048

	modelCtx, err := llama.InitFromModel(model, ctxParams)
	if err != nil {
		return "", fmt.Errorf("failed to initialize llama context: %w", err)
	}
	defer llama.Free(modelCtx)

	llama.SetAbortCallback(modelCtx, func() bool {
		select {
		case <-ctx.Done():
			return true
		default:
			return false
		}
	})

	vocab := llama.ModelGetVocab(model)
	prompt, err := lc.buildPrompt(model, params)
	if err != nil {
		return "", err
	}

	tokens := llama.Tokenize(vocab, prompt, true, false)
	if len(tokens) == 0 {
		return "", fmt.Errorf("llama prompt tokenization returned no tokens")
	}

	sp := llama.DefaultSamplerParams()
	sp.Temp = 0.8
	sp.TopK = 40
	sp.TopP = 0.9
	sp.MinP = 0.1
	sp.Seed = llama.DefaultSeed

	sampler := llama.NewSampler(model, llama.DefaultSamplers, sp)
	defer llama.SamplerFree(sampler)

	batch := llama.BatchGetOne(tokens)
	if llama.ModelHasEncoder(model) {
		if _, err := llama.Encode(modelCtx, batch); err != nil {
			return "", fmt.Errorf("failed to encode llama prompt: %w", err)
		}

		start := llama.ModelDecoderStartToken(model)
		if start == llama.TokenNull {
			start = llama.VocabBOS(vocab)
		}
		batch = llama.BatchGetOne([]llama.Token{start})
	}

	if _, err := llama.Decode(modelCtx, batch); err != nil {
		return "", fmt.Errorf("failed to decode llama prompt: %w", err)
	}

	maxTokens := params.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 600
	}

	var mdRenderer *MarkdownStreamRenderer
	if params.Stream && params.OnStream == nil {
		renderer, err := NewMarkdownStreamRenderer()
		if err != nil {
			lc.logger.Warn("Failed to create markdown renderer, falling back to plain text", zap.Error(err))
		} else {
			mdRenderer = renderer
		}
	}

	var response strings.Builder
	for i := 0; i < maxTokens; i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		token := llama.SamplerSample(sampler, modelCtx, -1)
		if token == llama.TokenNull {
			return strings.TrimSpace(response.String()), fmt.Errorf("llama returned an invalid token during generation")
		}
		if llama.VocabIsEOG(vocab, token) {
			break
		}

		buf := make([]byte, 256)
		n := llama.TokenToPiece(vocab, token, buf, 0, true)
		chunk := string(buf[:n])
		response.WriteString(chunk)

		if params.Stream {
			if params.OnStream != nil {
				if err := params.OnStream(chunk); err != nil {
					return "", err
				}
			} else if mdRenderer != nil {
				mdRenderer.ProcessChunk(chunk)
			} else {
				fmt.Print(chunk)
			}
		}

		batch = llama.BatchGetOne([]llama.Token{token})
		if _, err := llama.Decode(modelCtx, batch); err != nil {
			return strings.TrimSpace(response.String()), fmt.Errorf("failed during llama token decode: %w", err)
		}
	}

	if mdRenderer != nil {
		mdRenderer.Flush()
	}

	return strings.TrimSpace(response.String()), nil
}

func (lc *LlamaConnector) QueryWithTool(_ context.Context, _ *QueryParams, _ map[string]tools.Tool) (LlmResponseWithTools, error) {
	if _, err := lc.requireModelPath(); err != nil {
		return LlmResponseWithTools{}, err
	}

	return LlmResponseWithTools{}, fmt.Errorf("llama provider does not support tool calling yet")
}

func (lc *LlamaConnector) requireModelPath() (string, error) {
	if lc.modelPath != "" {
		return lc.modelPath, nil
	}

	modelPath, err := resolveLlamaModelPath(lc.modelID, lc.config)
	if err != nil {
		return "", err
	}

	lc.modelPath = modelPath
	return modelPath, nil
}

func resolveLlamaModelPath(modelID string, cfg config.Config) (string, error) {
	if modelID == "" {
		return "", fmt.Errorf("llama model cannot be empty")
	}

	if fileExists(modelID) {
		return modelID, nil
	}

	if cfg == nil {
		return "", fmt.Errorf("llama model %q is not a readable file path and no llama_models alias config is available", modelID)
	}

	llamaModels := cfg.GetLlamaModels()
	modelPath, ok := llamaModels[modelID]
	if !ok || modelPath == "" {
		return "", fmt.Errorf("llama model %q is not configured in llama_models and is not a readable file path", modelID)
	}

	if !fileExists(modelPath) {
		return "", fmt.Errorf("llama model %q resolves to %q, but that file does not exist or is not readable", modelID, modelPath)
	}

	return modelPath, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func initializeLlamaRuntime() error {
	var initErr error
	llamaRuntimeInit.Do(func() {
		libPath := os.Getenv("YZMA_LIB")
		if libPath == "" {
			initErr = fmt.Errorf("llama runtime library is not configured; set YZMA_LIB to the local llama.cpp shared library path")
			return
		}

		if err := llama.Load(libPath); err != nil {
			initErr = fmt.Errorf("failed to load llama runtime library from YZMA_LIB=%q: %w", libPath, err)
			return
		}

		llama.LogSet(llama.LogSilent())
		llama.Init()
	})

	return initErr
}

func (lc *LlamaConnector) buildPrompt(model llama.Model, params *QueryParams) (string, error) {
	messages := make([]yzmamessage.Message, 0, len(params.Messages)+2)
	if params.SysPrompt != nil && *params.SysPrompt != "" {
		messages = append(messages, yzmamessage.Chat{Role: "system", Content: *params.SysPrompt})
	}
	for _, msg := range params.Messages {
		messages = append(messages, yzmamessage.Chat{Role: msg.Role, Content: msg.Content})
	}
	if params.UserPrompt != nil && *params.UserPrompt != "" {
		messages = append(messages, yzmamessage.Chat{Role: "user", Content: *params.UserPrompt})
	}

	if len(messages) == 0 {
		return "", fmt.Errorf("llama query requires at least one message or prompt")
	}

	tmpl := llama.ModelChatTemplate(model, "")
	if tmpl != "" {
		prompt, err := yzmatemplate.Apply(tmpl, messages, true)
		if err == nil {
			return prompt, nil
		}
		lc.logger.Warn("Failed to apply model chat template, falling back to plain prompt assembly", zap.Error(err), zap.String("model", lc.modelID))
	}

	var prompt strings.Builder
	for _, msg := range messages {
		chatMsg, ok := msg.(yzmamessage.Chat)
		if !ok {
			continue
		}
		role := strings.TrimSpace(chatMsg.Role)
		if role == "" {
			role = "user"
		}
		prompt.WriteString(strings.ToUpper(role))
		prompt.WriteString(": ")
		prompt.WriteString(chatMsg.Content)
		prompt.WriteString("\n\n")
	}
	prompt.WriteString("ASSISTANT: ")

	return prompt.String(), nil
}
