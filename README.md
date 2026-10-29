# ocs-ai-answerer

连接 OCS 网课助手和 OpenAI Responses API 的本地答题服务。支持单选、多选、判断和填空；题目中的图片地址也可以交给支持图片输入的模型处理。

## 使用现成程序

选择与你的操作系统和处理器架构相匹配的可执行文件。**运行编译好的程序不需要安装 Go**；Go 只用于从源码编译。

首次运行时，程序会在可执行文件旁生成 `config.json`，然后退出。用文本编辑器填写该文件，再次运行程序即可。该文件包含凭据，请不要分享或提交到 Git。

Windows，在程序所在目录打开 PowerShell：

```powershell
.\ocs-ai-answerer.exe
```

Linux 或 macOS，在程序所在目录打开终端：

```bash
chmod +x ./ocs-ai-answerer
./ocs-ai-answerer
```

如果程序目录不可写，可以使用 `-config` 指定其他位置，例如 `./ocs-ai-answerer -config ./config.json`。配置文件不存在时，会在指定位置生成；其父目录必须已经存在。

## 配置

编辑首次启动生成的 `config.json`。保留 JSON 语法，只需填写相应的字符串值：

| 字段 | 说明 |
| --- | --- |
| `base_url` | 上游 Responses API 的基础地址，通常以 `/v1` 结尾；程序自动追加 `/responses`。 |
| `api_key` | 上游 API 密钥。 |
| `model` | 上游提供的模型名称；需支持 Responses API 和结构化输出。 |
| `token` | 自己设置的 OCS 访问口令，建议使用较长的随机字母和数字，与上游密钥不同。 |
| `listen` | 服务监听地址，默认 `127.0.0.1:7777`；留空也使用默认值。 |

图片题需要模型支持图片输入。题目及图片地址会发送给配置的上游服务。程序不保存答题记录。

看到 `listening on 127.0.0.1:7777` 表示服务已启动。保持窗口打开，按 `Ctrl+C` 停止。以后启动会自动读取同一配置，不需要重新填写。

## 接入 OCS

在 OCS 的“全局设置”中打开“题库配置”，选择自定义配置，粘贴下面的 JSON：

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

将 `Bearer TOKEN` 中的 `TOKEN` 替换为 `config.json` 中的 `token`，保留前面的 `Bearer `，再解析并保存。不要分享填有真实口令的配置。脚本管理器可能要求允许访问本机地址，请在提示时授权。

若修改 `listen` 端口，也要同步修改 OCS 配置中的地址。默认地址指运行 OCS 的设备本身；不同设备需要另外配置可访问的监听地址和网络访问控制。

## 从源码编译

需要 [Go 1.24 或更新版本](https://go.dev/dl/)。在项目目录执行对应命令：

```powershell
go build -o ocs-ai-answerer.exe .
```

```bash
go build -o ocs-ai-answerer .
```

编译结果可直接复制到相同操作系统和处理器架构的电脑运行；配置文件需要一同复制或在新位置重新生成。

## 常见问题

- 无法创建配置：检查程序目录或 `-config` 指定目录是否可写。
- OCS 无法访问：确认服务运行中、地址和端口一致，并检查脚本管理器的跨域授权。
- 返回未授权：确认 OCS 配置中的口令与 `config.json` 中的 `token` 一致，且保留 `Bearer ` 前缀。
- 答案不可用：检查终端提示、基础地址、上游密钥、模型名称及其接口支持情况。
- 图片题不可用：OCS 必须实际传入图片地址，上游模型也必须能访问图片。需要登录才能访问的图片可能无法处理。
- 填空或多选未正确填写：答案使用 `#` 连接；实际匹配效果取决于 OCS 和课程页面。

上游网络错误、429 或 5xx 最多尝试三次，遵循可用的 `Retry-After`，总超时为 150 秒。超时后重试可能产生重复调用和额外费用。模型可能答错，请自行检查。
