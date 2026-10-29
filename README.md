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
| `token` | 自行设置的 OCS 访问口令，建议使用较长的随机字母和数字。注意不是AI API的token！ |
| `listen` | 监听地址，默认为 `127.0.0.1:7777`。 |

`config.json` 包含密钥，请勿分享或提交到 Git。题目和图片地址会发送给配置的上游服务。

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

将 `TOKEN` 替换为 `config.json` 中的 `token`，保留 `Bearer `，然后解析并保存。修改服务监听端口后，也要修改这里的地址。脚本管理器提示跨域访问时，允许访问本机地址即可。

图片题需要 OCS 在题干或选项中传入 PNG、JPEG、WebP 或 GIF 图片地址，并由支持图片输入的模型处理；需要登录才能访问的图片可能不可用。模型可能答错，请自行检查。

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
