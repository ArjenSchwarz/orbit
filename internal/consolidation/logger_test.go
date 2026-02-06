package consolidation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger_Append(t *testing.T) {
	t.Run("creates new log file on first append", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir)

		entry := LogEntry{
			Timestamp:             time.Now(),
			ChosenVariantID:       1,
			CommitSHA:             "abc123",
			Agent:                 "claude-code",
			ImprovementsAttempted: 3,
			ImprovementsApplied:   2,
			ImprovementsSkipped:   1,
			TestsPassed:           true,
			PostPromptPassed:      true,
		}

		err := logger.Append(entry)
		require.NoError(t, err)

		// Verify file was created
		data, err := os.ReadFile(filepath.Join(tmpDir, logFileName))
		require.NoError(t, err)

		var log ConsolidationLog
		err = json.Unmarshal(data, &log)
		require.NoError(t, err)

		assert.Equal(t, SchemaVersion, log.SchemaVersion)
		require.Len(t, log.Entries, 1)
		assert.Equal(t, entry.CommitSHA, log.Entries[0].CommitSHA)
		assert.Equal(t, entry.ChosenVariantID, log.Entries[0].ChosenVariantID)
		assert.Equal(t, SchemaVersion, log.Entries[0].SchemaVersion)
	})

	t.Run("appends to existing log file", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir)

		// Append first entry
		entry1 := LogEntry{
			Timestamp:       time.Now(),
			ChosenVariantID: 1,
			CommitSHA:       "first123",
			Agent:           "claude-code",
		}
		err := logger.Append(entry1)
		require.NoError(t, err)

		// Append second entry
		entry2 := LogEntry{
			Timestamp:       time.Now().Add(time.Hour),
			ChosenVariantID: 2,
			CommitSHA:       "second456",
			Agent:           "codex",
		}
		err = logger.Append(entry2)
		require.NoError(t, err)

		// Verify both entries exist
		log, err := logger.readLog()
		require.NoError(t, err)
		require.Len(t, log.Entries, 2)
		assert.Equal(t, "first123", log.Entries[0].CommitSHA)
		assert.Equal(t, "second456", log.Entries[1].CommitSHA)
	})

	t.Run("creates orbit directory if missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		orbitDir := filepath.Join(tmpDir, "nested", ".orbit")
		logger := NewLogger(orbitDir)

		entry := LogEntry{
			Timestamp: time.Now(),
			CommitSHA: "abc123",
		}

		err := logger.Append(entry)
		require.NoError(t, err)

		// Verify directory and file were created
		_, err = os.Stat(filepath.Join(orbitDir, logFileName))
		assert.NoError(t, err)
	})

	t.Run("sets schema version on entry", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir)

		entry := LogEntry{
			Timestamp: time.Now(),
			CommitSHA: "abc123",
			// SchemaVersion not set
		}

		err := logger.Append(entry)
		require.NoError(t, err)

		log, err := logger.readLog()
		require.NoError(t, err)
		assert.Equal(t, SchemaVersion, log.Entries[0].SchemaVersion)
	})
}

func TestLogger_ConcurrentAppend(t *testing.T) {
	tmpDir := t.TempDir()
	logger := NewLogger(tmpDir)

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			entry := LogEntry{
				Timestamp:       time.Now(),
				ChosenVariantID: id,
				CommitSHA:       "concurrent",
				Agent:           "claude-code",
			}
			err := logger.Append(entry)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all entries were written without corruption
	log, err := logger.readLog()
	require.NoError(t, err)
	assert.Len(t, log.Entries, numGoroutines)

	// Verify JSON is valid
	data, err := os.ReadFile(filepath.Join(tmpDir, logFileName))
	require.NoError(t, err)
	var verifyLog ConsolidationLog
	err = json.Unmarshal(data, &verifyLog)
	require.NoError(t, err, "Log file should be valid JSON after concurrent writes")
}

func TestLogger_SaveReport(t *testing.T) {
	t.Run("saves report with timestamp in filename", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir)

		report := "## Consolidation Report\n\nTest content"
		filename, err := logger.SaveReport(report)
		require.NoError(t, err)

		assert.Contains(t, filename, "consolidation-report-")
		assert.Contains(t, filename, ".md")

		// Verify content was written
		data, err := os.ReadFile(filepath.Join(tmpDir, filename))
		require.NoError(t, err)
		assert.Equal(t, report, string(data))
	})

	t.Run("creates orbit directory if missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		orbitDir := filepath.Join(tmpDir, "nested", ".orbit")
		logger := NewLogger(orbitDir)

		filename, err := logger.SaveReport("test")
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(orbitDir, filename))
		assert.NoError(t, err)
	})
}

func TestLogger_GetLatestCommitSHA(t *testing.T) {
	t.Run("returns commit SHA from most recent entry", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir)

		// Add multiple entries
		_ = logger.Append(LogEntry{Timestamp: time.Now(), CommitSHA: "first"})
		_ = logger.Append(LogEntry{Timestamp: time.Now(), CommitSHA: "second"})
		_ = logger.Append(LogEntry{Timestamp: time.Now(), CommitSHA: "third"})

		sha, err := logger.GetLatestCommitSHA()
		require.NoError(t, err)
		assert.Equal(t, "third", sha)
	})

	t.Run("returns error when no entries exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir)

		// Create empty log file
		emptyLog := ConsolidationLog{
			SchemaVersion: SchemaVersion,
			Entries:       []LogEntry{},
		}
		data, _ := json.Marshal(emptyLog)
		_ = os.WriteFile(filepath.Join(tmpDir, logFileName), data, 0644)

		_, err := logger.GetLatestCommitSHA()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no consolidation entries found")
	})

	t.Run("returns error when log file does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir)

		_, err := logger.GetLatestCommitSHA()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read consolidation log")
	})

	t.Run("returns error when latest entry has no commit SHA", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir)

		_ = logger.Append(LogEntry{Timestamp: time.Now(), CommitSHA: ""})

		_, err := logger.GetLatestCommitSHA()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no commit SHA")
	})
}

func TestLogger_AtomicWrite(t *testing.T) {
	t.Run("no temp files remain after successful write", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir)

		_ = logger.Append(LogEntry{Timestamp: time.Now(), CommitSHA: "test"})

		// Check for temp files
		files, err := os.ReadDir(tmpDir)
		require.NoError(t, err)

		for _, f := range files {
			assert.NotContains(t, f.Name(), ".tmp", "No temp files should remain")
		}
	})
}
