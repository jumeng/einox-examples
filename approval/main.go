// approval 演示 hitl 审批通路：manual 档下写工具调用 → 工具以 *Suspend
// 挂起、引擎落检查点并发出 approval_request → 应用侧登记决议 → Resume 从
// 检查点重放，工具读到决议继续执行 → 轮收束。写面名单是业务内容
// （hitl.ApprovalConfig），工具实现自身不落审批语义。
//
// 跑法：go run ./approval
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jumeng/einox/checkpoint"
	"github.com/jumeng/einox/contract"
	"github.com/jumeng/einox/engine"
	"github.com/jumeng/einox/hitl"
	"github.com/jumeng/einox/llm"
	"github.com/jumeng/einox/llmtest"
	"github.com/jumeng/einox/session"
	"github.com/jumeng/einox/tools"

	"github.com/jumeng/einox-examples/internal/appsupport"
)

// noteTool 业务写工具：落一份笔记文件，并经 ctx 里的变更记录器报备
// （报备会汇入 session_end 的文件变更清单）。
func noteTool(dataDir string) contract.Tool {
	type out struct {
		Path  string `json:"path,omitempty"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	t, err := tools.InferTool("write_note", "把内容写入笔记文件",
		func(ctx context.Context, in struct {
			Content string `json:"content" jsonschema:"description=要写入的笔记内容"`
		}) (out, error) {
			path := filepath.Join(dataDir, "notes.md")
			if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
				return out{Error: err.Error()}, nil // 业务性失败走信封回喂模型自纠；Go error 会终止整轮
			}
			if rec := contract.ChangeRecorderOf(ctx); rec != nil {
				rec(path, "created")
			}
			return out{Path: path, OK: true}, nil
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

	fm := llmtest.New(
		llmtest.Turn{ToolCalls: []llmtest.ToolCallSpec{
			{ID: "call_1", Name: "write_note", Args: `{"content":"今天学会了 einox 的审批通路。"}`},
		}},
		llmtest.Turn{Text: "笔记已写入，任务完成。"},
		llmtest.Turn{Text: "审批通路示例"},
	)

	m, err := engine.NewManager(reg, engine.Options{
		Providers: func() []llm.ProviderSpec { return []llm.ProviderSpec{appsupport.FakeProvider()} },
		Instruction: func(sess engine.SessionBrief) string {
			return "你是示例助手。需要记笔记时调用 write_note 工具。"
		},
		Tools: func(sess engine.SessionBrief) []contract.Tool {
			return []contract.Tool{noteTool(st.Dir())}
		},
		CheckPoints: func(operator, sid string) engine.CheckPointStore {
			return checkpoint.NewCheckPointStore(st, operator, sid)
		},
		WorkspaceRoot: func(owner, sid string) string {
			return filepath.Join(st.UserTreeDir(owner), "workspaces", sid)
		},
		Approval: hitl.ApprovalConfig{
			WriteTools: map[string]bool{"write_note": true}, // 写面名单：业务内容
			Actions:    map[string]string{"write_note": "写入笔记"},
		},
		NewModel: fm.Factory(),
	})
	if err != nil {
		return fmt.Errorf("装配失败: %w", err)
	}

	s := reg.Create("demo", "审批通路示例", contract.ModeManual,
		contract.UserPrefs{Model: appsupport.FakeModelKey, Effort: "low", Mode: contract.ModeManual})
	ctx := context.Background()

	// 第一段：Run 同步返回时本轮并未结束——写工具挂起，状态翻 pending_approval。
	if !s.BeginRun("") {
		return fmt.Errorf("会话 %s 抢占执行失败", s.SID)
	}
	fmt.Println("── 第一段：发起任务（manual 档，写操作需人工批准）")
	m.Run(ctx, s, "帮我记一条笔记：今天学会了 einox 的审批通路。", nil, appsupport.Printer("│ "))

	if s.StateOf() != session.StatePendingApproval {
		return fmt.Errorf("预期挂起审批，实际状态 %s", s.StateOf())
	}
	appID := s.PendingAppID()
	items := s.PendingItems()
	fmt.Printf("── 挂起中: approval_id=%s 待审项=%v\n", appID, items)
	if len(items) != 1 {
		return fmt.Errorf("预期 1 个待审项，实际 %d", len(items))
	}

	// 第二段：应用侧决议（真实产品里这是审批端点）——逐项登记 + 回执落流 +
	// 落盘，然后 Resume 从检查点重放续流。注意：BeginRun 是「新开一轮」的抢占
	// 闸门（running/pending 态恒 false）；挂起续流的入口就是 Resume 本身。
	fmt.Println("── 第二段：登记决议（批准）并续流")
	d := contract.ApprovalDecision{Approve: true}
	s.SetDecisionFor(items[0], d)
	s.RecordDecision(appID, d) // 事件流是回放重建审批卡终态的真源——只决议不落流，卡片永远停在待审
	reg.Persist(s)
	m.Resume(ctx, s, appsupport.Printer("│ "))

	if ch := s.TitleFlight(); ch != nil {
		<-ch
	}

	// 决议为拒绝时把上面的 Approve 换成 false 即可看到另一条路：工具收到
	// disapproved 信封回喂，模型自行调整方案——整轮不失败。
	note, err := os.ReadFile(filepath.Join(st.Dir(), "notes.md"))
	if err != nil {
		return fmt.Errorf("批准路径应已写入笔记: %w", err)
	}
	fmt.Printf("── 终态: state=%s 笔记内容=%q\n", s.StateOf(), string(note))
	return nil
}

func main() {
	dataDir := os.Getenv("EINOX_EXAMPLE_DATA")
	if dataDir == "" {
		dataDir, _ = os.MkdirTemp("", "einox-approval-*")
	}
	if err := run(dataDir); err != nil {
		log.Fatal(err)
	}
}
