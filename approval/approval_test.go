package main

import "testing"

// 冒烟：挂起 → 批准 → 续流全通路。
func TestRun(t *testing.T) {
	if err := run(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
