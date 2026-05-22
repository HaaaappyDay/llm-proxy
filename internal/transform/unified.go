package transform

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ContentKind string

const (
	ContentText       ContentKind = "text"
	ContentToolCall   ContentKind = "tool_call"
	ContentToolResult ContentKind = "tool_result"
)

type UnifiedToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type UnifiedContentBlock struct {
	Type        ContentKind
	Text        string
	ToolCall    *UnifiedToolCall
	ToolCallID  string
	Extra       map[string]any
}

func TextBlock(text string) UnifiedContentBlock {
	return UnifiedContentBlock{Type: ContentText, Text: text}
}

func ToolCallBlock(id, name string, args map[string]any) UnifiedContentBlock {
	return UnifiedContentBlock{
		Type: ContentToolCall,
		ToolCall: &UnifiedToolCall{
			ID: id, Name: name, Arguments: args,
		},
	}
}

func ToolResultBlock(callID, text string) UnifiedContentBlock {
	return UnifiedContentBlock{Type: ContentToolResult, ToolCallID: callID, Text: text}
}

type UnifiedMessage struct {
	Role    Role
	Content []UnifiedContentBlock
}

func TextMessage(role Role, text string) UnifiedMessage {
	return UnifiedMessage{Role: role, Content: []UnifiedContentBlock{TextBlock(text)}}
}

type UnifiedTool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type UnifiedRequest struct {
	Model       string
	System      string
	Messages    []UnifiedMessage
	Tools       []UnifiedTool
	Temperature *float64
	MaxTokens   *int
	Metadata    map[string]any
	Extra       map[string]any
}

type UnifiedUsage struct {
	InputTokens  *int
	OutputTokens *int
}

type UnifiedResponse struct {
	ID           string
	Model        string
	Message      UnifiedMessage
	FinishReason string
	Usage        *UnifiedUsage
	Extra        map[string]any
}
