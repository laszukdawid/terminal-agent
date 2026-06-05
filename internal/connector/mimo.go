package connector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/auth"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"go.uber.org/zap"
)

const (
	MiMoProvider = "mimo"

	DefaultMiMoModel   = "mimo-v2.5-pro"
	DefaultMiMoBaseURL = "https://api.xiaomimimo.com/v1"

	miMoMaxRateLimitRetries = 10
	miMo429BodySnippetLimit = 500
)

type MiMoConnector struct {
	*OpenAIConnector
}

type miMoErrorResponse struct {
	Code    string             `json:"code"`
	Message string             `json:"message"`
	Type    string             `json:"type"`
	Error   *miMoErrorResponse `json:"error"`
}

func (r miMoErrorResponse) fields() (string, string, string) {
	if r.Error != nil {
		return r.Error.Code, r.Error.Message, r.Error.Type
	}
	return r.Code, r.Message, r.Type
}

func NewMiMoConnector(modelID *string) *MiMoConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewMiMoConnector")

	if modelID == nil || *modelID == "" {
		model := DefaultMiMoModel
		modelID = &model
	}

	apiKey := os.Getenv("MIMO_API_KEY")
	clientOptions := []option.RequestOption{
		option.WithBaseURL(getMiMoBaseURL()),
		option.WithMaxRetries(miMoMaxRateLimitRetries),
		option.WithMiddleware(newMiMoDiagnosticsMiddleware(&logger)),
	}
	authErr := error(nil)
	if apiKey == "" {
		authErr = fmt.Errorf("MIMO_API_KEY is required to use Xiaomi MiMo models")
	} else {
		clientOptions = append(clientOptions, option.WithAPIKey(apiKey))
	}

	client := openai.NewClient(clientOptions...)

	return &MiMoConnector{
		OpenAIConnector: &OpenAIConnector{
			client:  &client,
			logger:  logger,
			modelID: *modelID,
			auth: auth.ResolvedAuth{
				Type:  auth.CredentialTypeAPIKey,
				Token: apiKey,
			},
			authErr: authErr,
		},
	}
}

func getMiMoBaseURL() string {
	if baseURL := os.Getenv("MIMO_BASE_URL"); baseURL != "" {
		return baseURL
	}
	return DefaultMiMoBaseURL
}

func newMiMoDiagnosticsMiddleware(logger *zap.Logger) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		resp, err := next(req)
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			logMiMo429(req, resp, logger)
		}
		return resp, err
	}
}

func logMiMo429(req *http.Request, resp *http.Response, logger *zap.Logger) {
	body, readErr := io.ReadAll(resp.Body)
	if readErr == nil {
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}

	errorBody := miMoErrorResponse{}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &errorBody)
	}
	code, message, errorType := errorBody.fields()

	log := logger.Sugar().Debugw
	if retryCount, err := strconv.Atoi(req.Header.Get("X-Stainless-Retry-Count")); err == nil && retryCount >= miMoMaxRateLimitRetries {
		log = logger.Sugar().Warnw
	}

	log(
		"MiMo returned 429",
		"method", req.Method,
		"path", req.URL.Path,
		"retryCount", req.Header.Get("X-Stainless-Retry-Count"),
		"maxRetries", miMoMaxRateLimitRetries,
		"retryAfter", resp.Header.Get("Retry-After"),
		"retryAfterMs", resp.Header.Get("Retry-After-Ms"),
		"requestID", firstHeader(resp.Header, "X-Request-ID", "Request-ID", "X-Mimo-Request-ID", "X-Stainless-Request-ID"),
		"contentType", resp.Header.Get("Content-Type"),
		"contentLength", resp.ContentLength,
		"bodyBytes", len(body),
		"bodySnippet", truncateMiMoLogValue(strings.TrimSpace(string(body)), miMo429BodySnippetLimit),
		"code", code,
		"message", message,
		"type", errorType,
		"bodyReadError", readErr,
	)
}

func firstHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func truncateMiMoLogValue(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "... [truncated]"
}
