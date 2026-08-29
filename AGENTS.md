# AGENTS.md

面向 AI 编码代理与人类贡献者的仓库工作说明。

## 项目概览

本仓是 [einox](https://github.com/jumeng/einox)（agent 运行时基座）的官方示例仓：每个顶层目录一个完整可 `go run` 的示例，`internal/appsupport` 是共享的应用侧底座（文件版 `session.Store` + 终端事件打印器）。定位是消费者视角的真相——示例只能像真实用户一样 import einox 公开面。

## 常用命令

- 构建：`go build ./...`
- 测试：`go test ./...`（llmtest 剧本驱动，零端点零密钥，确定性）
- 运行示例：`go run ./minimal`（或 `./approval`、`./multiturn`）；`EINOX_EXAMPLE_DATA` 环境变量固定数据目录

## 维护约定

- **钉 einox 最新发布版本**（go.mod require 已发布 tag，不用 replace 指向本地或 main）——公开代理可见的版本才是消费者实际拿到的。einox 发新版后升级依赖并按编译器提示修示例；已知漂移先例：v0.1.1 的 `engine.NewManager` 是单返回值、`Options.Tools` 无 `SessionBrief` 入参（main 上已演进，README 快速开始是 main 形态）。
- **新示例遵循既有纪律**：llmtest 剧本驱动保证 CI 可跑；工具构造失败启动即拒（fail-fast，不静默吞错）；每轮 Run 后等 `TitleFlight()` 收口再读终态或开下一轮（首轮收尾的异步标题生成走同一模型工厂、同样消耗剧本槽位——不等待则剧本错位）；emit 回调不含 `user_message`（用户输入落流/订阅面才可见）。
- **「业务 0 import eino」**由仓根 `boundary_test.go` 守卫——示例扮演业务 agent 消费者，同主仓架构验收线。
- 事件目录、状态机、装配面全表等机制叙事归 einox 主仓 [docs](https://github.com/jumeng/einox/tree/main/docs)，本仓不复述（避免第二份会过期的真源）；示例注释只写该示例自身的通路与取舍。

## 代码风格

- 注释与文档用中文，标识符保持英文
- 新增示例保持单 main 包 + 单测试文件的形态；共享底座进 `internal/appsupport`，不在示例间复制粘贴

## 许可

Apache-2.0。引入第三方内容须文件头注明来源与协议（当前无第三方改编内容）。
