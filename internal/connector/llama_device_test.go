package connector

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/jupiterrider/ffi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLlamaDevice(t *testing.T) {
	device, err := resolveLlamaDevice(nil)
	require.NoError(t, err)
	assert.Equal(t, "auto", device)

	device, err = resolveLlamaDevice(&QueryParams{Device: "cpu"})
	require.NoError(t, err)
	assert.Equal(t, "cpu", device)

	_, err = resolveLlamaDevice(&QueryParams{Device: "tpu"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of auto, cpu, gpu")
}

func TestApplyLlamaContextDeviceCPU(t *testing.T) {
	params := llama.ContextParams{Offload_kqv: 1}

	applyLlamaContextDevice(&params, "cpu")

	assert.Equal(t, uint8(0), params.Offload_kqv)
}

func TestEnsureROCmCompatDirCreatesCompatibilitySymlinks(t *testing.T) {
	originalStat := osStat
	originalSymlink := osSymlink
	originalMkdirAll := osMkdirAll
	defer func() {
		osStat = originalStat
		osSymlink = originalSymlink
		osMkdirAll = originalMkdirAll
	}()

	cacheDir := t.TempDir()
	t.Setenv("HOME", cacheDir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	libDir := t.TempDir()
	compatSource := filepath.Join(t.TempDir(), "libamdhip64.so.6")
	require.NoError(t, os.WriteFile(compatSource, []byte("stub"), 0644))
	compatSource2 := filepath.Join(t.TempDir(), "libhipblas.so.2")
	require.NoError(t, os.WriteFile(compatSource2, []byte("stub"), 0644))
	compatSource3 := filepath.Join(t.TempDir(), "librocblas.so.4")
	require.NoError(t, os.WriteFile(compatSource3, []byte("stub"), 0644))

	osStat = func(path string) (os.FileInfo, error) {
		switch path {
		case "/lib64/libamdhip64.so.6":
			return os.Stat(compatSource)
		case "/lib64/libhipblas.so.2":
			return os.Stat(compatSource2)
		case "/lib64/librocblas.so.4":
			return os.Stat(compatSource3)
		default:
			return os.Stat(path)
		}
	}

	compatDir, _, _, err := ensureROCmCompatDir(libDir)
	require.NoError(t, err)
	require.NotEmpty(t, compatDir)

	assert.Equal(t, "/lib64/libamdhip64.so.6", mustReadlink(t, filepath.Join(compatDir, "libamdhip64.so.7")))
	assert.Equal(t, "/lib64/libhipblas.so.2", mustReadlink(t, filepath.Join(compatDir, "libhipblas.so.3")))
	assert.Equal(t, "/lib64/librocblas.so.4", mustReadlink(t, filepath.Join(compatDir, "librocblas.so.5")))
}

func TestEnsureROCmCompatDirReturnsEmptyWhenNothingNeeded(t *testing.T) {
	libDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(libDir, "libamdhip64.so.7"), []byte("stub"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(libDir, "libhipblas.so.3"), []byte("stub"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(libDir, "librocblas.so.5"), []byte("stub"), 0644))

	compatDir, _, _, err := ensureROCmCompatDir(libDir)
	require.NoError(t, err)
	assert.Empty(t, compatDir)
}

func TestPreloadROCmCompatLibraries(t *testing.T) {
	originalOpen := ffiOpen
	defer func() { ffiOpen = originalOpen }()

	compatDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(compatDir, "libamdhip64.so.7"), []byte("stub"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(compatDir, "librocblas.so.5"), []byte("stub"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(compatDir, "libhipblas.so.3"), []byte("stub"), 0644))

	opened := []string{}
	ffiOpen = func(path string) (ffi.Lib, error) {
		opened = append(opened, path)
		return ffi.Lib{}, nil
	}

	preloaded, err := preloadROCmCompatLibraries(compatDir)
	require.NoError(t, err)
	assert.Equal(t, opened, preloaded)
	assert.Len(t, preloaded, 3)
	assert.Contains(t, preloaded, filepath.Join(compatDir, "libamdhip64.so.7"))
	assert.Contains(t, preloaded, filepath.Join(compatDir, "librocblas.so.5"))
	assert.Contains(t, preloaded, filepath.Join(compatDir, "libhipblas.so.3"))
}

func TestQueryWithLlamaCLIFallbackUnavailable(t *testing.T) {
	t.Setenv("YZMA_LIB", t.TempDir())
	lc := &LlamaConnector{}
	_, err := lc.queryWithLlamaCLI(context.Background(), "/tmp/model.gguf", &QueryParams{UserPrompt: strPtr("hello")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llama-cli fallback is unavailable")
}

func TestQueryUsesCLIFallbackForGPUWithoutInProcessDevices(t *testing.T) {
	originalExec := execCommandContext
	defer func() { execCommandContext = originalExec }()

	yzmaDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(yzmaDir, "llama-cli"), []byte("stub"), 0755))
	t.Setenv("YZMA_LIB", yzmaDir)

	called := false
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		called = true
		return exec.CommandContext(ctx, "bash", "-lc", "printf '> hello\n'")
	}

	lastLlamaRuntimePreparation = llamaRuntimePreparation{
		CompatDir:           t.TempDir(),
		AvailableGPUDevices: 0,
		CLIAvailableDevices: []string{"ROCm0", "ROCm1"},
	}

	lc := &LlamaConnector{}
	response, err := lc.queryWithLlamaCLI(context.Background(), "/tmp/model.gguf", &QueryParams{UserPrompt: strPtr("hello")})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Contains(t, response, "hello")
}

func TestFirstCLIGPUDevice(t *testing.T) {
	assert.Equal(t, "ROCm0", firstCLIGPUDevice([]string{"CPU", "ROCm0", "ROCm1"}))
	assert.Equal(t, "CUDA0", firstCLIGPUDevice([]string{"CPU", "CUDA0"}))
	assert.Empty(t, firstCLIGPUDevice([]string{"CPU"}))
}

func mustReadlink(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	require.NoError(t, err)
	return target
}

func strPtr(s string) *string {
	return &s
}
