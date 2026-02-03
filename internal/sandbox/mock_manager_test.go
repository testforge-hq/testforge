package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewMockManager(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := zap.NewNop()
		workDir := "/tmp/test-sandbox"

		manager := NewMockManager(workDir, nil, logger)

		require.NotNil(t, manager)
		assert.Equal(t, workDir, manager.workDir)
		assert.Equal(t, logger, manager.logger)
		assert.Nil(t, manager.storage)
	})

	t.Run("without logger creates development logger", func(t *testing.T) {
		workDir := "/tmp/test-sandbox"

		manager := NewMockManager(workDir, nil, nil)

		require.NotNil(t, manager)
		assert.NotNil(t, manager.logger)
		assert.Equal(t, workDir, manager.workDir)
	})
}

func TestMockManager_parseResultsFromOutput(t *testing.T) {
	logger := zap.NewNop()
	manager := NewMockManager("/tmp", nil, logger)

	tests := []struct {
		name           string
		output         string
		expectedPassed int
		expectedFailed int
		expectedSkip   int
	}{
		{
			name:           "all passed",
			output:         "Running 10 tests...\n10 passed (5.2s)\n",
			expectedPassed: 10,
			expectedFailed: 0,
			expectedSkip:   0,
		},
		{
			name:           "mixed results",
			output:         "Running tests...\n8 passed\n2 failed\n1 skipped (10.5s)\n",
			expectedPassed: 8,
			expectedFailed: 2,
			expectedSkip:   1,
		},
		{
			name:           "all failed",
			output:         "0 passed\n5 failed (2.1s)\n",
			expectedPassed: 0,
			expectedFailed: 5,
			expectedSkip:   0,
		},
		{
			name:           "empty output",
			output:         "",
			expectedPassed: 0,
			expectedFailed: 0,
			expectedSkip:   0,
		},
		{
			name:           "verbose output with results",
			output:         "PASS browser/chromium tests\n  ✓ login test (1.2s)\n  ✓ signup test (0.8s)\n\n3 passed (2.5s)\n",
			expectedPassed: 3,
			expectedFailed: 0,
			expectedSkip:   0,
		},
		{
			name:           "results with errors",
			output:         "FAIL tests\n  ✗ broken test\nError: element not found\n\n2 passed\n1 failed (5.0s)\n",
			expectedPassed: 2,
			expectedFailed: 1,
			expectedSkip:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := manager.parseResultsFromOutput(tt.output)

			require.NotNil(t, results)
			assert.Equal(t, tt.expectedPassed, results.Stats.Expected)
			assert.Equal(t, tt.expectedFailed, results.Stats.Unexpected)
			assert.Equal(t, tt.expectedSkip, results.Stats.Skipped)
		})
	}
}

