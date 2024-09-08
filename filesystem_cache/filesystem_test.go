package filesystem_cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFilesystemCache(t *testing.T) {
	cachePath, _ := os.MkdirTemp("", "filesystem_cache_test")
	defer os.RemoveAll(cachePath)
	cache, err := NewFilesystemCache(cachePath)
	if err != nil {
		t.Errorf("Failed to create filesystem cache: %v", err)
	}

	// Verify that the cache path is set correctly
	if cache.path != cachePath {
		t.Errorf("Expected cache path %s, got %s", cachePath, cache.path)
	}

	// Verify that the cache directory is created
	_, err = os.Stat(cachePath)
	if os.IsNotExist(err) {
		t.Errorf("Cache directory %s does not exist", cachePath)
	}
}

func TestFilesystemCache_Get(t *testing.T) {
	cachePath, _ := os.MkdirTemp("", "filesystem_cache_test")
	defer os.RemoveAll(cachePath)
	cache, _ := NewFilesystemCache(cachePath)

	// Create a test file
	testKey := "test_key"
	testContent := "test_content"
	testFilePath := filepath.Join(cachePath, testKey)
	err := os.WriteFile(testFilePath, []byte(testContent), PERMISSIONS)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}

	// Retrieve the content from the cache
	content, ok := cache.Get(testKey)
	if !ok {
		t.Errorf("Expected cache hit for key %s, got cache miss", testKey)
	}

	// Verify that the retrieved content matches the test content
	if string(content) != testContent {
		t.Errorf("Expected content %s, got %s", testContent, string(content))
	}

	// Clean up the test file
	err = os.Remove(testFilePath)
	if err != nil {
		t.Errorf("Failed to remove test file: %v", err)
	}

	// Test that it'll create subdirectories if needed
	nestedDir := filepath.Join(cachePath, "nested")
	_, err = NewFilesystemCache(nestedDir)
	if err != nil {
		t.Errorf("Failed to create nested cache: %v", err)
	}
	if f, err := os.Stat(nestedDir); os.IsNotExist(err) || !f.IsDir() {
		t.Errorf("Expected nested cache directory to be created")
	}
}

func TestFilesystemCache_Set(t *testing.T) {
	cachePath, _ := os.MkdirTemp("", "filesystem_cache_test")
	defer os.RemoveAll(cachePath)
	cache, _ := NewFilesystemCache(cachePath)

	// Set a test key-value pair in the cache
	testKey := "test_key"
	testContent := "test_content"
	err := cache.Set(testKey, testContent, 0)
	if err != nil {
		t.Errorf("Failed to set key-value pair in cache: %v", err)
	}

	// Verify that the test file is created in the cache directory
	testFilePath := filepath.Join(cachePath, testKey)
	_, err = os.Stat(testFilePath)
	if os.IsNotExist(err) {
		t.Errorf("Test file %s does not exist in cache directory", testFilePath)
	}

	// Clean up the test file
	err = os.Remove(testFilePath)
	if err != nil {
		t.Errorf("Failed to remove test file: %v", err)
	}
}

func TestFilesystemCache_DeleteWithPrefix(t *testing.T) {
	cachePath, _ := os.MkdirTemp("", "filesystem_cache_test")
	defer os.RemoveAll(cachePath)
	cache, _ := NewFilesystemCache(cachePath)

	// Create test files with different prefixes
	testPrefix1 := "prefix1"
	testPrefix2 := "prefix2"
	testKey1 := testPrefix1 + "_key"
	testKey2 := testPrefix2 + "_key"
	testContent := "test_content"
	testFilePath1 := filepath.Join(cachePath, testKey1)
	testFilePath2 := filepath.Join(cachePath, testKey2)
	err := os.WriteFile(testFilePath1, []byte(testContent), PERMISSIONS)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}
	err = os.WriteFile(testFilePath2, []byte(testContent), PERMISSIONS)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}

	// Delete files with prefix "prefix1"
	err = cache.DeleteWithPrefix(testPrefix1)
	if err != nil {
		t.Errorf("Failed to delete files with prefix %s: %v", testPrefix1, err)
	}

	// Verify that the file with prefix "prefix1" is deleted
	_, err = os.Stat(testFilePath1)
	if !os.IsNotExist(err) {
		t.Errorf("Expected file %s to be deleted, but it still exists", testFilePath1)
	}

	// Verify that the file with prefix "prefix2" still exists
	_, err = os.Stat(testFilePath2)
	if os.IsNotExist(err) {
		t.Errorf("Expected file %s to exist, but it does not", testFilePath2)
	}

	// Clean up the remaining test file
	err = os.Remove(testFilePath2)
	if err != nil {
		t.Errorf("Failed to remove test file: %v", err)
	}
}

func TestFilesystemCache_Name(t *testing.T) {
	cachePath, _ := os.MkdirTemp("", "filesystem_cache_test")
	defer os.RemoveAll(cachePath)
	cache, _ := NewFilesystemCache(cachePath)

	// Verify that the cache name is returned correctly
	expectedName := "Filesystem"
	name := cache.Name()
	if name != expectedName {
		t.Errorf("Expected cache name %s, got %s", expectedName, name)
	}
}
