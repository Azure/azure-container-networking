package hash

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
)

type Checksums map[string]string

func Parse(r io.Reader) (Checksums, error) {
	checksums := Checksums{}
	linescanner := bufio.NewScanner(r)
	linescanner.Split(bufio.ScanLines)

	for linescanner.Scan() {
		line := linescanner.Text()
		entry := strings.Fields(line)
		if len(entry) != 2 { //nolint:gomnd // sha256 checksum file constant
			return nil, errors.Errorf("malformed sha checksum line: %s", line)
		}
		checksums[entry[1]] = entry[0]
	}
	return checksums, nil
}

func (sums Checksums) Check(src, dst string) (bool, error) {
	want, ok := sums[src]
	if !ok {
		return false, errors.Errorf("unknown path %s", src)
	}
	buf, err := os.ReadFile(dst)
	if err != nil {
		return false, errors.Wrapf(err, "unable to read file %s", dst)
	}
	have := sha256.Sum256(buf)
	return want == fmt.Sprintf("%x", have), nil
}
