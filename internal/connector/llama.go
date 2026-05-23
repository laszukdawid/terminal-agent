package connector

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/hybridgroup/yzma/pkg/llama"
	yzmamessage "github.com/hybridgroup/yzma/pkg/message"
	yzmatemplate "github.com/hybridgroup/yzma/pkg/template"
	"github.com/jupiterrider/ffi"
	"go.uber.org/zap"
)

const (
	LlamaProvider     = "llama"
	DefaultLlamaModel = "llama3.2"
	defaultLlamaNCtx  = 4096
	defaultLlamaBatch = 2048
	defaultMaxTokens  = 600
	llamaTokenBufSize = 256
)

type LlamaConnector struct {
	logger    zap.Logger
	modelID   string
	modelPath string
	config    config.Config
}

var llamaRuntimeInit sync.Once

var lastLlamaRuntimePreparation llamaRuntimePreparation

var osStat = os.Stat

var osSymlink = os.Symlink

var osMkdirAll = os.MkdirAll

var ffiOpen = ffi.Load

var execCommandContext = exec.CommandContext

type llamaRuntimePreparation struct {
	LibPath              string
	CompatDir            string
	CompatCreated        []string
	CompatPreloaded      []string
	CompatMissing        []string
	LDLibraryPath        string
	GpuCapableRuntime    bool
	AvailableGPUDevices  int
	AvailableCPUDevices  int
	AvailableDeviceNames []string
	CLIAvailableDevices  []string
}

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

	device, err := resolveLlamaDevice(params)
	if err != nil {
		return "", err
	}
	lc.logger.Debug("llama query device resolved",
		zap.String("requested_device", params.Device),
		zap.String("resolved_device", device),
		zap.String("model_id", lc.modelID),
		zap.String("model_path", modelPath),
	)
	logLlamaRuntimePreparation(&lc.logger, device)
	if shouldUseLlamaCLIGPUFallback(device) {
		lc.logger.Debug("llama query using CLI GPU fallback",
			zap.Strings("cli_available_devices", lastLlamaRuntimePreparation.CLIAvailableDevices),
			zap.Strings("in_process_devices", lastLlamaRuntimePreparation.AvailableDeviceNames),
		)
		return lc.queryWithLlamaCLI(ctx, modelPath, params)
	}

	modelParams, err := buildLlamaModelParams(device)
	if err != nil {
		return "", err
	}
	lc.logger.Debug("llama model params prepared",
		zap.String("device", device),
		zap.Int32("n_gpu_layers", modelParams.NGpuLayers),
		zap.Bool("supports_gpu_offload", llama.SupportsGpuOffload()),
	)

	model, err := llama.ModelLoadFromFile(modelPath, modelParams)
	if err != nil {
		return "", fmt.Errorf("failed to load llama model %q from %q: %w", lc.modelID, modelPath, err)
	}
	if model == 0 {
		return "", fmt.Errorf("failed to load llama model %q from %q", lc.modelID, modelPath)
	}
	defer llama.ModelFree(model)

	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = defaultLlamaNCtx
	ctxParams.NBatch = defaultLlamaBatch
	ctxParams.NUbatch = defaultLlamaBatch
	applyLlamaContextDevice(&ctxParams, device)
	lc.logger.Debug("llama context params prepared",
		zap.String("device", device),
		zap.Uint8("offload_kqv", ctxParams.Offload_kqv),
		zap.Uint32("n_ctx", ctxParams.NCtx),
		zap.Uint32("n_batch", ctxParams.NBatch),
		zap.Uint32("n_ubatch", ctxParams.NUbatch),
	)

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

	maxTokens := resolveLlamaMaxTokens(params)

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

		chunk, err := llamaTokenToPiece(vocab, token)
		if err != nil {
			return strings.TrimSpace(response.String()), err
		}
		response.WriteString(chunk)

		if err := streamLlamaChunk(params, mdRenderer, chunk); err != nil {
			return "", err
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

func (lc *LlamaConnector) queryWithLlamaCLI(ctx context.Context, modelPath string, params *QueryParams) (string, error) {
	cliPath := filepath.Join(os.Getenv("YZMA_LIB"), "llama-cli")
	if !fileExists(cliPath) {
		return "", fmt.Errorf("llama-cli fallback is unavailable at %q", cliPath)
	}

	prompt, err := lc.buildPrompt(0, params)
	if err != nil {
		return "", err
	}

	maxTokens := resolveLlamaMaxTokens(params)

	compatDir := lastLlamaRuntimePreparation.CompatDir
	ldLibraryPath := strings.Join([]string{compatDir, os.Getenv("YZMA_LIB")}, string(os.PathListSeparator))
	deviceName := firstCLIGPUDevice(lastLlamaRuntimePreparation.CLIAvailableDevices)
	if deviceName == "" {
		return "", fmt.Errorf("llama-cli fallback could not find a GPU device from CLI device list: %v", lastLlamaRuntimePreparation.CLIAvailableDevices)
	}
	args := []string{
		"--model", modelPath,
		"--device", deviceName,
		"--gpu-layers", "all",
		"--ctx-size", fmt.Sprintf("%d", defaultLlamaNCtx),
		"--batch-size", fmt.Sprintf("%d", defaultLlamaBatch),
		"--ubatch-size", fmt.Sprintf("%d", defaultLlamaBatch),
		"--single-turn",
		"--conversation",
		"--no-display-prompt",
		"--simple-io",
		"--no-warmup",
		"--temp", "0.8",
		"--top-k", "40",
		"--top-p", "0.9",
		"--min-p", "0.1",
		"--n-predict", fmt.Sprintf("%d", maxTokens),
		"--prompt", prompt,
	}

	cmd := execCommandContext(ctx, cliPath, args...)
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+ldLibraryPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to capture llama-cli stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to capture llama-cli stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start llama-cli fallback: %w", err)
	}

	var stderrBuf strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stderrBuf, stderr)
		close(stderrDone)
	}()

	response, err := lc.collectCLIResponse(stdout, params)
	if err != nil {
		_ = cmd.Wait()
		return "", err
	}

	if err := cmd.Wait(); err != nil {
		<-stderrDone
		return "", fmt.Errorf("llama-cli fallback failed: %w: %s", err, strings.TrimSpace(stderrBuf.String()))
	}
	<-stderrDone

	return strings.TrimSpace(response), nil
}

