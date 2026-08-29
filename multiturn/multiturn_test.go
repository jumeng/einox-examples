package main

import "testing"

// 冒烟：两轮 + 跨进程重建，断言历史连续性。
func TestRun(t *testing.T) {
	if err := run(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
