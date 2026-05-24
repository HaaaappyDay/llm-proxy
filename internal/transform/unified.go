package transform

import "fmt"

type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ContentKind string

const (
	ContentText       ContentKind = "text"
	ContentImage      ContentKind = "image"
	ContentFile       ContentKind = "file"
	ContentAudio      ContentKind = "audio"
	ContentToolCall   ContentKind = "tool_call"
	ContentToolResult ContentKind = "tool_result"
	ContentReasoning  ContentKind = "reasoning"
	ContentUnknown    ContentKind = "unknown"
)

type UnifiedToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
	RawInput  string
	Status    string
	Kind      string
}

type UnifiedContentBlock struct {
	Type       ContentKind
	Text       string
	Image      *UnifiedImage
	File       *UnifiedFile
	Audio      *UnifiedAudio
	ToolCall   *UnifiedToolCall
	ToolCallID string
	Reasoning  *UnifiedReasoning
	RawType    string
	Raw        map[string]any
	Extra      map[string]any
}

type UnifiedImage struct {
	URL       string
	Data      string
	MediaType string
	Detail    string
	FileID    string
}

type UnifiedFile struct {
	FileID    string
	FileData  string
	FileURL   string
	FileName  string
	MediaType string
}

type UnifiedAudio struct {
	Data   string
	Format string
}

type UnifiedReasoning struct {
	Text      string
	Signature string
	Encrypted string
}

func TextBlock(text string) UnifiedContentBlock {
	return UnifiedContentBlock{Type: ContentText, Text: text}
}

func ImageBlock(img UnifiedImage) UnifiedContentBlock {
	return UnifiedContentBlock{Type: ContentImage, Image: &img}
}

func FileBlock(file UnifiedFile) UnifiedContentBlock {
	return UnifiedContentBlock{Type: ContentFile, File: &file}
}

func AudioBlock(audio UnifiedAudio) UnifiedContentBlock {
	return UnifiedContentBlock{Type: ContentAudio, Audio: &audio}
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

func ReasoningBlock(reasoning UnifiedReasoning) UnifiedContentBlock {
	return UnifiedContentBlock{Type: ContentReasoning, Reasoning: &reasoning}
}

func UnknownBlock(sourceType string, raw map[string]any) UnifiedContentBlock {
	return UnifiedContentBlock{Type: ContentUnknown, RawType: sourceType, Raw: raw}
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
	Type        string
	Strict      *bool
	Raw         map[string]any
}

type UnifiedToolChoice struct {
	Mode string
	Name string
	Type string
	Raw  map[string]any
}

type UnifiedRequest struct {
	Model              string
	System             string
	Messages           []UnifiedMessage
	Tools              []UnifiedTool
	ToolChoice         *UnifiedToolChoice
	ResponseFormat     map[string]any
	PreviousResponseID string
	ParallelToolCalls  *bool
	Temperature        *float64
	TopP               *float64
	MaxTokens          *int
	Metadata           map[string]any
	Extra              map[string]any
}

type UnifiedUsage struct {
	InputTokens      *int
	OutputTokens     *int
	CacheReadTokens  *int
	CacheWriteTokens *int
	ReasoningTokens  *int
	TotalTokens      *int
}

type UnifiedResponse struct {
	ID           string
	Model        string
	Message      UnifiedMessage
	FinishReason string
	Usage        *UnifiedUsage
	Extra        map[string]any
}

type Format string

const (
	FormatAnthropic       Format = "anthropic_messages"
	FormatOpenAIChat      Format = "openai_chat_completions"
	FormatOpenAIResponses Format = "openai_responses"
)

type UnsupportedFeatureError struct {
	Source  Format
	Target  Format
	Feature string
	Message string
}

func (e *UnsupportedFeatureError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("%s cannot be converted from %s to %s", e.Feature, e.Source, e.Target)
}

func NewUnsupportedFeature(source, target Format, feature string) error {
	return &UnsupportedFeatureError{
		Source:  source,
		Target:  target,
		Feature: feature,
		Message: fmt.Sprintf("unsupported feature %q when converting from %s to %s", feature, source, target),
	}
}

func ValidateRequest(req UnifiedRequest, source, target Format) error {
	for _, tool := range req.Tools {
		if tool.Type != "" && tool.Type != "function" && target != FormatOpenAIResponses {
			return NewUnsupportedFeature(source, target, "hosted_or_custom_tool:"+tool.Type)
		}
	}
	if req.PreviousResponseID != "" && target != FormatOpenAIResponses {
		return NewUnsupportedFeature(source, target, "previous_response_id")
	}
	if len(req.ResponseFormat) > 0 && target == FormatAnthropic {
		return NewUnsupportedFeature(source, target, "structured_response_format")
	}
	if req.ToolChoice != nil {
		switch req.ToolChoice.Mode {
		case "", "none", "auto", "required", "any", "tool":
		default:
			return NewUnsupportedFeature(source, target, "tool_choice:"+req.ToolChoice.Mode)
		}
	}
	for _, msg := range req.Messages {
		for _, block := range msg.Content {
			if err := validateBlock(block, source, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func ValidateResponse(resp UnifiedResponse, source, target Format) error {
	for _, block := range resp.Message.Content {
		if err := validateBlock(block, source, target); err != nil {
			return err
		}
	}
	return nil
}

func validateBlock(block UnifiedContentBlock, source, target Format) error {
	switch block.Type {
	case ContentToolCall:
		if block.ToolCall != nil && block.ToolCall.Kind == "custom" && target != FormatOpenAIResponses {
			return NewUnsupportedFeature(source, target, "custom_tool_call")
		}
	case ContentAudio:
		if target != FormatOpenAIChat {
			return NewUnsupportedFeature(source, target, "audio_content")
		}
	case ContentFile:
		if target != FormatOpenAIResponses {
			return NewUnsupportedFeature(source, target, "file_content")
		}
	case ContentReasoning:
		if source != target {
			return NewUnsupportedFeature(source, target, "reasoning_or_thinking_content")
		}
	case ContentUnknown:
		if source != target {
			return NewUnsupportedFeature(source, target, "unknown_content:"+block.RawType)
		}
	}
	return nil
}
