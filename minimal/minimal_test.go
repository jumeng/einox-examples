package main

import "testing"

// 冒烟：完整跑通一轮（剧本模型驱动，确定性）。
func TestRun(t *testing.T) {
	if err := run(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