func (lc *LlamaConnector) collectCLIResponse(stdout io.Reader, params *QueryParams) (string, error) {
	reader := bufio.NewReader(stdout)
	var response strings.Builder
	suppressUntilPrompt := true

	var mdRenderer *MarkdownStreamRenderer
	if params.Stream && params.OnStream == nil {
		renderer, err := NewMarkdownStreamRenderer()
		if err == nil {
			mdRenderer = renderer
		}
	}

	for {
		chunk, err := reader.ReadString('\n')
		if chunk != "" {
			if suppressUntilPrompt {
				trimmed := strings.TrimSpace(chunk)
				if trimmed == ">" || strings.HasPrefix(trimmed, "> ") {
					suppressUntilPrompt = false
					chunk = strings.TrimPrefix(chunk, "> ")
				} else {
					chunk = ""
				}
			}
			if strings.HasPrefix(strings.TrimSpace(chunk), "[ Prompt:") || strings.TrimSpace(chunk) == "Exiting..." {
				chunk = ""
			}
		}
		if chunk != "" {
			response.WriteString(chunk)
			if err := streamLlamaChunk(params, mdRenderer, chunk); err != nil {
				return "", err
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("failed reading llama-cli output: %w", err)
		}
	}

	if mdRenderer != nil {
		mdRenderer.Flush()
	}

	return response.String(), nil
}

func resolveLlamaDevice(params *QueryParams) (string, error) {
	device := "auto"
	if params != nil && params.Device != "" {
		device = params.Device
	}

	switch device {
	case "auto", "cpu", "gpu":
		return device, nil
	default:
		return "", fmt.Errorf("invalid device %q: must be one of auto, cpu, gpu", device)
	}
}

func buildLlamaModelParams(device string) (llama.ModelParams, error) {
	modelParams := llama.ModelDefaultParams()

	switch device {
	case "auto":
		return modelParams, nil
	case "cpu":
		modelParams.NGpuLayers = 0
		return modelParams, nil
	case "gpu":
		if !llama.SupportsGpuOffload() {
			return llama.ModelParams{}, fmt.Errorf("device gpu requested but loaded llama runtime does not support GPU offload")
		}
		modelParams.NGpuLayers = 999
		return modelParams, nil
	default:
		return llama.ModelParams{}, fmt.Errorf("invalid device %q: must be one of auto, cpu, gpu", device)
	}
}

func applyLlamaContextDevice(ctxParams *llama.ContextParams, device string) {
	if ctxParams == nil {
		return
	}

	if device == "cpu" {
		ctxParams.Offload_kqv = 0
	}
}

func resolveLlamaMaxTokens(params *QueryParams) int {
	if params != nil && params.MaxTokens > 0 {
		return params.MaxTokens
	}
	return defaultMaxTokens
}

func llamaTokenToPiece(vocab llama.Vocab, token llama.Token) (string, error) {
	buf := make([]byte, llamaTokenBufSize)
	n := llama.TokenToPiece(vocab, token, buf, 0, true)
	if n < 0 {
		return "", fmt.Errorf("failed to convert llama token %d to text", token)
	}
	if int(n) > len(buf) {
		buf = make([]byte, int(n))
		n = llama.TokenToPiece(vocab, token, buf, 0, true)
		if n < 0 || int(n) > len(buf) {
			return "", fmt.Errorf("failed to convert llama token %d to text", token)
		}
	}
	return string(buf[:int(n)]), nil
}

func streamLlamaChunk(params *QueryParams, mdRenderer *MarkdownStreamRenderer, chunk string) error {
	if params == nil || !params.Stream {
		return nil
	}
	if params.OnStream != nil {
		return params.OnStream(chunk)
	}
	if mdRenderer != nil {
		mdRenderer.ProcessChunk(chunk)
		return nil
	}
	fmt.Print(chunk)
	return nil
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
		prep := llamaRuntimePreparation{}
		libPath := os.Getenv("YZMA_LIB")
		if libPath == "" {
			initErr = fmt.Errorf("llama runtime library is not configured; set YZMA_LIB to the local llama.cpp shared library path")
			return
		}
		prep.LibPath = libPath

		compatDir, compatCreated, compatPreloaded, compatMissing, err := prepareLlamaRuntimeLibraries(libPath)
		if err != nil {
			initErr = err
			return
		}
		prep.CompatDir = compatDir
		prep.CompatCreated = compatCreated
		prep.CompatPreloaded = compatPreloaded
		prep.CompatMissing = compatMissing
		prep.LDLibraryPath = os.Getenv("LD_LIBRARY_PATH")

		if err := llama.Load(libPath); err != nil {
			initErr = fmt.Errorf("failed to load llama runtime library from YZMA_LIB=%q: %w", libPath, err)
			return
		}

		llama.LogSet(llama.LogSilent())
		llama.Init()
		if err := llama.GGMLBackendLoadAllFromPath(libPath); err != nil {
			initErr = fmt.Errorf("failed to load llama backend plugins from YZMA_LIB=%q: %w", libPath, err)
			return
		}
		prep.GpuCapableRuntime = llama.SupportsGpuOffload()
		prep.AvailableDeviceNames, prep.AvailableGPUDevices, prep.AvailableCPUDevices = detectLlamaBackendDevices()
		prep.CLIAvailableDevices = detectLlamaCLIDevices(libPath, prep.CompatDir)
		lastLlamaRuntimePreparation = prep
	})

	return initErr
}

