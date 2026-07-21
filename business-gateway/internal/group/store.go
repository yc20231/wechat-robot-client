package group

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

type Type string

const (
	TypeCustomer Type = "customer"
	TypeAdmin    Type = "admin"
)

type Binding struct {
	GroupID      string `json:"group_id"`
	GroupName    string `json:"group_name"`
	Type         Type   `json:"type"`
	CustomerCode string `json:"customer_code,omitempty"`
	Enabled      bool   `json:"enabled"`
}

type bindingFile struct {
	Groups []Binding `json:"groups"`
}

type Store interface {
	Get(groupID string) (Binding, bool)
	List() []Binding
	Upsert(binding Binding) error
	Delete(groupID string) error
}

type FileStore struct {
	mu   sync.RWMutex
	path string
	data map[string]Binding
}

func NewFileStore(path string) (*FileStore, error) {
	store := &FileStore{path: path, data: make(map[string]Binding)}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取群绑定文件: %w", err)
	}
	var file bindingFile
	if err := json.Unmarshal(content, &file); err != nil {
		return nil, fmt.Errorf("解析群绑定文件: %w", err)
	}
	for _, binding := range file.Groups {
		binding = normalize(binding)
		if err := Validate(binding); err != nil {
			return nil, fmt.Errorf("群绑定 %q 无效: %w", binding.GroupID, err)
		}
		store.data[binding.GroupID] = binding
	}
	return store, nil
}

func (s *FileStore) Get(groupID string) (Binding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	binding, ok := s.data[strings.TrimSpace(groupID)]
	return binding, ok
}

func (s *FileStore) List() []Binding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Binding, 0, len(s.data))
	for _, binding := range s.data {
		result = append(result, binding)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].GroupID < result[j].GroupID })
	return result
}

func (s *FileStore) Upsert(binding Binding) error {
	binding = normalize(binding)
	if err := Validate(binding); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.data[binding.GroupID]
	s.data[binding.GroupID] = binding
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data[binding.GroupID] = previous
		} else {
			delete(s.data, binding.GroupID)
		}
		return err
	}
	return nil
}

func (s *FileStore) Delete(groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return errors.New("group_id 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.data[groupID]
	delete(s.data, groupID)
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data[groupID] = previous
		}
		return err
	}
	return nil
}

func (s *FileStore) persistLocked() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建群绑定目录: %w", err)
	}
	groups := make([]Binding, 0, len(s.data))
	for _, binding := range s.data {
		groups = append(groups, binding)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].GroupID < groups[j].GroupID })
	content, err := json.MarshalIndent(bindingFile{Groups: groups}, "", "  ")
	if err != nil {
		return fmt.Errorf("编码群绑定: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".groups-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时群绑定文件: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("设置群绑定文件权限: %w", err)
	}
	if _, err := tmp.Write(append(content, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("写入群绑定: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("同步群绑定: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭群绑定文件: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("替换群绑定文件: %w", err)
	}
	return nil
}

func Validate(binding Binding) error {
	if binding.GroupID == "" {
		return errors.New("group_id 不能为空")
	}
	switch binding.Type {
	case TypeCustomer:
		if binding.CustomerCode == "" {
			return errors.New("客户群必须设置 customer_code")
		}
	case TypeAdmin:
		if binding.CustomerCode != "" {
			return errors.New("管理员群不能绑定 customer_code")
		}
	default:
		return errors.New("type 必须是 customer 或 admin")
	}
	return nil
}

func normalize(binding Binding) Binding {
	binding.GroupID = strings.TrimSpace(binding.GroupID)
	binding.GroupName = strings.TrimSpace(binding.GroupName)
	binding.Type = Type(strings.ToLower(strings.TrimSpace(string(binding.Type))))
	binding.CustomerCode = strings.TrimSpace(binding.CustomerCode)
	return binding
}
