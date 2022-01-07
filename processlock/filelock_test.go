package processlock

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	lockDir, _       = os.Getwd()
	existingLockFile string
	newLockFile      string
)

func TestMain(m *testing.M) {
	existingLockFile = filepath.Join(lockDir, "existing.lock")
	newLockFile = filepath.Join(lockDir, "new.lock")
	os.Remove(existingLockFile)
	os.Remove(newLockFile)
	f, _ := os.Create(existingLockFile)
	exitCode := m.Run()
	f.Close()
	os.Remove(existingLockFile)
	os.Remove(newLockFile)
	os.Exit(exitCode)
}

func TestFileLock(t *testing.T) {
	tests := []struct {
		name           string
		flock          Interface
		wantErr        bool
		deleteLockfile bool
		wantErrMsg     string
		lockfileName   string
	}{
		{
			name:           "Create new file and acquire Lock",
			flock:          &fileLock{filePath: newLockFile},
			wantErr:        false,
			deleteLockfile: true,
			lockfileName:   newLockFile,
		},
		{
			name:         "acquire Lock on existing file",
			flock:        &fileLock{filePath: existingLockFile},
			lockfileName: existingLockFile,
			wantErr:      false,
		},
		{
			name:         "acquire Lock on existing file after releasing",
			flock:        &fileLock{filePath: existingLockFile},
			lockfileName: existingLockFile,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flock.Lock()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				err = tt.flock.Unlock()
				require.NoError(t, err)
				err = tt.flock.Unlock()
				require.NoError(t, err, "Calling Release lock again should not throw error for already released lock:%v", err)

				// read lockfile contents to check if contents match with pid of current process
				b, errRead := os.ReadFile(tt.lockfileName)
				require.NoError(t, errRead, "Got error reading lockfile:%v", errRead)
				pidStr := string(b)
				pid, _ := strconv.Atoi(pidStr)
				require.Equal(t, os.Getpid(), pid, "Expected pid %d but got %d", os.Getpid(), pid)
			}
			if tt.deleteLockfile {
				os.Remove(tt.lockfileName)
			}
		})
	}
}

func TestReleaseFileLockError(t *testing.T) {
	tests := []struct {
		name       string
		flock      Interface
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "Release file lock without acquring it",
			flock:      &fileLock{filePath: newLockFile},
			wantErr:    true,
			wantErrMsg: ErrInvalidFile.Error(),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flock.Unlock()
			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, tt.wantErrMsg, err.Error(), "Expected:%s but got:%s", tt.wantErrMsg, err.Error())
			}
		})
	}
}
