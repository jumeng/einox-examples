# einox-examples

[einox](https://github.com/jumeng/einox)（可嵌入 agent 运行时基座）的可运行示例——每个目录是一个完整可 `go run` 的程序，覆盖从组装根（`engine.Options`）到事件消费、审批续流、会话跨进程存活的应用面。

所有示例默认经 `llmtest` 剧本假模型驱动：**零端点、零密钥**即可跑通完整链路；接真实模型只删一行注入（见 [minimal](minimal/main.go) 内注释）。

## 示例索引

| 目录 | 演示 |
|---|---|
| [minimal](minimal/) | 四项必填的最小装配 + 业务工具（`tools.InferTool`，结构体反射出 schema）+ emit 回调消费事件流，跑通「工具调用 → 结果回喂 → 收束」一整轮 |
| [approval](approval/) | hitl 审批通路：manual 档写工具挂起（`approval_request`）→ 应用侧登记决议 → `Resume` 从检查点重放续流 |
| [multiturn](multiturn/) | 事件消费第二形态：`Subscribe` 旁观通道；会话跨进程存活：新进程 `Reattach` + `LoadHistory` 续聊，历史自动带回模型 |

```bash
go run ./minimal     # 或 ./approval、./multiturn
go test ./...        # 全量冒烟（剧本确定性驱动）
```

数据目录默认用临时目录；设 `EINOX_EXAMPLE_DATA=/path/to/dir` 可固定落盘位置（检查 `users/<owner>/sessions/<sid>/session.json`、`checkpoints/` 与工作区布局）。

## 接真实模型

示例注入的 `NewModel: fm.Factory()` 行删掉即走真实端点；`Providers` 换成你的端点清单。复合键 `provider/model` 需与创建会话时的 `UserPrefs.Model` 一致：

```go
Providers: func() []llm.ProviderSpec {
    return []llm.ProviderSpec{{
        ID: "deepseek", Name: "DeepSeek",
        Kind:    "openai", // anthropic | openai
        BaseURL: "https://api.deepseek.com",
        APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
        Enabled: true,
        Models:  []llm.ModelSpec{{ID: "deepseek-chat", Input: []string{"text"}, Priority: 100}},
    }}
},
// 创建会话时对应：contract.UserPrefs{Model: "deepseek/deepseek-chat", Effort: "low", Mode: contract.ModeManual}
```

## 共享底座

`internal/appsupport` 是示例共享的应用侧底座：文件版会话存储（`session.Store` 契约的演示实现）与终端事件打印器（示例的「渲染层」——真实应用把同一事件流映射到 SSE/WS/CLI 管线）。生产应用实现自己的 Store 即可，注意契约要求 `UserTreeDir` 为本地文件系统路径。

## 边界守卫

仓根 `boundary_test.go` 守「业务 0 import eino」——三层栈（eino → einox → 业务）的架构验收线，业务仓抄同款测试进仓根即生效。

## 文档与延伸

- 装配面全表与能力说明：[einox docs](https://github.com/jumeng/einox/tree/main/docs)（03 能力面 / 04 装配 / 05 沙箱）
- 沙箱（`run_command` 围栏）、子代理、FinalGate 收束门、记忆通道等可选能力的装配见 docs/04 对应字段——示例按需在此仓补充
- eino 框架层（模型接入、chain/graph 编排、adk 原生用法）：[cloudwego/eino-examples](https://github.com/cloudwego/eino-examples)

## 许可

[Apache-2.0](LICENSE)
