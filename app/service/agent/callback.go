package agent

import (
	"context"
	"log/slog"

	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

var _ callbacks.Handler = (*LogCallbackHandler)(nil)

type LogCallbackHandler struct{}

func (l LogCallbackHandler) HandleText(ctx context.Context, text string) {
	//slog.DebugContext(ctx, "Text", "text", text)
}

func (l LogCallbackHandler) HandleLLMStart(ctx context.Context, prompts []string) {
	//slog.DebugContext(ctx, "LLM start", "prompts", prompts)
}

func (l LogCallbackHandler) HandleLLMGenerateContentStart(ctx context.Context, ms []llms.MessageContent) {
	//slog.DebugContext(ctx, "LLM generate content start", "messages", ms)
}

func (l LogCallbackHandler) HandleLLMGenerateContentEnd(ctx context.Context, res *llms.ContentResponse) {
	//slog.DebugContext(ctx, "LLM generate content end", "response", res)
}

func (l LogCallbackHandler) HandleLLMError(ctx context.Context, err error) {
	slog.ErrorContext(ctx, "LLM error", "error", err)
}

func (l LogCallbackHandler) HandleChainStart(ctx context.Context, inputs map[string]any) {
	//slog.DebugContext(ctx, "Chain start", "inputs", inputs)
}

func (l LogCallbackHandler) HandleChainEnd(ctx context.Context, outputs map[string]any) {
	//slog.DebugContext(ctx, "Chain end", "outputs", outputs)
}

func (l LogCallbackHandler) HandleChainError(ctx context.Context, err error) {
	slog.ErrorContext(ctx, "Chain error", "error", err)
}

func (l LogCallbackHandler) HandleToolStart(ctx context.Context, input string) {
	//slog.DebugContext(ctx, "Tool start", "input", input)
}

func (l LogCallbackHandler) HandleToolEnd(ctx context.Context, output string) {
	//slog.DebugContext(ctx, "Tool end", "output", output)
}

func (l LogCallbackHandler) HandleToolError(ctx context.Context, err error) {
	slog.ErrorContext(ctx, "Tool error", "error", err)
}

func (l LogCallbackHandler) HandleAgentAction(ctx context.Context, action schema.AgentAction) {
	//slog.DebugContext(ctx, "Agent action",
	//	"tool", action.Tool,
	//	"tool_input", action.ToolInput,
	//	"log", action.Log,
	//)
}

func (l LogCallbackHandler) HandleAgentFinish(ctx context.Context, finish schema.AgentFinish) {
	//slog.DebugContext(ctx, "Agent finish",
	//	"return_values", finish.ReturnValues,
	//	"log", finish.Log,
	//)
}

func (l LogCallbackHandler) HandleRetrieverStart(ctx context.Context, query string) {
	//slog.DebugContext(ctx, "Retriever start", "query", query)
}

func (l LogCallbackHandler) HandleRetrieverEnd(ctx context.Context, query string, documents []schema.Document) {
	//slog.DebugContext(ctx, "Retriever end",
	//	"query", query,
	//	"document_count", len(documents),
	//	"documents", documents,
	//)
}

func (l LogCallbackHandler) HandleStreamingFunc(ctx context.Context, chunk []byte) {
	//slog.DebugContext(ctx, "Streaming", "chunk", string(chunk))
}
