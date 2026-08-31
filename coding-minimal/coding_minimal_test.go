package main

import "testing"

// 冒烟：完整跑通一轮（剧本模型驱动，确定性）——裁交互三件 + FinalGate 开门
// （auto 档占位判据恒过）+ 剧本工具调用照常执行。
func TestRun(t *testing.T) {
	if err := run(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