func prepareLlamaRuntimeLibraries(libPath string) (string, []string, []string, []string, error) {
	compatDir, compatCreated, compatMissing, err := ensureROCmCompatDir(libPath)
	if err != nil {
		return "", nil, nil, nil, err
	}

	compatPreloaded, err := preloadROCmCompatLibraries(compatDir)
	if err != nil {
		return "", nil, nil, nil, err
	}
	if compatDir == "" {
		return "", compatCreated, compatPreloaded, compatMissing, nil
	}

	current := os.Getenv("LD_LIBRARY_PATH")
	paths := []string{compatDir, libPath}
	if current != "" {
		paths = append(paths, current)
	}
	if err := os.Setenv("LD_LIBRARY_PATH", strings.Join(paths, string(os.PathListSeparator))); err != nil {
		return "", nil, nil, nil, fmt.Errorf("failed to set LD_LIBRARY_PATH for llama runtime: %w", err)
	}

	return compatDir, compatCreated, compatPreloaded, compatMissing, nil
}

func ensureROCmCompatDir(libPath string) (string, []string, []string, error) {
	compatMap := map[string][]string{
		"libamdhip64.so.7": {"/lib64/libamdhip64.so.7", "/lib64/libamdhip64.so.6", "/lib64/libamdhip64.so"},
		"libhipblas.so.3":  {"/lib64/libhipblas.so.3", "/lib64/libhipblas.so.2", "/lib64/libhipblas.so"},
		"librocblas.so.5":  {"/lib64/librocblas.so.5", "/lib64/librocblas.so.4", "/lib64/librocblas.so"},
	}

	needed := false
	compatMissing := make([]string, 0, len(compatMap))
	for target, candidates := range compatMap {
		if fileExists(filepath.Join(libPath, target)) {
			continue
		}
		if _, ok := firstExistingPath(candidates); ok {
			needed = true
			continue
		}
		compatMissing = append(compatMissing, target)
	}
	if !needed {
		return "", nil, compatMissing, nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to resolve user cache dir for llama ROCm compatibility: %w", err)
	}
	compatDir := filepath.Join(cacheDir, "terminal-agent", "llama-rocm-compat")
	if err := osMkdirAll(compatDir, 0755); err != nil {
		return "", nil, nil, fmt.Errorf("failed to create llama ROCm compatibility dir %q: %w", compatDir, err)
	}

	compatCreated := make([]string, 0, len(compatMap))
	for target, candidates := range compatMap {
		dest := filepath.Join(compatDir, target)
		if fileExists(dest) {
			continue
		}
		source, ok := firstExistingPath(candidates)
		if !ok {
			continue
		}
		if err := osSymlink(source, dest); err != nil {
			if !fileExists(dest) {
				return "", nil, nil, fmt.Errorf("failed to create ROCm compatibility symlink %q -> %q: %w", dest, source, err)
			}
		}
		compatCreated = append(compatCreated, fmt.Sprintf("%s -> %s", dest, source))
	}

	return compatDir, compatCreated, compatMissing, nil
}

