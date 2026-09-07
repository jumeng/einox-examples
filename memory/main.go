// memory 演示跨会话记忆的三通道闭环（einox.agent.yaml 清单：preset
// support 打底 + 委托书覆盖项）：推通道 AgentsMD（owner 域 memory.md 按序
// 注入模型输入）+ 写通道 TurnEpilogue（轮自然收束把「标题/任务/摘要」追加
// 进 memory.md）+ 拉通道 recall（模型检索本 owner 历史会话）。交互节奏用
// plan 档计划卡一次授权：submit_plan 挂起 → 应用侧批准 → file_ticket 写
// 操作在任务期授权内直落（无第二次挂起）。会话二（同 owner 新会话）验证
// 记忆注入与检索。模型经 llmtest 剧本注入（零端点零密钥可跑）。
//
// 跑法：go run ./memory
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jumeng/einox/checkpoint"
	"github.com/jumeng/einox/contract"
	"github.com/jumeng/einox/engine"
	"github.com/jumeng/einox/hitl"
	"github.com/jumeng/einox/llm"
	"github.com/jumeng/einox/llmtest"
	"github.com/jumeng/einox/prompts"
	"github.com/jumeng/einox/session"
	"github.com/jumeng/einox/tools"

	"github.com/jumeng/einox-examples/internal/appsupport"
)

