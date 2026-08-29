// 业务侧边界守卫：「业务 0 import eino」是 einox 三层栈的架构验收线——
// 示例仓扮演业务 agent 消费者，同样守这道线。片段源自 einox docs/04 装配
// 指南（任何命名的业务仓抄进仓根即生效）。
package examples_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNoEinoImports(t *testing.T) {
	cmd := exec.Command("go", "list", "-test", "-f",
		"{{.ImportPath}}\t{{range .Imports}}{{.}} {{end}}", "./...")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("go list 失败: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		pkg, imports, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		for _, imp := range strings.Fields(imports) {
			if strings.HasPrefix(imp, "[") {
				continue // go list -test 的测试变体引用（[pkg.test]），非真实 import
			}
			if imp == "github.com/cloudwego/eino" || strings.HasPrefix(imp, "github.com/cloudwego/eino/") {
				t.Errorf("业务禁直接依赖 eino：%s → %s（只应 import 基座契约面）", pkg, imp)
			}
		}
	}
}