func firstExistingPath(paths []string) (string, bool) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := osStat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func preloadROCmCompatLibraries(compatDir string) ([]string, error) {
	if compatDir == "" {
		return nil, nil
	}

	libraries := []string{"libamdhip64.so.7", "librocblas.so.5", "libhipblas.so.3"}
	preloaded := make([]string, 0, len(libraries))
	for _, name := range libraries {
		path := filepath.Join(compatDir, name)
		if !fileExists(path) {
			continue
		}
		if _, err := ffiOpen(path); err != nil {
			return nil, fmt.Errorf("failed to preload ROCm compatibility library %q: %w", path, err)
		}
		preloaded = append(preloaded, path)
	}

	return preloaded, nil
}

func detectLlamaBackendDevices() ([]string, int, int) {
	deviceNames := []string{}
	gpuDevices := 0
	cpuDevices := 0
	count := llama.GGMLBackendDeviceCount()
	for i := uint64(0); i < count; i++ {
		dev := llama.GGMLBackendDeviceGet(i)
		if dev == 0 {
			continue
		}
		name := llama.GGMLBackendDeviceName(dev)
		deviceNames = append(deviceNames, name)
		if strings.Contains(strings.ToLower(name), "cpu") {
			cpuDevices++
		} else {
			gpuDevices++
		}
	}
	return deviceNames, gpuDevices, cpuDevices
}