// fileTicketTool 业务写工具：为用户登记一张客服工单（owner 域 tickets/
// 子树，工作区外——工作区一轮一清，工单是业务数据须跨轮存活），并经 ctx
// 里的变更记录器报备（汇入 session_end 的文件变更清单）。
func fileTicketTool(st *appsupport.Store, owner string) contract.Tool {
	type out struct {
		Path  string `json:"path,omitempty"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	t, err := tools.InferTool("file_ticket",
		"为用户登记一张客服工单（写操作）。title 一句话概括故障或诉求；detail 补充现象与上下文；contact 用户联系方式。",
		func(ctx context.Context, in struct {
			Title   string `json:"title" jsonschema:"description=工单标题"`
			Detail  string `json:"detail,omitempty" jsonschema:"description=故障现象与上下文"`
			Contact string `json:"contact,omitempty" jsonschema:"description=联系方式"`
		}) (out, error) {
			rel := filepath.ToSlash(filepath.Join("tickets", time.Now().Format("20060102-150405")+".md"))
			doc := fmt.Sprintf("# %s\n\n- 详情: %s\n- 联系方式: %s\n- 状态: 已登记\n", in.Title, in.Detail, in.Contact)
			if err := st.WriteUserTreeFile(owner, rel, []byte(doc)); err != nil {
				return out{Error: err.Error()}, nil // 业务性失败走信封回喂模型自纠；Go error 会终止整轮
			}
			if rec := contract.ChangeRecorderOf(ctx); rec != nil {
				rec(filepath.Join(st.UserTreeDir(owner), filepath.FromSlash(rel)), "created")
			}
			return out{Path: rel, OK: true}, nil
		})
	if err != nil {
		log.Fatal(err) // 构造期反射失败属装配错误：启动即拒，不静默吞错
	}
	return t
}

func newManager(reg *session.Registry, st *appsupport.Store, fm *llmtest.Model) (*engine.Manager, error) {
	return engine.NewManager(reg, engine.Options{
		Providers: func() []llm.ProviderSpec { return []llm.ProviderSpec{appsupport.FakeProvider()} },
		// Instruction 拼装序（rules.md 基线律）：业务职责段 + prompts.Coding()
		// （fs 族在场——read_file/list_dir/search_files 为 recall 外置换指针
		// 取回与工单附件读取保留）+ 会话配置段（plan 档语义）。
		Instruction: func(sess engine.SessionBrief) string {
			var b strings.Builder
			b.WriteString("你是客服问答 agent「小印」。职责：解答用户问题、登记客服工单、跟进历史诉求。\n" +
				"记忆纪律：\n" +
				"- 对话开头注入的用户约定段是跨会话记忆（历史工单摘要与偏好），当作已知的用户背景使用，不重复索要其中已有信息。\n" +
				"- 用户提到「之前/上次/那个问题」或主题与历史相关时，先调用 recall 检索本用户历史会话再回答；检索不到就正常追问，最多换一次词重试。\n" +
				"- 需要登记工单时调用 file_ticket（写操作）。\n")
			b.WriteString(prompts.Coding())
			b.WriteString("\n会话配置：本会话运行在 plan（计划卡）档——涉及写操作的任务先 submit_plan 提交计划供用户审批，" +
				"批准后任务期内写操作免逐项确认，被拒按反馈修订重提；简单问答（无写操作）直接回答，不必提交计划。\n")
			return b.String()
		},
		Tools: func(sess engine.SessionBrief) []contract.Tool {
			return []contract.Tool{fileTicketTool(st, sess.Owner)}
		},
		CheckPoints: func(operator, sid string) engine.CheckPointStore {
			return checkpoint.NewCheckPointStore(st, operator, sid)
		},
		WorkspaceRoot: func(owner, sid string) string {
			return filepath.Join(st.UserTreeDir(owner), "workspaces", sid)
		},

		// ---- 清单启用项的装配（einox.agent.yaml）----
		// tools.session-tools-off: [cmd, patch]——纯对话无命令执行诉求；
		// fs/todo/ask/plan 族保留（recall 依赖律：fs 在场）。
		SessionToolsOff: []string{engine.FamilyCmd, engine.FamilyPatch},
		// harness.agentsmd: 记忆推通道——owner 域记忆文件按序注入。
		AgentsMD: func(sess engine.SessionBrief) []string {
			return []string{filepath.Join(st.UserTreeDir(sess.Owner), "memory.md")}
		},
		// harness.recall: 记忆拉通道（opt-in，装配即知情决策）。
		Recall: true,
		// engine 域记忆写通道：轮自然收束把「标题/任务/摘要」追加进 owner 域
		// memory.md，下一会话经 AgentsMD 注入——与 recall 合成读写环。epilogue
		// 先于异步标题执行（Title 恒空）——回退 Task 与基座列表同款。
		TurnEpilogue: func(sum engine.TurnEndSummary) {
			title := sum.Title
			if title == "" {
				title = sum.Task
			}
			path := filepath.Join(st.UserTreeDir(sum.Owner), "memory.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				log.Printf("memory: 记忆目录创建失败: %v", err)
				return
			}
			f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				log.Printf("memory: 记忆文件打开失败: %v", err)
				return
			}
			defer f.Close()
			fmt.Fprintf(f, "\n## %s\n- 任务: %s\n- 摘要: %s\n", title, sum.Task, sum.Summary)
		},
		// hitl.mode: plan + 写面名单（业务内容）：file_ticket（业务写工具）
		// 与 delete_file（fs 族保留面里唯一的写工具）。
		Approval: hitl.ApprovalConfig{
			WriteTools: map[string]bool{"file_ticket": true, "delete_file": true},
			Actions:    map[string]string{"file_ticket": "登记工单", "delete_file": "删除文件"},
		},
		NewModel: fm.Factory(), // 测试注入：接真实端点时删掉本行
	})
}

// run 两段演示：会话一登记工单（plan 档计划卡一次授权 + 记忆落盘）；会话二
// 同 owner 新会话（记忆注入 + recall 检索）。返回 error 形态供测试复用。
func run(dataDir string) error {
	st, err := appsupport.NewStore(dataDir)
	if err != nil {
		return err
	}
	reg := session.NewRegistry(st)
	ctx := context.Background()
	owner := "customer-01"

	// 剧本：会话一 4 次调用（submit_plan → file_ticket → 收口 → 首轮收尾
	// 异步标题）；会话二 3 次调用（recall → 收口 → 标题）。挂起续流（Resume）
	// 不重演挂起前的模型调用；经挂起+Resume 收尾的首轮同样触发异步标题
	// （U-1 修复：首轮标记锚定 Run 入口，挂起段入史不再污染判定）。
	fm := llmtest.New(
		llmtest.Turn{ToolCalls: []llmtest.ToolCallSpec{{ID: "c1", Name: "submit_plan", Args: `{"task":"登记打印机故障工单","summary":"为用户登记打印机无法打印的故障工单并告知跟进时限","steps":[{"title":"确认信息","detail":"确认故障现象与联系方式"},{"title":"登记工单","detail":"调用 file_ticket 落工单文件并回复用户"}],"risks":"无"}`}}},
		llmtest.Turn{ToolCalls: []llmtest.ToolCallSpec{{ID: "c2", Name: "file_ticket", Args: `{"title":"打印机无法打印","detail":"打印任务无输出，指示灯闪烁","contact":"customer-01"}`}}},
		llmtest.Turn{Text: "工单已登记，工程师将在 24 小时内联系您。"},
		llmtest.Turn{Text: "打印机报修跟进"}, // 会话一标题槽（genTitle 走 Generate，同耗剧本）
		llmtest.Turn{ToolCalls: []llmtest.ToolCallSpec{{ID: "c3", Name: "recall", Args: `{"query":"打印机"}`}}},
		llmtest.Turn{Text: "查到您上一轮会话登记过「打印机故障报修」工单，工程师将在 24 小时内联系您。"},
		llmtest.Turn{Text: "打印机工单追问"},
	)

	m, err := newManager(reg, st, fm)
	if err != nil {
		return fmt.Errorf("装配失败: %w", err)
	}

	// ── 会话一：登记工单（plan 档）──
	fmt.Println("── 会话一：登记工单（plan 档：计划卡一次授权）")
	s1 := reg.Create(owner, "打印机故障报修", contract.ModePlan,
		contract.UserPrefs{Model: appsupport.FakeModelKey, Effort: "low", Mode: contract.ModePlan})
	if !s1.BeginRun("") {
		return fmt.Errorf("会话 %s 抢占执行失败", s1.SID)
	}
	m.Run(ctx, s1, "我的打印机无法打印，帮我登记一张工单。", nil, appsupport.Printer("│ "))

	// Run 同步返回时本轮未结束：submit_plan 挂起等审批（plan_request 卡）。
	if s1.StateOf() != session.StatePendingApproval {
		return fmt.Errorf("预期计划卡挂起，实际状态 %s", s1.StateOf())
	}
	if kind, _ := s1.PendingDueOf(); kind != "plan" {
		return fmt.Errorf("预期挂起类型 plan，实际 %q", kind)
	}
	planID := s1.PendingAppID()
	fmt.Printf("── 计划卡挂起: plan_id=%s（应用侧批准 → Resume 续流）\n", planID)
	d := contract.ApprovalDecision{Approve: true}
	s1.SetDecision(d)               // 决议入槽（Resume 时 plan 工具消费——批准 = 授任务期写）
	s1.RecordPlanDecision(planID, d) // 回执落流（回放重建卡片终态的真源）
	reg.Persist(s1)
	m.Resume(ctx, s1, appsupport.Printer("│ "))
	if ch := s1.TitleFlight(); ch != nil {
		<-ch
	}
	if s1.StateOf() != session.StateEnded {
		return fmt.Errorf("会话一预期自然收束，实际状态 %s", s1.StateOf())
	}

	// 计划授权效果：file_ticket 在任务期授权内直落——全程无 approval_request。
	for _, ev := range s1.SnapshotEvents() {
		if ev.Event == contract.EvApprovalRequest {
			return fmt.Errorf("plan 档批准后写工具不应再逐项挂起（出现 approval_request）")
		}
	}
	tickets := filepath.Join(st.UserTreeDir(owner), "tickets")
	entries, err := os.ReadDir(tickets)
	if err != nil || len(entries) != 1 {
		return fmt.Errorf("预期 1 张工单文件，实际 %d（err=%v）", len(entries), err)
	}
	ticket, err := os.ReadFile(filepath.Join(tickets, entries[0].Name()))
	if err != nil {
		return err
	}
	if !strings.Contains(string(ticket), "打印机无法打印") {
		return fmt.Errorf("工单内容缺失: %s", ticket)
	}

	// 写通道证据：memory.md 已落会话一的「标题/任务/摘要」条目。
	mem1, err := os.ReadFile(filepath.Join(st.UserTreeDir(owner), "memory.md"))
	if err != nil {
		return fmt.Errorf("TurnEpilogue 应已写入记忆文件: %w", err)
	}
	if !strings.Contains(string(mem1), "\n## 打印机故障报修\n") {
		return fmt.Errorf("记忆条目应含任务标题回退，实际:\n%s", mem1)
	}
	title1 := s1.TitleOf()
	if title1 != "打印机报修跟进" {
		return fmt.Errorf("挂起+Resume 收尾的首轮应生成标题（U-1 修复），实得 %q", title1)
	}
	fmt.Printf("── 会话一收束: state=%s title=%q\n", s1.StateOf(), title1)
	fmt.Printf("── 记忆落盘: %s\n── 工单落盘: %s\n",
		filepath.Join(st.UserTreeDir(owner), "memory.md"), filepath.Join(tickets, entries[0].Name()))

	// ── 会话二：同 owner 新会话——推通道注入 + 拉通道检索 ──
	fmt.Println("── 会话二：新会话（同 owner）——记忆注入与检索")
	s2 := reg.Create(owner, "追问打印机工单进度", contract.ModePlan,
		contract.UserPrefs{Model: appsupport.FakeModelKey, Effort: "low", Mode: contract.ModePlan})
	if !s2.BeginRun("") {
		return fmt.Errorf("会话 %s 抢占执行失败", s2.SID)
	}
	m.Run(ctx, s2, "我上次报修的打印机现在什么进度？", nil, appsupport.Printer("│ "))
	if ch := s2.TitleFlight(); ch != nil {
		<-ch
	}
	if s2.StateOf() != session.StateEnded {
		return fmt.Errorf("会话二预期自然收束，实际状态 %s", s2.StateOf())
	}

	// 推通道证据：会话二首次模型输入含 memory.md 条目（AgentsMD 注入）。
	inputs := fm.Inputs()
	if len(inputs) != 7 { // 会话一 4 调（3 + 标题槽）+ 会话二 3 调（2 + 标题）
		return fmt.Errorf("预期 7 次模型调用，实际 %d", len(inputs))
	}
	var secondFirst strings.Builder
	for _, msg := range inputs[4] {
		secondFirst.WriteString(msg.Content)
	}
	if !strings.Contains(secondFirst.String(), "## 打印机故障报修") {
		return fmt.Errorf("会话二输入应含注入的记忆条目:\n%s", secondFirst.String())
	}
	// 拉通道证据：会话二第二次模型输入含 recall 信封（本 owner 历史会话，
	// 携 sid 与标题——命中会话一；标题为 U-1 修复后异步生成的真实标题）。
	var secondSecond strings.Builder
	for _, msg := range inputs[5] {
		secondSecond.WriteString(msg.Content)
	}
	for _, want := range []string{s1.SID, "打印机报修跟进"} {
		if !strings.Contains(secondSecond.String(), want) {
			return fmt.Errorf("recall 结果应含 %q:\n%s", want, secondSecond.String())
		}
	}
	// 读写环证据：会话二收束后记忆再增一条（累计 2 条）。
	mem2, err := os.ReadFile(filepath.Join(st.UserTreeDir(owner), "memory.md"))
	if err != nil {
		return err
	}
	if n := strings.Count(string(mem2), "\n## "); n != 2 {
		return fmt.Errorf("预期记忆累计 2 条，实际 %d:\n%s", n, mem2)
	}
	fmt.Printf("── 会话二收束: 记忆累计 %d 条（读写环闭合）\n", strings.Count(string(mem2), "\n## "))
	return nil
}

func main() {
	dataDir := os.Getenv("EINOX_EXAMPLE_DATA")
	if dataDir == "" {
		dataDir, _ = os.MkdirTemp("", "einox-memory-*")
	}
	if err := run(dataDir); err != nil {
		log.Fatal(err)
	}
}
