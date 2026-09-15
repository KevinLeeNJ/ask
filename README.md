# ask

`ask` 是一个终端快捷 AI 问答工具，面向快速提问和连续追问。它不会扫描当前仓库，也不会启动 Agent 工作流。

## 功能

- 一条命令快速提问，并自动保存对话上下文。
- 支持 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages 兼容 API。
- 支持流式 Markdown、独立展示 reasoning，并在正式回答开始时折叠思考内容。
- 使用本地 SQLite 保存对话记录，支持选择、重命名、删除和保留策略。
- `ask config` 提供菜单式配置和隐藏 API key 输入。
- 对话会继承最近一次使用的 Provider 和模型；命令参数可临时覆盖。
- 支持纯文本、管道、JSON 和不同终端能力下的输出模式。

## 安装

安装脚本会自动识别 macOS、Linux 以及 `amd64`、`arm64` 架构，下载最新 release 中的对应二进制，并校验 SHA-256。

macOS 或 Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/KevinLeeNJ/ask/main/install.sh | sh
```

默认安装到 `$HOME/.local/bin/ask`。如果该目录不在 `PATH` 中，脚本会输出需要添加的配置。也可以直接指定目录：

```bash
curl -fsSL https://raw.githubusercontent.com/KevinLeeNJ/ask/main/install.sh |
  sh -s -- --install-dir "$HOME/bin"
```

安装指定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/KevinLeeNJ/ask/main/install.sh |
  ASK_VERSION=v0.1.0 sh
```

Windows 可以在 Git Bash 或 WSL 中运行安装脚本，也可以直接从 [Releases](https://github.com/KevinLeeNJ/ask/releases) 下载对应的 `.zip` 或 `.tar.gz`。

安装完成后检查：

```bash
ask --version
```

## 快速开始

首次使用时运行配置向导：

```bash
ask config
```

向导会完成以下步骤：

1. 选择 OpenAI Chat Completions、OpenAI Responses 或 Anthropic Messages 兼容协议。
2. 配置 Provider ID、Base URL 和请求超时。
3. 隐藏输入 API key。
4. 获取并选择可用模型，或者手工输入模型 ID。
5. 选择默认模型。
6. 预览并确认 shell profile 写入内容。
7. 保存配置。

API key 不写入 `config.toml` 或 SQLite。写入 shell profile 后，执行界面显示的 `source` 命令，或者重新打开终端。

环境变量名由 Provider ID 自动生成：转为大写，将 `-` 替换为 `_`，最后追加 `_API_KEY`。例如：

```text
opencode       -> OPENCODE_API_KEY
openai-main    -> OPENAI_MAIN_API_KEY
```

配置完成后直接提问：

```bash
ask 解释 Go 的逃逸分析
ask 比较 PostgreSQL 和 SQLite
```

## 常用命令

```bash
# 普通提问；多个位置参数会合并成一个问题
ask 你是什么模型 你的架构是什么

# 开始新对话
ask --new 比较 PostgreSQL 和 SQLite

# 继续指定对话，支持完整 ID 或唯一前缀
ask --conversation 0192a 继续上一段分析

# 将保留词作为普通问题
ask -- config

# 临时指定 Provider 和模型
ask --provider opencode --model deepseek-v4.1-flash 分析这个竞态条件

# 临时开启或关闭思考
ask --thinking 分析这个死锁
ask --no-thinking 将这句话翻译成英文

# 对话管理
ask conversations
ask conversations list

# 机器可读输出
ask --json 概括这段日志
```

查看全部参数：

```bash
ask --help
```

## 对话与模型路由

已有对话会优先沿用该对话最新一次实际使用的 Provider 和模型。`--provider` 和 `--model` 只覆盖当前调用，不会修改永久配置。

默认模型由 `config.toml` 中的 `active_provider` 和对应 Provider 的 `default_model` 决定。

## 思考模式

`reasoning.mode` 支持：

- `auto`：使用本地多语言启发式规则判断是否需要思考，不发送额外探测请求；开启时只使用当前模型的最低有效等级。
- `on`：强制开启思考，使用最低有效等级。
- `off`：关闭思考。

临时覆盖：

```bash
ask --thinking 分析这个内存泄漏
ask --no-thinking 将 JSON 转成 YAML
```

遇到内置能力表未覆盖的模型时，显式开启思考会尝试从 models.dev 获取公开的 reasoning 元数据并缓存最低有效等级。

## 输出模式

- `plain`：原始 Markdown，无 ANSI，适用于管道、重定向和不支持终端控制的场景。
- `inline`：只追加输出，不使用清屏或光标回退。
- `full`：在安全 TTY 中使用局部重绘和 Markdown 语义渲染。
- `auto`：根据 stdout 和终端能力自动选择。

stdout 只承载正式回答或 JSON；进度、思考内容、错误和 usage 写入 stderr。

## 配置文件

默认配置位置：

```text
macOS / Linux: $HOME/.config/ask/config.toml
Windows:       %AppData%\ask\config.toml
```

SQLite 对话数据库位于同一目录：

```text
ask.db
```

可以使用 `ASK_CONFIG_FILE` 指定其他配置文件：

```bash
ASK_CONFIG_FILE="$HOME/.config/ask/work.toml" ask config
ASK_CONFIG_FILE="$HOME/.config/ask/work.toml" ask 你好
```

## 卸载

如果使用默认安装目录：

```bash
rm "$HOME/.local/bin/ask"
```

如需同时删除配置、密钥和对话记录：

```bash
rm -rf "$HOME/.config/ask"
```

如果配置向导向 shell profile 写入了 API key，请手动删除对应的受管理区块。

## 开发

需要 Go 1.25 或更高版本。

```bash
go test ./...
go vet ./...
```

发布由 GoReleaser 和 GitHub Actions 自动完成。维护者对目标 commit 打版本 tag 并推送：

```bash
git tag v0.1.1
git push origin v0.1.1
```

工作流会构建 macOS、Linux、Windows 的 `amd64` 和 `arm64` 二进制，生成校验文件，并创建 GitHub Release。

## License

[MIT](LICENSE)
