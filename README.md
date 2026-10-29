# OCS Go 答题服务

一个本机运行的 OCS 自定义题库服务，使用 OpenAI Responses API。支持单选、多选、判断和填空，以及题干或选项中以常见图片扩展名结尾的 HTTP(S) 图片地址。图片必须能被上游模型访问；OCS 未提供图片地址时无法识别图片。

## 运行

需要 Go 1.24 或更新版本。通过当前终端的环境变量配置：

```powershell
$env:OPENAI_BASE_URL = '<Responses API 的基础地址，通常以 /v1 结尾>'
$env:OPENAI_API_KEY = '<上游密钥>'
$env:OPENAI_MODEL = '<支持 Responses 和结构化输出的模型名>'
$env:OCS_TOKEN = '<自行生成的访问令牌>'
go run .
```

服务默认监听 `127.0.0.1:5000`，可通过 `OCS_LISTEN` 修改。服务会向基础地址追加 `/responses`；如果服务商地址已包含该路径，请将它从基础地址移除。程序不读取 `.env` 文件。不要把真实凭据提交到 Git 或分享 OCS 配置。

## OCS 配置

在 OCS 的题库配置中导入以下 JSON，替换 `TOKEN`。使用脚本管理器的跨域请求可能需要授权访问本机地址，具体取决于 OCS 版本和脚本管理器。

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

OCS 必须提供明确题型和可用选项；模型无法给出可校验答案时服务返回错误。多选和填空答案用 `#` 连接，实际填空匹配效果取决于 OCS 和课程页面。模型可能答错，请自行检查。

上游网络错误、429 或 5xx 最多尝试三次，遵循可用的 `Retry-After`，总超时为 150 秒。上游超时后重试可能造成重复调用和额外费用。服务不保存题目或答案；题目及图片 URL 会发送给配置的上游服务。
