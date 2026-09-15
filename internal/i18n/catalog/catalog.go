package catalog

import (
	"fmt"
	"regexp"
	"sort"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/i18n/locale"
)

type Catalog struct {
	language  string
	localizer *goi18n.Localizer
}

func New(language string) *Catalog {
	normalized := locale.Normalize(language)
	if normalized != locale.Chinese {
		normalized = locale.English
	}
	localizer := goi18n.NewLocalizer(bundle, normalized, locale.English)
	return &Catalog{language: normalized, localizer: localizer}
}

func (c *Catalog) Language() string {
	return c.language
}

func (c *Catalog) Text(messageID string, args map[string]string) string {
	text, err := c.localizer.Localize(&goi18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: args,
	})
	if err == nil && text != "" {
		return text
	}
	if text != "" {
		return text
	}
	fallback, _ := c.localizer.Localize(&goi18n.LocalizeConfig{MessageID: "error.internal"})
	if fallback != "" {
		return fallback
	}
	return "ask: unexpected internal error"
}

func (c *Catalog) Error(err error) string {
	info := failure.Info(err)
	if info == nil {
		return c.Text("error.internal", nil)
	}
	if _, ok := allMessages(locale.English)[info.MessageID]; !ok {
		return c.errorWithCause(info.Cause)
	}
	text := c.Text(info.MessageID, info.Args)
	if text == "" {
		return c.errorWithCause(info.Cause)
	}
	return text
}

func (c *Catalog) errorWithCause(cause error) string {
	if cause != nil {
		return fmt.Sprintf("%s: %v", c.Text("error.internal", nil), cause)
	}
	return c.Text("error.internal", nil)
}

func Keys(language string) []string {
	messages := allMessages(language)
	keys := make([]string, 0, len(messages))
	for key := range messages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var bundle = newBundle()

var placeholderPattern = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)

func newBundle() *goi18n.Bundle {
	bundle := goi18n.NewBundle(language.English)
	bundle.MustAddMessages(language.English, messagesFor(allMessages(locale.English))...)
	bundle.MustAddMessages(language.MustParse(locale.Chinese), messagesFor(allMessages(locale.Chinese))...)
	return bundle
}

func allMessages(language string) map[string]string {
	base := english
	menu := englishMenu
	if language == locale.Chinese {
		base = chinese
		menu = chineseMenu
	}
	messages := make(map[string]string, len(base)+len(menu))
	for key, value := range base {
		messages[key] = value
	}
	for key, value := range menu {
		messages[key] = value
	}
	return messages
}

func messagesFor(values map[string]string) []*goi18n.Message {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	messages := make([]*goi18n.Message, 0, len(keys))
	for _, key := range keys {
		messages = append(messages, &goi18n.Message{
			ID:    key,
			Other: placeholderPattern.ReplaceAllString(values[key], "{{.${1}}}"),
		})
	}
	return messages
}

