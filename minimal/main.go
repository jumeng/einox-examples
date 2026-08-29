// minimal 演示 einox 最小装配：Options 四项必填（Providers / Instruction /
// CheckPoints / WorkspaceRoot）+ 一个业务工具，模型经 llmtest 剧本注入
// （零端点零密钥可跑）。剧本推进完整一轮：用户消息 → 模型发起工具调用 →
// 工具结果回喂 → 最终回复 → 轮收束。
//
// 跑法：go run ./minimal
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jumeng/einox/checkpoint"
	"github.com/jumeng/einox/contract"
	"github.com/jumeng/einox/engine"
	"github.com/jumeng/einox/llm"
	"github.com/jumeng/einox/llmtest"
	"github.com/jumeng/einox/session"
	"github.com/jumeng/einox/tools"

	"github.com/jumeng/einox-examples/internal/appsupport"
)

// nowTool 业务工具：结构体反射出参数 schema（零 eino import 是装配侧的
// 架构验收线，本仓仓根的 boundary_test.go 守卫）。
func nowTool() contract.Tool {
	t, err := tools.InferTool("now", "查询当前时间（RFC3339）", func(ctx context.Context, in struct{}) (struct {
		Now string `json:"now"`
	}, error) {
		return struct {
			Now string `json:"now"`
		}{Now: time.Now().Format(time.RFC3339)}, nil
	})
	if err != nil {
		log.Fatal(err) // 构造期反射失败属装配错误：启动即拒，不静默吞错
	}
	return t
}

func run(dataDir string) error {
	st, err := appsupport.NewStore(dataDir)
	if err != nil {
		return err
	}
	reg := session.NewRegistry(st)

	// 剧本模型：第 1 次调用发起 now 工具调用；第 2 次给出最终回复；
	// 第 3 次是引擎首轮收尾后的异步标题生成（同一工厂构造，同样消耗剧本）。
	fm := llmtest.New(
		llmtest.Turn{ToolCalls: []llmtest.ToolCallSpec{{ID: "call_1", Name: "now", Args: "{}"}}},
		llmtest.Turn{Text: "当前时间是刚查到的那个，还有别的需要吗？"},
		llmtest.Turn{Text: "时间查询示例"},
	)

	m, err := engine.NewManager(reg, engine.Options{
		Providers: func() []llm.ProviderSpec { return []llm.ProviderSpec{appsupport.FakeProvider()} },
		Instruction: func(sess engine.SessionBrief) string {
			return "你是示例助手。需要时间信息时调用 now 工具。"
		},
		Tools: func(sess engine.SessionBrief) []contract.Tool { return []contract.Tool{nowTool()} },
		CheckPoints: func(operator, sid string) engine.CheckPointStore {
			return checkpoint.NewCheckPointStore(st, operator, sid)
		},
		WorkspaceRoot: func(owner, sid string) string {
			return filepath.Join(st.UserTreeDir(owner), "workspaces", sid)
		},
		NewModel: fm.Factory(), // 测试注入：接真实端点时删掉本行
	})
	if err != nil {
		return fmt.Errorf("装配失败: %w", err) // 必填项缺失/族名未知等装配错误启动期即拒
	}

	s := reg.Create("demo", "最小装配示例", contract.ModeManual,
		contract.UserPrefs{Model: appsupport.FakeModelKey, Effort: "low", Mode: contract.ModeManual})

	// Run 是同步阻塞的；事件经 emit 回调实时扇出（事件同时落会话记录）。
	// 注意 emit 回调不含 user_message——用户输入应用侧自有，落流/订阅面才
	// 可见（订阅形态见 multiturn 示例）。
	if !s.BeginRun("") {
		return fmt.Errorf("会话 %s 抢占执行失败", s.SID)
	}
	m.Run(context.Background(), s, "现在几点了？", nil, appsupport.Printer("│ "))

	// 首轮收尾会异步生成标题（经同一模型工厂）——等它落地再读终态。
	if ch := s.TitleFlight(); ch != nil {
		<-ch
	}

	fmt.Printf("── 终态: state=%s title=%q 模型调用数=%d 事件数=%d\n",
		s.StateOf(), s.TitleOf(), fm.CallCount(), len(s.SnapshotEvents()))
	fmt.Printf("── 会话记录: %s\n", filepath.Join(st.UserTreeDir("demo"), "sessions", s.SID, "session.json"))
	return nil
}

func main() {
	dataDir := os.Getenv("EINOX_EXAMPLE_DATA")
	if dataDir == "" {
		dataDir, _ = os.MkdirTemp("", "einox-minimal-*")
	}
	if err := run(dataDir); err != nil {
		log.Fatal(err)
	}
}
