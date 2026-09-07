package main

import "testing"

// 冒烟：跨会话记忆三通道闭环（剧本模型驱动，确定性）——会话一 plan 档
// 计划卡一次授权 + TurnEpilogue 记忆落盘；会话二 AgentsMD 注入 + recall
// 检索 + 记忆再增长。
func TestRun(t *testing.T) {
	if err := run(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
