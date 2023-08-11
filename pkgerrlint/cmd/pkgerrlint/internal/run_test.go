package internal_test

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/pkgerrlint/cmd/pkgerrlint/internal"
)

func TestRun(t *testing.T) {
	runTests, err := filepath.Glob("./testdata/*.go")
	if err != nil {
		t.Fatal("error loading test files: err:", err)
	}

	for _, testPath := range runTests {
		testPath := testPath

		t.Run(testPath, func(t *testing.T) {
			// similarly to example tests, each test file has, at its end, a set of
			// comments depicting the expected standard output when run on that file.
			// Example tests themselves can't be used because the Go source file is not
			// being executed.
			testFile, err := os.Open(testPath)
			if err != nil {
				t.Fatal("error opening test file: err:", err)
			}
			defer testFile.Close()

			sub, err := io.ReadAll(testFile)
			if err != nil {
				t.Fatal("error reading contents of test file: err:", err)
			}

			// extract the expected output
			scn := bufio.NewScanner(bytes.NewReader(sub))
			exp := []string{}
			scanningOutput := false // serves as scanner state for loading exp
			for scn.Scan() {
				line := scn.Text()

				// search for "Output" as a signifier that the expected output follows
				if strings.HasPrefix(line, "// Output:") {
					scanningOutput = true
					continue
				}

				const commentStart = "//"

				if scanningOutput {
					if strings.HasPrefix(line, commentStart) {
						next := strings.TrimPrefix(line, commentStart)
						next = strings.TrimLeft(next, " ") // remove leading spaces as well
						exp = append(exp, next)
					} else {
						// the end of comments signifies the end of the Output block
						scanningOutput = false
					}
				}
			}

			// we need a fake "standard output"
			stdout := bytes.NewBufferString("")
			err = internal.Run(stdout, testPath)
			if err != nil {
				t.Fatal("unexpected error: err:", err)
			}

			outLines := bytes.Split(stdout.Bytes(), []byte{'\n'})
			got := make([]string, 0, len(outLines))
			for _, line := range outLines {
				// trim empty newlines:
				if string(line) == "" {
					continue
				}
				got = append(got, string(line))
			}

			// ensure the output was as expected
			if len(got) != len(exp) {
				diff(t, exp, got)
			}

			for lineIdx := range got {
				gotLine := got[lineIdx]
				expLine := exp[lineIdx]

				if expLine != gotLine {
					diff(t, exp, got)
				}
			}
		})
	}
}

func diff(t *testing.T, exp, got []string) {
	t.Helper()
	t.Log("expected output differs from received output:")
	t.Logf("exp (len %d):\n", len(exp))

	for _, line := range exp {
		t.Log(line)
	}

	t.Logf("got (len %d):\n", len(got))

	for _, line := range got {
		t.Logf(line)
	}

	t.FailNow()
}
