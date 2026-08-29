// Package appsupport 是示例共享的应用侧底座：会话域文件存储（session.Store
// 契约的演示实现）与事件打印器（示例的「渲染层」）。生产应用按自己的数据面
// 实现同契约——注意 session.Store 的 UserTreeDir 须为本地文件系统路径
// （recall 检索与工作区清理直用 os 操作，文件存储是基座唯一支持的形态）。
package appsupport

import (
	"os"
	"path/filepath"

	"github.com/jumeng/einox/checkpoint"
	"github.com/jumeng/einox/session"
)

// Store 文件版会话存储：<root>/users/<owner>/ 布局。会话记录（sessions/
// <sid>/session.json）、检查点（sessions/<sid>/checkpoints/）与工作区
// （workspaces/<sid>）都落在用户域子树下——与基座清理链（Registry.Delete、
// Sweeper）使用的路径约定一致。
type Store struct {
	root string
}

// NewStore 构造（root 不存在即建）。
func NewStore(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: abs}, nil
}

// 编译期钉契约：存储须同时满足会话域与检查点两道存储面。
var (
	_ session.Store        = (*Store)(nil)
	_ checkpoint.UserStore = (*Store)(nil)
)

func (s *Store) ReadUserTreeFile(operator, rel string) ([]byte, bool) {
	data, err := os.ReadFile(filepath.Join(s.userDir(operator), filepath.FromSlash(rel)))
	if err != nil {
		return nil, false
	}
	return data, true
}

func (s *Store) WriteUserTreeFile(operator, rel string, data []byte) error {
	path := filepath.Join(s.userDir(operator), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) RemoveUserTree(operator, rel string) error {
	return os.RemoveAll(filepath.Join(s.userDir(operator), filepath.FromSlash(rel)))
}

func (s *Store) UserTreeDir(operator string) string { return s.userDir(operator) }

func (s *Store) ListUserTreeSessions(operator string) []string {
	return listDir(s.userDir(operator), "sessions")
}

func (s *Store) ListUsers() []string { return listDir(s.root, "users") }

func (s *Store) TmpDir() string { return filepath.Join(s.root, ".tmp") }

func (s *Store) Dir() string { return s.root }

func (s *Store) userDir(operator string) string { return filepath.Join(s.root, "users", operator) }

func listDir(base, name string) []string {
	entries, err := os.ReadDir(filepath.Join(base, name))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}
