package util

import (
	"fmt"
	"os"
	"path/filepath"
)

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ReadFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return string(data), nil
}

func WriteFileContent(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}
	return nil
}

func GetCgroupPath(components ...string) string {
	return filepath.Join(components...)
}

func ParseContainerID(fullID string) string {
	parts := []byte(fullID)

	for i := 0; i < len(parts)-2; i++ {
		if parts[i] == ':' && parts[i+1] == '/' && parts[i+2] == '/' {
			return string(parts[i+3:])
		}
	}

	return fullID
}

func GetPodIdentifier(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func GetContainerIdentifier(namespace, podName, containerName string) string {
	return fmt.Sprintf("%s/%s/%s", namespace, podName, containerName)
}

func SanitizePath(path string) string {
	replacements := map[rune]rune{
		'\\':   '/',
		'\x00': '_',
	}

	result := make([]rune, 0, len(path))
	for _, r := range path {
		if replacement, ok := replacements[r]; ok {
			result = append(result, replacement)
		} else {
			result = append(result, r)
		}
	}

	return string(result)
}
