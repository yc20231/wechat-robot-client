package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Role string

const (
	RoleOwner Role = "owner"
	RoleRoot  Role = "root"
	RoleAdmin Role = "admin"
)

type Entry struct {
	WxID  string `json:"wxid"`
	Role  Role   `json:"role"`
	Fixed bool   `json:"fixed,omitempty"`
}

type bindingFile struct {
	Admins []Entry `json:"admins"`
}

type Store interface {
	RoleOf(wxID string) (Role, bool)
	IsOwner(wxID string) bool
	IsRoot(wxID string) bool
	IsAdmin(wxID string) bool
	List() []Entry
	SetRole(wxID string, role Role) error
	DemoteRoot(wxID string) error
	Delete(wxID string) error
}

type FileStore struct {
	mu     sync.RWMutex
	path   string
	owners map[string]struct{}
	data   map[string]Role
}

func NewFileStore(path string, owners map[string]struct{}) (*FileStore, error) {
	store := &FileStore{
		path:   path,
		owners: cloneSet(owners),
		data:   make(map[string]Role),
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取管理员文件: %w", err)
	}
	var file bindingFile
	if err := json.Unmarshal(content, &file); err != nil {
		return nil, fmt.Errorf("解析管理员文件: %w", err)
	}
	for _, entry := range file.Admins {
		entry.WxID = strings.TrimSpace(entry.WxID)
		if entry.WxID == "" || (entry.Role != RoleRoot && entry.Role != RoleAdmin) {
			return nil, fmt.Errorf("管理员记录无效: wxid=%q role=%q", entry.WxID, entry.Role)
		}
		if _, fixed := store.owners[entry.WxID]; fixed {
			continue
		}
		store.data[entry.WxID] = entry.Role
	}
	return store, nil
}

func (s *FileStore) RoleOf(wxID string) (Role, bool) {
	wxID = strings.TrimSpace(wxID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.owners[wxID]; ok {
		return RoleOwner, true
	}
	role, ok := s.data[wxID]
	return role, ok
}

func (s *FileStore) IsOwner(wxID string) bool {
	role, ok := s.RoleOf(wxID)
	return ok && role == RoleOwner
}

func (s *FileStore) IsRoot(wxID string) bool {
	role, ok := s.RoleOf(wxID)
	return ok && (role == RoleOwner || role == RoleRoot)
}

func (s *FileStore) IsAdmin(wxID string) bool {
	_, ok := s.RoleOf(wxID)
	return ok
}

func (s *FileStore) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Entry, 0, len(s.owners)+len(s.data))
	for wxID := range s.owners {
		result = append(result, Entry{WxID: wxID, Role: RoleOwner, Fixed: true})
	}
	for wxID, role := range s.data {
		result = append(result, Entry{WxID: wxID, Role: role})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Role != result[j].Role {
			return result[i].Role < result[j].Role
		}
		return result[i].WxID < result[j].WxID
	})
	return result
}

func (s *FileStore) SetRole(wxID string, role Role) error {
	wxID = strings.TrimSpace(wxID)
	if wxID == "" {
		return errors.New("wxid 不能为空")
	}
	if role != RoleRoot && role != RoleAdmin {
		return errors.New("动态管理员角色必须是 root 或 admin")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, fixed := s.owners[wxID]; fixed {
		return errors.New("固定所有者不可修改")
	}
	previous, existed := s.data[wxID]
	s.data[wxID] = role
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data[wxID] = previous
		} else {
			delete(s.data, wxID)
		}
		return err
	}
	return nil
}

func (s *FileStore) DemoteRoot(wxID string) error {
	wxID = strings.TrimSpace(wxID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, fixed := s.owners[wxID]; fixed {
		return errors.New("固定所有者不可移除或降级")
	}
	role, ok := s.data[wxID]
	if !ok || role != RoleRoot {
		return errors.New("目标不是动态根管理员")
	}
	s.data[wxID] = RoleAdmin
	if err := s.persistLocked(); err != nil {
		s.data[wxID] = role
		return err
	}
	return nil
}

func (s *FileStore) Delete(wxID string) error {
	wxID = strings.TrimSpace(wxID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, fixed := s.owners[wxID]; fixed {
		return errors.New("固定所有者不可移除或降级")
	}
	previous, existed := s.data[wxID]
	if !existed {
		return errors.New("目标不是动态管理员")
	}
	delete(s.data, wxID)
	if err := s.persistLocked(); err != nil {
		s.data[wxID] = previous
		return err
	}
	return nil
}

func (s *FileStore) persistLocked() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建管理员目录: %w", err)
	}
	entries := make([]Entry, 0, len(s.data))
	for wxID, role := range s.data {
		entries = append(entries, Entry{WxID: wxID, Role: role})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].WxID < entries[j].WxID })
	content, err := json.MarshalIndent(bindingFile{Admins: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("编码管理员文件: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".admins-*.tmp")
	if err != nil {
		return fmt.Errorf("创建管理员临时文件: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("设置管理员文件权限: %w", err)
	}
	if _, err := tmp.Write(append(content, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("写入管理员文件: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("同步管理员文件: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭管理员文件: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("替换管理员文件: %w", err)
	}
	return nil
}

func cloneSet(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source))
	for value := range source {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}
