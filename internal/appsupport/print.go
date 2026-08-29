// print.go 示例的「渲染层」：把 contract.Event 映射到终端输出。真实应用把
// 同一事件流映射到自己的管线（SSE/WebSocket/CLI 传输归应用——传输无关是
// 契约面的设计前提）。
package appsupport

import (
	"fmt"

	"github.com/jumeng/einox/contract"
	"github.com/jumeng/einox/llm"
	"github.com/jumeng/einox/session"
)

// Printer 返回事件打印回调（可直接作为 Manager.Run/Resume 的 emit 回调）。
// 前缀用于多阶段示例区分轮次来源。
func Printer(prefix string) func(session.Event) {
	inText := false // text_delta 无段界事件——下个非增量事件前补换行
	return func(ev session.Event) {
		if ev.Event == contract.EvTextDelta {
			fmt.Print(asDelta(ev.Data))
			inText = true
			return
		}
		if inText {
			fmt.Println()
			inText = false
		}
		switch ev.Event {
		case contract.EvUserMessage:
			fmt.Printf("%s用户   | %s\n", prefix, userText(ev.Data))
		case contract.EvToolCall:
			if c, ok := ev.Data.(contract.ToolCall); ok {
				fmt.Printf("%s工具调用 | %s(%s)\n", prefix, c.Tool, c.ArgsDigest)
			}
		case contract.EvToolResult:
			if r, ok := ev.Data.(contract.ToolResult); ok {
				fmt.Printf("%s工具结果 | %s %s\n", prefix, okMark(r.OK), r.Digest)
			}
		case contract.EvApprovalRequest:
			printApproval(prefix, ev.Data)
		case contract.EvApprovalDecision:
			if d, ok := ev.Data.(contract.DecisionOut); ok {
				fmt.Printf("%s决议   | %s\n", prefix, decisionText(d.Approve))
			}
		case contract.EvSessionEnd:
			if e, ok := ev.Data.(contract.SessionEnd); ok {
				fmt.Printf("%s── 轮收束 | 摘要: %s", prefix, e.Summary)
				for _, f := range e.Files {
					fmt.Printf("；%s(%s×%d)", f.Path, f.Action, f.Count)
				}
				fmt.Println()
			}
		case contract.EvError:
			if e, ok := ev.Data.(contract.ErrorOut); ok {
				fmt.Printf("%s错误   | [%s] %s\n", prefix, e.Code, e.Message)
			}
		case contract.EvThinkingDelta, contract.EvUsage:
			// 示例剧本不含思考增量；usage 静默——聚焦主链路
		default:
			fmt.Printf("%s事件   | %s\n", prefix, ev.Event)
		}
	}
}

func printApproval(prefix string, data any) {
	req, ok := data.(contract.ApprovalReq)
	if !ok {
		fmt.Printf("%s审批请求 | %v\n", prefix, data)
		return
	}
	fmt.Printf("%s审批请求 | %s（动作: %s）", prefix, req.Tool, req.Action)
	if req.Note != "" {
		fmt.Printf(" 注: %s", req.Note)
	}
	fmt.Println()
	for _, it := range req.Items {
		for _, p := range it.Plan {
			fmt.Printf("%s  ├ 待审 %s.%s %s ×%d\n", prefix, it.ItemID, it.Tool, p.Summary, p.Count)
		}
	}
}

func asDelta(d any) string {
	if dt, ok := d.(contract.Delta); ok {
		return dt.Delta
	}
	if m, ok := d.(map[string]any); ok { // 磁盘回放形态（json 反序列化）
		s, _ := m["delta"].(string)
		return s
	}
	return ""
}

func userText(d any) string {
	switch v := d.(type) {
	case contract.UserMsg:
		return v.Text
	case map[string]any:
		s, _ := v["text"].(string)
		return s
	}
	return ""
}

func okMark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func decisionText(approve bool) string {
	if approve {
		return "批准"
	}
	return "拒绝"
}

// FakeModelKey / FakeProvider 剧本端点：Providers 是必填项（空清单 = 未配置
// 模型错误面），但 NewModel 注入 llmtest 假模型后端点字段不再被真实消费——
// 复合键 fake/demo 只需与创建会话时的 UserPrefs.Model 一致。接真实端点时：
// 去掉 NewModel 注入、把 Providers 换成你的 ProviderSpec 清单即可。
const FakeModelKey = "fake/demo"

func FakeProvider() llm.ProviderSpec {
	return llm.ProviderSpec{
		ID: "fake", Name: "剧本端点", Kind: "openai", Enabled: true,
		Models: []llm.ModelSpec{{ID: "demo", Input: []string{"text"}, Priority: 100}},
	}
}
