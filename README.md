# ocs-ai-answerer

通过 OpenAI Responses API 为 OCS 提供答题服务，支持单选、多选、判断和填空题。支持 Windows、Linux 和 macOS。

## 启动

运行与你的系统和处理器架构相匹配的程序。运行编译好的程序不需要 Go。

Windows：

```powershell
.\ocs-ai-answerer.exe
```

Linux / macOS：

```bash
chmod +x ./ocs-ai-answerer
./ocs-ai-answerer
```

首次启动会在程序旁生成 `config.json` 并退出。填写配置后再次运行；看到 `listening on 127.0.0.1:7777` 即表示服务已启动，按 `Ctrl+C` 停止。

## 配置

编辑 `config.json`：

| 字段 | 说明 |
| --- | --- |
| `base_url` | 上游 API 基础地址，程序会自动追加 `/responses`。 |
| `api_key` | 上游 API 密钥。 |
| `model` | 模型名称，需支持 Responses API 和结构化输出。 |
| `ocs_token` | OCS 访问口令数组，可设置多个。建议使用较长的随机字母和数字，与上游密钥不同。 |
| `listen` | 监听地址，默认为 `127.0.0.1:7777`。 |
| `reasoning_effort` | 可选，思考等级：`none`、`minimal`、`low`、`medium`、`high`、`xhigh` 或 `max`；留空使用上游默认值。可用等级取决于模型。 |
| `max_output_tokens` | 可选，输出 token 上限，正整数；设为 `0` 或省略时使用上游默认值。包含推理 token，设置过低可能导致回答不完整。 |
| `log_enabled` | 是否记录答题日志，默认为 `false`。 |

`config.json` 包含密钥，请勿分享或提交到 Git。题目和图片地址会发送给配置的上游服务。

已有配置需要将原来的 `token` 字符串改为 `ocs_token` 数组，例如 `["first", "second"]`；即使只有一个口令也要放在数组中。修改配置后重启程序。

如需使用其他配置文件，可运行 `./ocs-ai-answerer -config ./config.json`（Windows 使用 `.\ocs-ai-answerer.exe`）。指定文件不存在时，程序会生成空配置，其父目录须已存在。

## 接入 OCS

在 OCS 的“全局设置 → 题库配置”中选择自定义配置，粘贴以下内容：

```json
[
  {
    "name": "本地 AI 答题",
    "url": "http://127.0.0.1:7777/api/answer",
    "method": "post",
    "type": "GM_xmlhttpRequest",
    "contentType": "json",
    "headers": { "Authorization": "Bearer TOKEN", "Content-Type": "application/json" },
    "data": { "title": "${title}", "options": "${options}", "type": "${type}" },
    "handler": "return (res) => res.answer ? [res.question, res.answer] : undefined"
  }
]
```

将 `TOKEN` 替换为 `config.json` 中任一 `ocs_token`，保留 `Bearer `，然后解析并保存。修改服务监听端口后，也要修改这里的地址。脚本管理器提示跨域访问时，允许访问本机地址即可。

图片题需要 OCS 在题干或选项中传入 PNG、JPEG、WebP 或 GIF 图片地址，并由支持图片输入的模型处理；需要登录才能访问的图片可能不可用。模型可能答错，请自行检查。

## 答题日志

将 `log_enabled` 设为 `true` 后，程序会在配置文件旁的 `answers.jsonl` 中记录每个通过认证的请求，包括题目、OCS 口令前 12 个字符、接收请求的 Unix 时间、模型、思考等级、状态、耗时和上游返回的 token 用量。口令不足 12 个字符时会完整记录；日志可能包含敏感信息，注意保护和定期清理。

耗时单位为毫秒：`first_token_ms` 从接收请求计到上游第一段非空文本，`first_to_complete_ms` 从首段文本计到上游结束，`total_ms` 从接收请求计到本地响应完成。上游未提供的计时或用量为 `null`，不是 0。开启日志后会使用流式 Responses API，需要上游支持流式响应；重试等待计入总耗时，但用量仅来自最后一次上游响应。

`input_tokens` 包含缓存命中的输入 token，`output_tokens` 包含推理 token。思考等级为空表示既未配置，也未由上游报告。

## 从源码编译

安装 Go 1.24 或更新版本，在项目目录运行：

```powershell
# Windows
go build -o ocs-ai-answerer.exe .
```

```bash
# Linux / macOS
go build -o ocs-ai-answerer .
```
