package safetyreminder

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// LoadStaticPoster loads the approved poster for date when one is configured.
func LoadStaticPoster(date time.Time, directory string) ([]byte, bool, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, false, nil
	}

	path := filepath.Join(directory, date.Format("2006-01-02")+".png")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("检查静态安全海报 %s 失败: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("静态安全海报 %s 不是普通文件", path)
	}

	pngBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("读取静态安全海报 %s 失败: %w", path, err)
	}
	if !bytes.HasPrefix(pngBytes, pngSignature) {
		return nil, false, fmt.Errorf("静态安全海报 %s 不是有效 PNG 文件", path)
	}
	return pngBytes, true, nil
}