func detectLlamaCLIDevices(libPath, compatDir string) []string {
	cliPath := filepath.Join(libPath, "llama-cli")
	if !fileExists(cliPath) {
		return nil
	}

	cmd := execCommandContext(context.Background(), cliPath, "--list-devices")
	ldLibraryPath := strings.Join([]string{compatDir, libPath}, string(os.PathListSeparator))
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+ldLibraryPath)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	devices := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "Available devices:" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			devices = append(devices, strings.TrimSpace(parts[0]))
		}
	}
	return devices
}

func firstCLIGPUDevice(devices []string) string {
	for _, device := range devices {
		lower := strings.ToLower(device)
		if strings.Contains(lower, "rocm") || strings.Contains(lower, "cuda") || strings.Contains(lower, "metal") || strings.Contains(lower, "vulkan") || strings.Contains(lower, "hip") {
			return device
		}
	}
	return ""
}

func shouldUseLlamaCLIGPUFallback(device string) bool {
	if device != "gpu" {
		return false
	}
	if lastLlamaRuntimePreparation.AvailableGPUDevices > 0 {
		return false
	}
	return firstCLIGPUDevice(lastLlamaRuntimePreparation.CLIAvailableDevices) != ""
}

func logLlamaRuntimePreparation(logger *zap.Logger, requestedDevice string) {
	prep := lastLlamaRuntimePreparation
	fields := []zap.Field{
		zap.String("requested_device", requestedDevice),
		zap.String("yzma_lib", prep.LibPath),
		zap.String("compat_dir", prep.CompatDir),
		zap.Strings("compat_created", prep.CompatCreated),
		zap.Strings("compat_preloaded", prep.CompatPreloaded),
		zap.Strings("compat_missing", prep.CompatMissing),
		zap.Bool("gpu_capable_runtime", prep.GpuCapableRuntime),
		zap.Int("available_gpu_devices", prep.AvailableGPUDevices),
		zap.Int("available_cpu_devices", prep.AvailableCPUDevices),
		zap.Strings("available_devices", prep.AvailableDeviceNames),
		zap.Strings("cli_available_devices", prep.CLIAvailableDevices),
	}
	if prep.LDLibraryPath != "" {
		fields = append(fields, zap.String("ld_library_path", prep.LDLibraryPath))
	}
	logger.Debug("llama runtime prepared", fields...)
	if requestedDevice == "gpu" && prep.AvailableGPUDevices == 0 && len(prep.CLIAvailableDevices) == 0 {
		logger.Warn("GPU device requested but neither in-process nor CLI llama runtime reported GPU backend devices",
			zap.String("yzma_lib", prep.LibPath),
			zap.Strings("available_devices", prep.AvailableDeviceNames),
			zap.Strings("cli_available_devices", prep.CLIAvailableDevices),
			zap.Strings("compat_preloaded", prep.CompatPreloaded),
			zap.Strings("compat_missing", prep.CompatMissing),
		)
	} else if requestedDevice == "gpu" && prep.AvailableGPUDevices == 0 && len(prep.CLIAvailableDevices) > 0 {
		logger.Debug("In-process llama runtime has no GPU devices; CLI fallback GPU devices detected",
			zap.Strings("available_devices", prep.AvailableDeviceNames),
			zap.Strings("cli_available_devices", prep.CLIAvailableDevices),
		)
	}
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