var english = map[string]string{
	"error.internal":                             "ask: unexpected internal error",
	"usage.error":                                "ask: {message}",
	"usage.separator_option":                     "argument error: the first token after `--` cannot start with `-`: `{token}`\nTo send the literal text `-- {token}` as the question, run:\n  {correction}",
	"usage.no_question":                          "usage: ask [flags] <question>\nConfigure: ask config\nConversations: ask conversations",
	"usage.conflict":                             "argument error: `{left}` conflicts with `{right}`",
	"usage.value_required":                       "argument error: `{flag}` requires a value",
	"usage.invalid_value":                        "argument error: invalid value `{value}` for `{flag}`; expected {expected}",
	"usage.unknown_command":                      "argument error: unknown command `{command}`",
	"config.not_found":                           "configuration not found at {path}; run `ask config`",
	"config.read_failed":                         "cannot read configuration {path}: {reason}",
	"config.decode_failed":                       "cannot parse configuration {path}: {reason}",
	"config.write_failed":                        "cannot write configuration {path}: {reason}",
	"config.encode_failed":                       "cannot encode configuration",
	"config.secret_field_forbidden":              "configuration contains a forbidden secret field: {field}",
	"config.user_dir_unavailable":                "cannot determine the user configuration directory",
	"config.tui_not_available":                   "the configuration interface is not available in this build",
	"config.version_unsupported":                 "unsupported configuration version: {version}",
	"config.active_provider_missing":             "active_provider is not configured",
	"config.active_provider_unknown":             "active provider `{provider}` does not exist",
	"config.provider_id_invalid":                 "invalid provider ID `{provider}`; use lowercase letters, digits, and hyphens",
	"config.provider_format_invalid":             "provider `{provider}` has unsupported API format `{value}`; use an OpenAI or Anthropic compatible format",
	"config.base_url_missing":                    "provider `{provider}` has no Base URL",
	"config.base_url_invalid":                    "Base URL `{url}` is invalid",
	"config.provider_base_url_invalid":           "provider `{provider}` Base URL `{url}` is invalid",
	"config.base_url_empty":                      "Base URL is required",
	"config.base_url_unparseable":                "Base URL cannot be parsed",
	"config.base_url_scheme_invalid":             "Base URL must use http or https",
	"config.base_url_host_missing":               "Base URL must include a host",
	"config.base_url_query_fragment":             "Base URL cannot contain a query or fragment",
	"config.default_model_missing":               "provider `{provider}` has no default model",
	"config.default_model_not_enabled":           "default model `{model}` is not enabled for provider `{provider}`",
	"config.enabled_models_missing":              "provider `{provider}` has no enabled models",
	"config.enabled_model_empty":                 "provider `{provider}` contains an empty model ID",
	"config.request_timeout_invalid":             "provider `{provider}` has an invalid request timeout",
	"config.header_name_invalid":                 "provider `{provider}` contains an empty custom header name",
	"config.header_reserved":                     "provider `{provider}` cannot override reserved header `{header}`",
	"config.header_value_invalid":                "provider `{provider}` custom header `{header}` contains an invalid newline",
	"config.retention_invalid":                   "invalid history retention value `{value}`; expected 7d, 30d, 60d, or never",
	"config.context_messages_invalid":            "max_context_messages must be greater than zero",
	"config.context_tokens_invalid":              "max_context_tokens must be greater than zero",
	"config.max_output_tokens_invalid":           "max_output_tokens must be greater than zero",
	"config.temperature_invalid":                 "temperature must be between 0 and 2",
	"config.reasoning_mode_invalid":              "invalid reasoning mode `{value}`",
	"config.reasoning_disabled_behavior_invalid": "invalid reasoning disabled behavior `{value}`; expected send or omit",
	"config.ui_language_invalid":                 "invalid UI language `{value}`; expected auto, zh-CN, or en-US",
	"config.render_mode_invalid":                 "invalid render mode `{value}`; expected auto, full, inline, or plain",
	"config.ui_markdown_invalid":                 "invalid Markdown mode `{value}`; expected auto, on, or off",
	"config.ui_color_invalid":                    "invalid color mode `{value}`; expected auto, on, or off",
	"auth.api_key_missing":                       "API key is not set for provider `{provider}`; expected environment variable `{env_var}`. Run `ask config` or set the variable.",
	"route.provider_unknown":                     "provider `{provider}` does not exist",
	"route.model_missing":                        "provider `{provider}` has no usable default model",
	"catalog.unsupported":                        "this provider does not expose a compatible model list",
	"conversations.tui_not_available":            "the conversation interface is not available in this build",
	"conversation.not_found":                     "conversation `{conversation}` was not found",
	"conversation.prefix_ambiguous":              "conversation prefix `{prefix}` matches more than one conversation",
	"conversation.title_empty":                   "conversation title cannot be empty",
	"conversation.lease_busy":                    "conversation `{conversation}` is currently in use",
	"context.question_too_large":                 "question exceeds the configured context token limit",
	"id.generate_failed":                         "cannot generate a unique ID",
	"request.create_failed":                      "cannot create provider request",
	"request.encode_failed":                      "cannot encode provider request",
	"request.canceled":                           "request canceled",
	"transport.timeout":                          "request to provider `{provider}` timed out",
	"transport.network":                          "network error for provider `{provider}`: {reason}",
	"transport.read_failed":                      "cannot read provider `{provider}` response",
	"upstream.auth_failed":                       "authentication failed for provider `{provider}` (HTTP {status})",
	"upstream.permission_denied":                 "permission denied for provider `{provider}` (HTTP {status})",
	"upstream.not_found":                         "provider `{provider}` endpoint was not found (HTTP {status}); check Base URL and API format",
	"upstream.rate_limited":                      "provider `{provider}` rate limited the request (HTTP {status})",
	"upstream.server_error":                      "provider `{provider}` returned a server error (HTTP {status})",
	"upstream.request_failed":                    "provider `{provider}` rejected the request (HTTP {status}): {reason}",
	"protocol.response_decode_failed":            "provider `{provider}` returned an invalid response: {reason}",
	"protocol.response_too_large":                "provider `{provider}` response exceeded the safe size limit",
	"protocol.stream_read_failed":                "stream from provider `{provider}` could not be read: {reason}",
	"stdin.read_failed":                          "cannot read standard input",
	"shell.unsupported":                          "unsupported shell; choose a profile manually",
	"shell.value_invalid":                        "the value cannot be written safely to the selected shell profile",
	"shell.block_invalid":                        "the ask managed block is incomplete or duplicated",
	"shell.backup_failed":                        "cannot create shell profile backup {path}: {reason}",
	"shell.write_failed":                         "cannot write shell profile {path}: {reason}",
	"shell.read_failed":                          "cannot read shell profile {path}: {reason}",
	"shell.profile_missing":                      "shell profile path is empty",
	"shell.profile_invalid":                      "shell profile path is invalid",
	"shell.detection_failed":                     "cannot detect shell `{shell}`; choose a shell and profile manually",
	"shell.variable_conflict":                    "environment variable `{variable}` already exists outside the ask managed block in {path}",
	"storage.path_missing":                       "database path is empty",
	"storage.open_failed":                        "cannot open database {path}: {reason}",
	"storage.permission_failed":                  "cannot restrict database permissions for {path}: {reason}",
	"storage.initialize_failed":                  "cannot initialize database: {reason}",
	"storage.migration_failed":                   "cannot migrate the conversation database",
	"storage.operation_failed":                   "database operation `{operation}` failed: {reason}",
	"title.client_missing":                       "title generation client is not configured",
	"title.empty":                                "title generation returned an empty title",
	"upstream.empty_response":                    "provider `{provider}` returned an empty response",
	"upstream.response_failed":                   "provider `{provider}` failed the request: {reason}",
	"output.json_failed":                         "cannot encode JSON output",
	"reasoning.anthropic_budget_invalid":         "Anthropic reasoning requires a positive token budget",
	"reasoning.model_unknown":                    "reasoning level is unknown for model `{model}`; configure an explicit reasoning mapping or use --no-thinking",
	"help.text": `ask - fast terminal AI questions

Usage:
  ask [flags] <question...>
  ask config
  ask conversations [list|select|delete|rename]
  ask --help
  ask --version

Examples:
  ask How does Go context work?
  ask -- config
  ask "-- --model" is what
  ask --thinking Analyze this race condition
  ask --no-thinking Translate this sentence

Flags:
  --lang <locale>              auto, zh-CN, or en-US
  -c, --conversation <id>      use a conversation
  -n, --new                    create a new conversation
  -p, --provider <id>          override the provider
  -m, --model <name>           override the model
  --thinking / --no-thinking   override reasoning for this request
  --show-thinking              show returned reasoning
  --hide-thinking              hide returned reasoning
  --system <text>              override the system prompt
  --no-stream                  wait for the complete response
  --markdown / --no-markdown   override Markdown rendering
  --no-color                   disable ANSI color
  --render-mode <mode>         auto, full, inline, or plain
  --json                       print only the final JSON result
  --quiet                      hide status and usage
  --max-context-messages <n>   override the message limit
  --timeout <duration>         override the request timeout

API compatibility:
  Only OpenAI Chat Completions, OpenAI Responses, and Anthropic Messages
  compatible endpoints are supported. Set the Base URL without protocol paths
  such as /v1/chat/completions.

API keys:
  ask config accepts a key through hidden input and writes it to a managed block
  in the selected shell profile. Source that profile or open a new terminal.

Reserved questions:
  "ask config" opens configuration. To ask the literal word config, use
  "ask -- config". A standalone "--" cannot be followed by an option-like
  question token. Use ask "-- --model" is what instead of
  ask -- --model is what.

CLI language follows the terminal locale. --lang and ui.language override it.
Model answers are not translated.
`,
	"version.text":    "ask {version} (commit {commit}, built {built})",
	"status.provider": "provider={provider} model={model} elapsed={elapsed} input={input} output={output}",
	"status.complete": "completed in {elapsed}",
}