func TestMockManager_Cleanup(t *testing.T) {
	// Create a temp directory structure
	tempDir := t.TempDir()
	runID := "test-run-123"
	runDir := filepath.Join(tempDir, runID)

	// Create directory with some files
	require.NoError(t, os.MkdirAll(filepath.Join(runDir, "scripts"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(runDir, "test.txt"), []byte("test"), 0644))

	logger := zap.NewNop()
	manager := NewMockManager(tempDir, nil, logger)

	// Verify directory exists
	_, err := os.Stat(runDir)
	require.NoError(t, err)

	// Cleanup
	err = manager.Cleanup(runID)
	require.NoError(t, err)

	// Verify directory is removed
	_, err = os.Stat(runDir)
	assert.True(t, os.IsNotExist(err))
}

func TestMockManager_Cleanup_NonExistent(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	manager := NewMockManager(tempDir, nil, logger)

	// Cleanup non-existent run should not error
	err := manager.Cleanup("non-existent-run")
	assert.NoError(t, err)
}

func TestMockManager_copyFile(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	manager := NewMockManager(tempDir, nil, logger)

	t.Run("copy regular file", func(t *testing.T) {
		srcFile := filepath.Join(tempDir, "source.txt")
		dstFile := filepath.Join(tempDir, "dest.txt")
		content := []byte("test content")

		require.NoError(t, os.WriteFile(srcFile, content, 0644))

		err := manager.copyFile(srcFile, dstFile)
		require.NoError(t, err)

		// Verify content matches
		dstContent, err := os.ReadFile(dstFile)
		require.NoError(t, err)
		assert.Equal(t, content, dstContent)
	})

	t.Run("copy symlink", func(t *testing.T) {
		srcFile := filepath.Join(tempDir, "target.txt")
		symlink := filepath.Join(tempDir, "link")
		dstLink := filepath.Join(tempDir, "link_copy")

		// Create target file
		require.NoError(t, os.WriteFile(srcFile, []byte("target"), 0644))

		// Create symlink
		require.NoError(t, os.Symlink(srcFile, symlink))

		// Copy symlink
		err := manager.copyFile(symlink, dstLink)
		require.NoError(t, err)

		// Verify it's still a symlink
		info, err := os.Lstat(dstLink)
		require.NoError(t, err)
		assert.True(t, info.Mode()&os.ModeSymlink != 0)
	})

	t.Run("copy non-existent file fails", func(t *testing.T) {
		err := manager.copyFile(filepath.Join(tempDir, "nonexistent"), filepath.Join(tempDir, "dest"))
		assert.Error(t, err)
	})
}

func TestMockManager_copyDir(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	manager := NewMockManager(tempDir, nil, logger)

	// Create source directory structure
	srcDir := filepath.Join(tempDir, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("file1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("file2"), 0644))

	// Create node_modules (should be skipped)
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "node_modules", "pkg"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "node_modules", "pkg", "index.js"), []byte("module"), 0644))

	// Create test-results (should be skipped)
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "test-results"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "test-results", "results.json"), []byte("{}"), 0644))

	dstDir := filepath.Join(tempDir, "dst")

	err := manager.copyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify files were copied
	assert.FileExists(t, filepath.Join(dstDir, "file1.txt"))
	assert.FileExists(t, filepath.Join(dstDir, "subdir", "file2.txt"))

	// Verify node_modules was skipped
	assert.NoDirExists(t, filepath.Join(dstDir, "node_modules"))

	// Verify test-results was skipped
	assert.NoDirExists(t, filepath.Join(dstDir, "test-results"))

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("file1"), content)
}

func TestMockManager_copyDir_SkipsPlaywrightReport(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	manager := NewMockManager(tempDir, nil, logger)

	srcDir := filepath.Join(tempDir, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "playwright-report"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "playwright-report", "index.html"), []byte("<html>"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "test.spec.ts"), []byte("test"), 0644))

	dstDir := filepath.Join(tempDir, "dst")

	err := manager.copyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify test file was copied
	assert.FileExists(t, filepath.Join(dstDir, "test.spec.ts"))

	// Verify playwright-report was skipped
	assert.NoDirExists(t, filepath.Join(dstDir, "playwright-report"))
}

func TestMockManager_downloadScripts_LocalPath(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	manager := NewMockManager(tempDir, nil, logger)

	// Create source directory with scripts
	srcDir := filepath.Join(tempDir, "source-scripts")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "test.spec.ts"), []byte("test()"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "package.json"), []byte("{}"), 0644))

	// Download (copy) to target
	targetDir := filepath.Join(tempDir, "target-scripts")

	err := manager.downloadScripts(nil, srcDir, targetDir)
	require.NoError(t, err)

	// Verify files were copied
	assert.FileExists(t, filepath.Join(targetDir, "test.spec.ts"))
	assert.FileExists(t, filepath.Join(targetDir, "package.json"))
}

func TestMockManager_downloadScripts_LocalPathNotFound(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	manager := NewMockManager(tempDir, nil, logger)

	targetDir := filepath.Join(tempDir, "target")
	err := manager.downloadScripts(nil, "/nonexistent/path", targetDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "local scripts path not found")
}

func TestMockManager_downloadScripts_MinIOWithoutStorage(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	manager := NewMockManager(tempDir, nil, logger)

	targetDir := filepath.Join(tempDir, "target")
	err := manager.downloadScripts(nil, "bucket/scripts.zip", targetDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storage not configured")
}

func TestMockManager_downloadScripts_RelativePath(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	manager := NewMockManager(tempDir, nil, logger)

	// Create source with relative path prefix
	srcDir := filepath.Join(tempDir, "rel-source")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "test.ts"), []byte("test"), 0644))

	// Change to tempDir so relative path works
	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	targetDir := filepath.Join(tempDir, "target")
	err := manager.downloadScripts(nil, "./rel-source", targetDir)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(targetDir, "test.ts"))
}
