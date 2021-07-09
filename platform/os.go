package platform

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
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

	if errors.Is(err, os.ErrNotExist) {
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

// Copy opens the two files specified and copies the contents of the
// source file in to the destination file.
func Copy(sourceFilename, destinationFilename string) error {
	src, err := os.Open(sourceFilename)
	if err != nil {
		return err
	}
	defer src.Close()

	dest, err := os.Open(destinationFilename)
	if err != nil {
		return err
	}
	defer dest.Close()

	if _, err := io.Copy(dest, src); err != nil {
		return err
	}
	return dest.Sync()
}