var chinese = func() map[string]string {
	messages := make(map[string]string, len(english))
	for key, value := range english {
		messages[key] = value
	}
	messages["error.internal"] = "ask: 发生未预期的内部错误"
	messages["usage.error"] = "ask: 参数错误：{message}"
	messages["usage.separator_option"] = "ask: 参数错误：`--` 后不能直接跟以 `-` 开头的问题 token：`{token}`\n如需将 `-- {token}` 作为问题正文，请执行：\n  {correction}"
	messages["usage.no_question"] = "用法: ask [flags] <问题>\n配置 API: ask config\n选择对话: ask conversations"
	messages["usage.conflict"] = "ask: 参数错误：`{left}` 与 `{right}` 互斥"
	messages["usage.value_required"] = "ask: 参数错误：`{flag}` 需要一个值"
	messages["usage.invalid_value"] = "ask: 参数错误：`{flag}` 的值 `{value}` 无效；应为 {expected}"
	messages["config.not_found"] = "未找到配置文件 {path}；请运行 `ask config`"
	messages["config.read_failed"] = "无法读取配置文件 {path}：{reason}"
	messages["config.decode_failed"] = "无法解析配置文件 {path}：{reason}"
	messages["config.write_failed"] = "无法写入配置文件 {path}：{reason}"
	messages["config.encode_failed"] = "无法编码配置"
	messages["config.secret_field_forbidden"] = "配置中包含禁止的密钥字段：{field}"
	messages["config.user_dir_unavailable"] = "无法确定用户配置目录"
	messages["config.tui_not_available"] = "当前构建尚未提供配置界面"
	messages["config.version_unsupported"] = "配置版本 {version} 不受支持"
	messages["config.active_provider_missing"] = "尚未配置 active_provider"
	messages["config.active_provider_unknown"] = "active_provider 指向的 `{provider}` 不存在"
	messages["config.provider_id_invalid"] = "Provider ID `{provider}` 无效；只能使用小写字母、数字和连字符，且必须以小写字母开头"
	messages["config.provider_format_invalid"] = "Provider `{provider}` 的 API 格式 `{value}` 不受支持；请选择 OpenAI 或 Anthropic 兼容格式"
	messages["config.base_url_missing"] = "Provider `{provider}` 尚未配置 Base URL"
	messages["config.base_url_invalid"] = "Base URL `{url}` 无效"
	messages["config.provider_base_url_invalid"] = "Provider `{provider}` 的 Base URL `{url}` 无效"
	messages["config.base_url_empty"] = "请填写 Base URL"
	messages["config.base_url_unparseable"] = "无法解析 Base URL"
	messages["config.base_url_scheme_invalid"] = "Base URL 必须使用 http 或 https"
	messages["config.base_url_host_missing"] = "Base URL 必须包含主机名"
	messages["config.base_url_query_fragment"] = "Base URL 不能包含 query 或 fragment"
	messages["config.default_model_missing"] = "Provider `{provider}` 尚未配置默认模型"
	messages["config.default_model_not_enabled"] = "Provider `{provider}` 的默认模型 `{model}` 不在启用模型列表中"
	messages["config.enabled_models_missing"] = "Provider `{provider}` 尚未启用任何模型"
	messages["config.enabled_model_empty"] = "Provider `{provider}` 的启用模型列表包含空值"
	messages["config.request_timeout_invalid"] = "Provider `{provider}` 的请求超时无效"
	messages["config.header_name_invalid"] = "Provider `{provider}` 包含空的请求头名称"
	messages["config.header_reserved"] = "Provider `{provider}` 不能覆盖保留请求头 `{header}`"
	messages["config.header_value_invalid"] = "Provider `{provider}` 的请求头 `{header}` 包含非法换行"
	messages["config.retention_invalid"] = "history.retention 的值 `{value}` 无效；可选 7d、30d、60d 或 never"
	messages["config.context_messages_invalid"] = "max_context_messages 必须大于 0"
	messages["config.context_tokens_invalid"] = "max_context_tokens 必须大于 0"
	messages["config.max_output_tokens_invalid"] = "max_output_tokens 必须大于 0"
	messages["config.temperature_invalid"] = "temperature 必须在 0 到 2 之间"
	messages["config.reasoning_mode_invalid"] = "思考模式 `{value}` 无效"
	messages["config.reasoning_disabled_behavior_invalid"] = "思考关闭行为 `{value}` 无效；应为 send 或 omit"
	messages["config.ui_language_invalid"] = "ui.language 的值 `{value}` 无效；可选 auto、zh-CN 或 en-US"
	messages["config.render_mode_invalid"] = "render_mode 的值 `{value}` 无效；可选 auto、full、inline 或 plain"
	messages["config.ui_markdown_invalid"] = "Markdown 模式 `{value}` 无效；可选 auto、on 或 off"
	messages["config.ui_color_invalid"] = "颜色模式 `{value}` 无效；可选 auto、on 或 off"
	messages["auth.api_key_missing"] = "Provider `{provider}` 尚未设置 API key；应设置环境变量 `{env_var}`。请运行 `ask config` 或设置该变量。"
	messages["route.provider_unknown"] = "Provider `{provider}` 不存在"
	messages["route.model_missing"] = "Provider `{provider}` 没有可用的默认模型"
	messages["catalog.unsupported"] = "该 Provider 未提供兼容的模型列表"
	messages["conversations.tui_not_available"] = "当前构建尚未提供对话管理界面"
	messages["conversation.not_found"] = "未找到对话 `{conversation}`"
	messages["conversation.prefix_ambiguous"] = "对话 ID 前缀 `{prefix}` 匹配到多个对话"
	messages["conversation.title_empty"] = "对话标题不能为空"
	messages["conversation.lease_busy"] = "对话 `{conversation}` 正在使用中"
	messages["context.question_too_large"] = "问题超过配置的上下文 token 上限"
	messages["id.generate_failed"] = "无法生成唯一 ID"
	messages["request.create_failed"] = "无法创建 Provider 请求"
	messages["request.encode_failed"] = "无法编码 Provider 请求"
	messages["request.canceled"] = "请求已取消"
	messages["transport.timeout"] = "请求 Provider `{provider}` 超时"
	messages["transport.network"] = "Provider `{provider}` 网络错误：{reason}"
	messages["transport.read_failed"] = "无法读取 Provider `{provider}` 的响应"
	messages["upstream.auth_failed"] = "认证失败（HTTP {status}）"
	messages["upstream.permission_denied"] = "权限不足（HTTP {status}）"
	messages["upstream.not_found"] = "Provider `{provider}` endpoint 不存在（HTTP {status}）；请检查 Base URL 和 API 格式"
	messages["upstream.rate_limited"] = "Provider `{provider}` 请求受限（HTTP {status}）"
	messages["upstream.server_error"] = "Provider `{provider}` 服务错误（HTTP {status}）"
	messages["upstream.request_failed"] = "Provider `{provider}` 拒绝请求（HTTP {status}）：{reason}"
	messages["protocol.response_decode_failed"] = "Provider `{provider}` 返回了无效响应：{reason}"
	messages["protocol.response_too_large"] = "Provider `{provider}` 的响应超过安全大小限制"
	messages["protocol.stream_read_failed"] = "无法读取 Provider `{provider}` 的流：{reason}"
	messages["stdin.read_failed"] = "无法读取标准输入"
	messages["shell.unsupported"] = "shell 不受支持；请手工选择 profile"
	messages["shell.value_invalid"] = "该值无法安全写入所选 shell profile"
	messages["shell.block_invalid"] = "ask 受管理区块不完整或重复"
	messages["shell.backup_failed"] = "无法创建 shell profile 备份 {path}：{reason}"
	messages["shell.write_failed"] = "无法写入 shell profile {path}：{reason}"
	messages["shell.read_failed"] = "无法读取 shell profile {path}：{reason}"
	messages["shell.profile_missing"] = "shell profile 路径为空"
	messages["shell.profile_invalid"] = "shell profile 路径无效"
	messages["shell.detection_failed"] = "无法检测 shell `{shell}`；请手工选择 shell 和 profile"
	messages["shell.variable_conflict"] = "环境变量 `{variable}` 已在 {path} 的受管理区块之外存在"
	messages["storage.path_missing"] = "数据库路径为空"
	messages["storage.open_failed"] = "无法打开数据库 {path}：{reason}"
	messages["storage.permission_failed"] = "无法收紧数据库 {path} 的权限：{reason}"
	messages["storage.initialize_failed"] = "无法初始化数据库：{reason}"
	messages["storage.migration_failed"] = "无法迁移对话数据库"
	messages["storage.operation_failed"] = "数据库操作 `{operation}` 失败：{reason}"
	messages["title.client_missing"] = "尚未配置标题生成客户端"
	messages["title.empty"] = "标题生成返回了空标题"
	messages["upstream.empty_response"] = "模型返回了空响应"
	messages["upstream.response_failed"] = "Provider `{provider}` 请求失败：{reason}"
	messages["output.json_failed"] = "无法编码 JSON 输出"
	messages["reasoning.anthropic_budget_invalid"] = "Anthropic 思考模式需要大于 0 的 token budget"
	messages["reasoning.model_unknown"] = "无法识别模型 `{model}` 的思考强度；请配置明确映射或使用 --no-thinking"
	messages["usage.unknown_command"] = "ask: 参数错误：未知命令 `{command}`"
	messages["status.provider"] = "provider={provider} model={model} elapsed={elapsed} input={input} output={output}"
	messages["status.complete"] = "已在 {elapsed} 内完成"
	messages["help.text"] = `ask - 终端快捷 AI 问答

用法:
  ask [flags] <问题...>
  ask config
  ask conversations [list|select|delete|rename]
  ask --help
  ask --version

示例:
  ask Go 的 context 如何工作？
  ask -- config
  ask "-- --model" 是什么
  ask --thinking 分析这个竞态条件
  ask --no-thinking 将这句话翻译成英文

参数:
  --lang <locale>              auto、zh-CN 或 en-US
  -c, --conversation <id>      使用指定对话
  -n, --new                    创建新对话
  -p, --provider <id>          临时覆盖 Provider
  -m, --model <name>           临时覆盖模型
  --thinking / --no-thinking   临时覆盖思考模式
  --show-thinking              显示 Provider 返回的思考内容
  --hide-thinking              隐藏思考内容
  --system <text>              临时覆盖系统提示词
  --no-stream                  等待完整响应
  --markdown / --no-markdown   临时覆盖 Markdown 渲染
  --no-color                   禁用 ANSI 颜色
  --render-mode <mode>         auto、full、inline 或 plain
  --json                       只输出最终 JSON
  --quiet                      隐藏状态和用量
  --max-context-messages <n>   临时覆盖上下文消息数
  --timeout <duration>         临时覆盖请求超时

API 兼容性:
  仅支持 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages
  兼容服务。Base URL 不应包含 /v1/chat/completions 等协议后缀。

API key:
  ask config 会通过隐藏输入接收密钥，并写入所选 shell profile 的受管理区块。
  写入后请 source 该文件或重新打开终端。

保留词:
  ask config 进入配置；若要提问字面量 config，请执行 ask -- config。
  独立 -- 后不能跟选项样式的问题 token，应使用
  ask "-- --model" 是什么，而不是 ask -- --model 是什么。

CLI 语言跟随终端 locale，可用 --lang 或 ui.language 覆盖。
模型回答不会被自动翻译。
`
	messages["version.text"] = "ask {version} (commit {commit}, built {built})"
	return messages
}()
