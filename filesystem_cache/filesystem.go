package filesystem_cache

import (
	"fmt"
	"os"
	"path"
)

const PERMISSIONS = 0644

type FilesystemCache struct {
	path string
}

func NewFilesystemCache(path string) (*FilesystemCache, error) {
	f, err := os.Stat(path)
	if os.IsNotExist(err) {
		// if the path does not exist, we can create it
		err := os.MkdirAll(path, 0755)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %v", path, err)
		}
	} else if !f.Mode().IsDir() {
		return nil, fmt.Errorf("path %s is not a directory", path)
	}
	return &FilesystemCache{path}, nil
}

func (c *FilesystemCache) Get(key string) ([]byte, bool) {
	// Read the content of the file with the given key
	// If the file does not exist, return false
	// If the file exists, return the content as a byte slice
	content, err := os.ReadFile(fmt.Sprintf("%v/%v", c.path, key))
	if err != nil {
		return nil, false
	}
	return content, true
}

func (c *FilesystemCache) Set(key string, content string, duration int) error {
	// Write the content to a file with the given key
	// duration is not used in this implementation as pruning is not implemented
	cachePath := fmt.Sprintf("%v/%v", c.path, key)
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		dir := path.Dir(cachePath)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}
	return os.WriteFile(cachePath, []byte(content), PERMISSIONS)
}

func (c *FilesystemCache) DeleteWithPrefix(prefix string) error {
	// Delete all files with the given prefix from the cache.
	// We can use the filepath.Glob function to get all files with the given prefix
	files, err := os.ReadDir(c.path)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", c.path, err)
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !file.Type().IsRegular() {
			continue
		}

		if file.Name()[:len(prefix)] == prefix {
			err := os.Remove(fmt.Sprintf("%v/%v", c.path, file.Name()))
			if err != nil {
				return fmt.Errorf("failed to delete file %s: %v", file.Name(), err)
			}
		}
	}

	return nil
}

func (c *FilesystemCache) Name() string {
	return "Filesystem"
}
