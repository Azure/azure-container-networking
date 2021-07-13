package platform

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
	"github.com/Azure/azure-container-networking/log"
)

var(
	ErrTimeoutLocking                 = fmt.Errorf("timed out locking file")
	ErrNonBlockingLockIsAlreadyLocked = fmt.Errorf("attempted to perform non-blocking lock on an already locked file")
)

const (
	// Extension added to the file name for lock.
	lockExtension = ".lock"
)

// ReadFileByLines reads file line by line and return array of lines.
func ReadFileByLines(filename string) ([]string, error) {
	var (
		lineStrArr []string
	)

	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("Error opening %s file error %v", filename, err)
	}

	defer f.Close()

	r := bufio.NewReader(f)

	for {
		lineStr, err := r.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return nil, fmt.Errorf("Error reading %s file error %v", filename, err)
			}

			lineStrArr = append(lineStrArr, lineStr)
			break
		}

		lineStrArr = append(lineStrArr, lineStr)
	}

	return lineStrArr, nil
}

func CheckIfFileExists(filepath string) (bool, error) {
	_, err := os.Stat(filepath)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return true, err
}

func CreateDirectory(dirPath string) error {
	if dirPath == "" {
		log.Printf("dirPath is empty, nothing to create.")
		return nil
	}

	isExist, err := CheckIfFileExists(dirPath)
	if err != nil {
		log.Printf("CheckIfFileExists returns err:%v", err)
		return err
	}

	if !isExist {
		err = os.Mkdir(dirPath, os.ModePerm)
	}

	return err
}

func InitLock() error {
	if CNILockPath == "" {
		return nil
	}

	err := os.MkdirAll(CNILockPath, os.FileMode(0664))
	if err != nil {
		log.Errorf("failed creating lock directory:%+v", err)
	}

	return err
}

func AcquireLockFile(name string, block bool) error {
	var lockFile *os.File
	var err error
	lockPerm := os.FileMode(0664) + os.FileMode(os.ModeExclusive)
	// Maximum number of retries before failing a lock call.
	lockMaxRetries := 200
	// Delay between lock retries.
	lockRetryDelay := 100 * time.Millisecond
	lockFilePath := CNILockPath + name + lockExtension
	// Try to acquire the lock file.
	var lockRetryCount int
	var modTimeCur time.Time
	var modTimePrev time.Time
	for lockRetryCount < lockMaxRetries {
		lockFile, err = os.OpenFile(lockFilePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, lockPerm)
		if err == nil {
			break
		}

		if !block {
			return ErrNonBlockingLockIsAlreadyLocked
		}

		// Reset the lock retry count if the timestamp for the lock file changes.
		if fileInfo, err := os.Stat(lockFilePath); err == nil {
			modTimeCur = fileInfo.ModTime()
			if !modTimeCur.Equal(modTimePrev) {
				lockRetryCount = 0
			}
			modTimePrev = modTimeCur
		}

		time.Sleep(lockRetryDelay)

		lockRetryCount++
	}

	if lockRetryCount == lockMaxRetries {
		return ErrTimeoutLocking
	}

	defer lockFile.Close()

	currentPid := os.Getpid()
	log.Printf("Write pid %d to lockfile %s", currentPid, lockFilePath)
	// Write the process ID for easy identification.
	if _, err = lockFile.WriteString(strconv.Itoa(currentPid)); err == nil {
		return nil
	}

	log.Errorf("Write to lockfile failed:%+v. Removing lock file", err)

	// remove lockfile
	return ReleaseLockFile(name)
}

func ReleaseLockFile(name string) error {
	var err error
	lockFilePath := CNILockPath + name + lockExtension
	if err = os.Remove(lockFilePath); err != nil {
		log.Errorf("Removing file %s failed with %+v", lockFilePath, err)
	}

	return err
}