// coding-minimal 演示极简编码装配 profile（docs/04「极简编码装配」小节的
// 可跑对照）：与 minimal（最小 Options）的差异在裁剪取向——本 profile 裁
// 交互三件（todo/ask/plan——单用户 CLI 形态常冗余）、保执行三件（fs/cmd/
// patch = read/write/edit/bash 的 einox 对位），并配 ContextBudget 常驻面
// 账本与收紧的 AGENTS.md 字节预算。验证纪律靠 FinalGate 而非提示词堆叠——
// 极简不等于无门。模型经 llmtest 剧本注入（零端点零密钥可跑）。
//
// 跑法：go run ./coding-minimal
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jumeng/einox/checkpoint"
	"github.com/jumeng/einox/contract"
	"github.com/jumeng/einox/engine"
	"github.com/jumeng/einox/llm"
	"github.com/jumeng/einox/llmtest"
	"github.com/jumeng/einox/session"
	"github.com/jumeng/einox/tools"

	"github.com/jumeng/einox-examples/internal/appsupport"
)

// echoTool 业务工具示例：描述按渐进披露守则写——3–6 行讲清「何时用/语义
// 边界」，不写 few-shot 示例（前沿模型训练时见过同形 schema）。
func echoTool() contract.Tool {
	t, err := tools.InferTool("echo_note",
		"把一段确认性备注写入工作区 notes.txt（追加一行）。仅用于记录任务过程中的结论性备注；正式产出请用文件面工具。",
		func(ctx context.Context, in struct {
			Note string `json:"note"`
		}) (struct {
			Wrote string `json:"wrote"`
		}, error) {
			return struct {
				Wrote string `json:"wrote"`
			}{Wrote: in.Note}, nil
		})
	if err != nil {
		log.Fatal(err)
	}
	return t
}

func run(dataDir string) error {
	st, err := appsupport.NewStore(dataDir)
	if err != nil {
		return err
	}
	reg := session.NewRegistry(st)

	// 剧本模型：第 1 次调用写备注（走工具）；第 2 次给出最终回复；第 3 次
	// 是首轮收尾后的异步标题生成。
	fm := llmtest.New(
		llmtest.Turn{ToolCalls: []llmtest.ToolCallSpec{{ID: "call_1", Name: "echo_note", Args: `{"note":"已核对现场"}`}}},
		llmtest.Turn{Text: "备注已记录，任务完成。"},
		llmtest.Turn{Text: "极简编码示例"},
	)

	m, err := engine.NewManager(reg, engine.Options{
		Providers: func() []llm.ProviderSpec { return []llm.ProviderSpec{appsupport.FakeProvider()} },
		Instruction: func(sess engine.SessionBrief) string {
			// 短形态：业务职责一段即可——验证纪律靠 FinalGate 判据而非提示词堆叠。
			return "你是极简编码助手：先核对现场再动手，产出后自验。"
		},
		Tools: func(sess engine.SessionBrief) []contract.Tool { return []contract.Tool{echoTool()} },
		CheckPoints: func(operator, sid string) engine.CheckPointStore {
			return checkpoint.NewCheckPointStore(st, operator, sid)
		},
		WorkspaceRoot: func(owner, sid string) string {
			return filepath.Join(st.UserTreeDir(owner), "workspaces", sid)
		},
		NewModel: fm.Factory(),

		// ---- 极简编码 profile 的装配（区别于 minimal 之处）----
		// 裁交互三件，保 fs/cmd/patch 执行面：
		SessionToolsOff: []string{engine.FamilyTodo, engine.FamilyAsk, engine.FamilyPlan},
		// AGENTS.md 注入面收紧：
		AgentsMDMaxBytes: 8192,
		// 验证纪律归门不归提示词（判据归应用——示例用自包判据）：
		FinalGate: func(sess engine.SessionBrief) *engine.GateConfig {
			if sess.Mode != contract.ModeAuto {
				return nil // 演示只在 auto 档开门
			}
			return &engine.GateConfig{Checkers: []engine.GateChecker{func(ctx context.Context, root string) error {
				if strings.Contains(strings.ToLower(root), "不存在") { // 占位判据：真实形态 = go test / 自包审查
					return errors.New("判据失败示例")
				}
				return nil
			}}}
		},
		// 常驻面账本 ContextBudget（0=缺省关，推荐 8192）随 einox 下个发布版
		// 入 profile——发版后此处加一行 `ContextBudget: 8192,` 即得超限
		// harness_note（Kind: budget）告警（本仓钉已发布 tag，不用本地 replace）。
	})
	if err != nil {
		return fmt.Errorf("装配失败: %w", err)
	}

	s := reg.Create("demo", "极简编码示例", contract.ModeAuto,
		contract.UserPrefs{Model: appsupport.FakeModelKey, Effort: "low", Mode: contract.ModeAuto})

	if !s.BeginRun("") {
		return fmt.Errorf("会话 %s 抢占执行失败", s.SID)
	}
	m.Run(context.Background(), s, "核对后写一条备注", nil, appsupport.Printer("│ "))

	if ch := s.TitleFlight(); ch != nil {
		<-ch
	}

	// profile 效果可观察面：裁族后交互三件不在场（事件流无 todo/ask/plan 卡）。
	var toolsSeen int
	for _, ev := range s.SnapshotEvents() {
		if ev.Event == contract.EvToolCall {
			toolsSeen++
		}
	}
	fmt.Printf("── 终态: state=%s 工具调用=%d（交互三件已裁：无 todo/ask/plan 卡）\n",
		s.StateOf(), toolsSeen)
	fmt.Printf("── 会话记录: %s\n", filepath.Join(st.UserTreeDir("demo"), "sessions", s.SID, "session.json"))
	return nil
}

func main() {
	dataDir := os.Getenv("EINOX_EXAMPLE_DATA")
	if dataDir == "" {
		dataDir, _ = os.MkdirTemp("", "einox-coding-minimal-*")
	}
	if err := run(dataDir); err != nil {
		log.Fatal(err)
	}
}
