# OCS Go 答题服务

一个连接 OCS 网课助手和 OpenAI Responses API 的本地题库服务。OCS 获取题目并填写答案，本程序将题目发送给模型，然后返回整理后的答案。

支持 Windows、Linux 和 macOS；支持单选、多选、判断和填空。题干或选项中包含常见图片格式的 HTTP(S) 地址时，也可以交给支持图片输入的模型处理。

## 准备

- 安装 [Go 1.24 或更新版本](https://go.dev/dl/)，在终端运行 `go version` 检查。
- 安装 OCS 网课助手，以及其所需的脚本管理器。
- 准备支持 Responses API 和结构化输出的模型、服务基础地址和 API 密钥。图片题还需要模型支持图片输入。
- 获取本项目源码，并打开项目目录中的终端。无需安装其他 Go 依赖。

## 编译

在项目目录运行：

Windows PowerShell：

```powershell
go build -o ocs-go-answerer.exe .
```

Linux 或 macOS 终端：

```bash
go build -o ocs-go-answerer .
```

编译后即可运行生成的文件，不需要额外安装。运行编译后的程序不需要 Go；如果复制到另一台电脑，需要为该电脑的操作系统和处理器架构编译。

## 配置并启动

下面的配置只对当前终端会话生效。`OCS_TOKEN` 是自己设置的访问口令，应使用较长的随机字母和数字，并与上游 API 密钥不同。不要将真实凭据提交到 Git 或分享给他人。

Windows PowerShell 7：

```powershell
$env:OPENAI_BASE_URL = Read-Host 'Responses API 基础地址'
$env:OPENAI_MODEL = Read-Host '模型名称'
$env:OPENAI_API_KEY = Read-Host '上游 API 密钥' -MaskInput
$env:OCS_TOKEN = Read-Host '自定义 OCS 访问口令' -MaskInput
.\ocs-go-answerer.exe
```

Linux 或 macOS，使用 Bash：

```bash
read -r -p 'Responses API 基础地址: ' OPENAI_BASE_URL
read -r -p '模型名称: ' OPENAI_MODEL
read -r -s -p '上游 API 密钥: ' OPENAI_API_KEY; printf '\n'
read -r -s -p '自定义 OCS 访问口令: ' OCS_TOKEN; printf '\n'
export OPENAI_BASE_URL OPENAI_MODEL OPENAI_API_KEY OCS_TOKEN
./ocs-go-answerer
```

macOS 默认终端可能使用 zsh；可先输入 `bash`，再运行上面的 Bash 命令。

| 配置项 | 说明 |
| --- | --- |
| `OPENAI_BASE_URL` | 上游 API 的基础地址，通常以 `/v1` 结尾。程序自动追加 `/responses`，不要在基础地址中重复填写该部分。 |
| `OPENAI_MODEL` | 上游提供的模型名称。 |
| `OPENAI_API_KEY` | 上游 API 密钥。 |
| `OCS_TOKEN` | 本地服务的访问口令，OCS 配置中需要填写相同的值。 |
| `OCS_LISTEN` | 可选，服务监听地址；默认 `127.0.0.1:5000`。 |

程序不自动读取 `.env` 文件。看到 `listening on 127.0.0.1:5000` 表示服务已经启动；保持终端打开，按 `Ctrl+C` 停止。关闭终端后重新启动时，需要再次设置这些环境变量。

## 接入 OCS

在 OCS 的“全局设置”中打开“题库配置”，选择自定义配置，粘贴下面的 JSON：

```json
[
  {
    "name": "本地 AI 答题",
    "url": "http://127.0.0.1:5000/api/answer",
    "method": "post",
    "type": "GM_xmlhttpRequest",
    "contentType": "json",
    "headers": { "Authorization": "Bearer TOKEN", "Content-Type": "application/json" },
    "data": { "title": "${title}", "options": "${options}", "type": "${type}" },
    "handler": "return (res) => res.answer ? [res.question, res.answer] : undefined"
  }
]
```

将 `Bearer TOKEN` 中的 `TOKEN` 替换为启动服务时设置的 `OCS_TOKEN`，保留前面的 `Bearer `，再解析并保存。不要把填有真实口令的配置分享给他人。脚本管理器可能会要求允许访问本机地址；请在提示时授权。

如果修改了 `OCS_LISTEN` 的端口，也要同步修改配置中的地址。默认地址指运行 OCS 的那台设备本身：若 OCS 和服务不在同一台设备上，需要另外配置可访问的监听地址和网络访问控制。

## 使用与排错

- OCS 显示无法访问：确认服务正在运行、地址和端口一致，并检查脚本管理器的跨域授权。
- 返回未授权：确认 OCS 配置中的口令与 `OCS_TOKEN` 一致，且保留 `Bearer ` 前缀。
- 答案不可用：检查终端提示，以及基础地址、上游密钥、模型名称和模型对 Responses API、结构化输出的支持情况。
- 图片题不可用：OCS 必须实际传入图片地址，上游模型也必须能访问图片。需要登录才能访问的图片可能无法处理。
- 填空或多选未正确填写：答案使用 `#` 连接；实际匹配效果取决于 OCS 和课程页面。

上游网络错误、429 或 5xx 最多尝试三次，遵循可用的 `Retry-After`，总超时为 150 秒。超时后重试可能产生重复调用和额外费用。本程序不保存题目或答案，但题目及图片地址会发送给配置的上游服务。模型可能答错，请自行检查。
