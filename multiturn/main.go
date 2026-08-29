// multiturn 演示两件事：① 事件的第二种消费形态——订阅先于执行、经
// Subscribe 旁观通道收流（emit 回调之外，多视图/断线重连场景的形态）；
// ② 会话跨进程存活——落盘 session.json，新进程经 Reattach + LoadHistory
// 重建会话与历史，第二轮输入含第一轮消息（历史连续性由基座承担）。
//
// 跑法：go run ./multiturn
package main

import (
	"context"
	"errors"
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

	"github.com/jumeng/einox-examples/internal/appsupport"
)

func newManager(reg *session.Registry, st *appsupport.Store, fm *llmtest.Model) *engine.Manager {
	return engine.NewManager(reg, engine.Options{
		Providers: func() []llm.ProviderSpec { return []llm.ProviderSpec{appsupport.FakeProvider()} },
		Instruction: func(sess engine.SessionBrief) string {
			return "你是示例助手。"
		},
		// 本示例无业务工具：nil 即不装配，会话域基座件（todo/plan/fs 等）仍按缺省挂载
		Tools: nil,
		CheckPoints: func(operator, sid string) engine.CheckPointStore {
			return checkpoint.NewCheckPointStore(st, operator, sid)
		},
		WorkspaceRoot: func(owner, sid string) string {
			return filepath.Join(st.UserTreeDir(owner), "workspaces", sid)
		},
		NewModel: fm.Factory(),
	})
}

func run(dataDir string) error {
	st, err := appsupport.NewStore(dataDir)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// 剧本：第 1 轮回复、首轮收尾的异步标题、第 2 轮回复。
	fm := llmtest.New(
		llmtest.Turn{Text: "你好！我是第一轮的回复。"},
		llmtest.Turn{Text: "多轮会话示例"},
		llmtest.Turn{Text: "记得——你第一轮说「你好，第一轮」。历史已随第二轮带回模型。"},
	)

	fmt.Println("── 第一段：进程 A（Subscribe 旁观通道消费事件流）")
	regA := session.NewRegistry(st)
	mA := newManager(regA, st, fm)
	s := regA.Create("demo", "多轮会话示例", contract.ModeManual,
		contract.UserPrefs{Model: appsupport.FakeModelKey, Effort: "low", Mode: contract.ModeManual})

	p := appsupport.Printer("│ ")
	ch, _ := s.Subscribe() // 订阅先于执行：轻量轮次的事件不至于在启动间隙丢失
	if !s.BeginRun("") {
		return fmt.Errorf("会话 %s 抢占执行失败", s.SID)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// 事件已由 Session.Record 落流 + 扇出，emit 回调这里留空即可
		mA.Run(ctx, s, "你好，第一轮。", nil, func(session.Event) {})
	}()
	for ended := false; !ended; {
		select {
		case ev := <-ch:
			p(ev)
			if ev.Event == contract.EvSessionEnd || ev.Event == contract.EvError {
				ended = true
			}
		case <-time.After(10 * time.Second):
			return errors.New("等待事件流收线超时")
		}
	}
	<-done
	s.Unsubscribe(ch)
	if ch := s.TitleFlight(); ch != nil {
		<-ch
	}
	sid := s.SID
	fmt.Printf("── 进程 A 退出: sid=%s（会话记录已落盘）\n", sid)

	fmt.Println("── 第二段：进程 B 重启——Reattach 重建会话与历史，emit 回调形态续聊")
	// 与第一段对照：emit 回调不回传 user_message（用户输入应用侧自有），
	// 订阅通道则全量可见——两形态的真实差异。
	regB := session.NewRegistry(st)
	mB := newManager(regB, st, fm)
	s2 := regB.Reattach("demo", sid)
	if s2 == nil {
		return fmt.Errorf("Reattach %s 失败", sid)
	}
	regB.LoadHistory(s2)
	if !s2.BeginRun("") {
		return fmt.Errorf("重启后抢占执行失败")
	}
	mB.Run(ctx, s2, "第二轮：你还记得我第一轮说了什么吗？", nil, appsupport.Printer("│ "))
	if ch := s2.TitleFlight(); ch != nil {
		<-ch
	}

	// 历史连续性证据：共 3 次模型调用（两轮 + 首轮收尾的异步标题）；第二轮
	// 输入在第一轮基础上多了第一轮 assistant 回复与本轮用户消息——跨进程
	// 历史由基座从盘上恢复并带回模型。
	ins := fm.Inputs()
	if len(ins) != 3 {
		return fmt.Errorf("预期 3 次模型调用（两轮 + 标题），实际 %d", len(ins))
	}
	first, last := ins[0], ins[len(ins)-1]
	if len(last) != len(first)+2 {
		return fmt.Errorf("第二轮输入应在第一轮基础上多 2 条消息（assistant 回复 + 本轮输入），实际 %d → %d", len(first), len(last))
	}
	for _, it := range regB.List("demo") {
		fmt.Printf("── 会话列表: sid=%s title=%q state=%s\n", it.SID, it.Title, it.State)
	}
	fmt.Printf("── 模型输入消息数: 第一轮 %d → 第二轮 %d（历史跨轮带回）\n", len(first), len(last))
	return nil
}

func main() {
	dataDir := os.Getenv("EINOX_EXAMPLE_DATA")
	if dataDir == "" {
		dataDir, _ = os.MkdirTemp("", "einox-multiturn-*")
	}
	if err := run(dataDir); err != nil {
		log.Fatal(err)
	}
}
