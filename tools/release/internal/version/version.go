package version

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var (
	releaseBranchPattern = regexp.MustCompile(`^release/(v\d+\.\d+)$`)
	rootTagPattern       = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
)

var binaries = []string{
	"dropgz",
	"azure-ipam",
	"azure-iptables-monitor",
	"azure-ip-masq-merger",
	"cilium-log-collector",
}

type execRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type Service struct {
	run execRunner
}

type VersionInfo struct {
	LatestTag string `json:"latest_tag"`
	NewTag    string `json:"new_tag"`
	TargetSHA string `json:"target_sha"`
}

type BinaryChange struct {
	Binary      string `json:"binary"`
	NewTag      string `json:"new_tag"`
	PreviousTag string `json:"previous_tag"`
}

func NewService() *Service {
	return &Service{run: runCommand}
}

func (s *Service) NextVersion(ctx context.Context, branch string) (VersionInfo, error) {
	resolved, err := s.resolveRef(ctx, branch)
	if err != nil {
		return VersionInfo{}, err
	}

	latestTag, err := s.latestBranchTag(ctx, resolved, branch)
	if err != nil {
		return VersionInfo{}, err
	}

	newTag, err := bumpTag(latestTag)
	if err != nil {
		return VersionInfo{}, err
	}

	targetSHA, err := s.revParse(ctx, branch)
	if err != nil {
		return VersionInfo{}, err
	}

	return VersionInfo{
		LatestTag: latestTag,
		NewTag:    newTag,
		TargetSHA: targetSHA,
	}, nil
}

func (s *Service) DetectBinaryChanges(ctx context.Context, branch string) ([]BinaryChange, error) {
	resolved, err := s.resolveRef(ctx, branch)
	if err != nil {
		return nil, err
	}

	changes := make([]BinaryChange, 0, len(binaries))
	for _, binary := range binaries {
		latestTag, err := s.latestBinaryTag(ctx, resolved, binary)
		if err != nil {
			return nil, err
		}
		if latestTag == "" {
			continue
		}

		changed, err := s.hasBinaryChanges(ctx, latestTag, resolved, binary)
		if err != nil {
			return nil, err
		}
		if !changed {
			continue
		}

		newTag, err := bumpBinaryTag(binary, latestTag)
		if err != nil {
			return nil, err
		}

		changes = append(changes, BinaryChange{
			Binary:      binary,
			NewTag:      newTag,
			PreviousTag: latestTag,
		})
	}

	return changes, nil
}

func (s *Service) latestBranchTag(ctx context.Context, resolvedRef, branch string) (string, error) {
	if branch == "master" {
		return s.firstMatchingTag(ctx, resolvedRef, "v*", rootTagPattern)
	}

	match := releaseBranchPattern.FindStringSubmatch(branch)
	if len(match) != 2 {
		return "", fmt.Errorf("unsupported release branch %q", branch)
	}

	prefix := regexp.MustCompile("^" + regexp.QuoteMeta(match[1]) + `\.\d+$`)
	return s.firstMatchingTag(ctx, resolvedRef, match[1]+".*", prefix)
}

func (s *Service) latestBinaryTag(ctx context.Context, branch, binary string) (string, error) {
	pattern := regexp.MustCompile("^" + regexp.QuoteMeta(binary) + `/v\d+\.\d+\.\d+$`)
	out, err := s.run(ctx, "git", "tag", "--merged", branch, "-l", binary+"/v*", "--sort=-v:refname")
	if err != nil {
		return "", fmt.Errorf("listing tags for %s: %w", binary, err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" {
			continue
		}
		if pattern.MatchString(tag) {
			return tag, nil
		}
	}

	return "", nil
}

func (s *Service) firstMatchingTag(ctx context.Context, branch, glob string, pattern *regexp.Regexp) (string, error) {
	out, err := s.run(ctx, "git", "tag", "--merged", branch, "-l", glob, "--sort=-v:refname")
	if err != nil {
		return "", fmt.Errorf("listing tags for %s: %w", branch, err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" {
			continue
		}
		if pattern.MatchString(tag) {
			return tag, nil
		}
	}

	return "", fmt.Errorf("no matching tag found for %s", branch)
}

func (s *Service) hasBinaryChanges(ctx context.Context, fromTag, branch, binary string) (bool, error) {
	out, err := s.run(ctx, "git", "diff", "--name-only", fromTag+".."+branch, "--", binary+"/", "Makefile", "build/")
	if err != nil {
		return false, fmt.Errorf("checking changes for %s: %w", binary, err)
	}

	return strings.TrimSpace(string(out)) != "", nil
}

func (s *Service) revParse(ctx context.Context, ref string) (string, error) {
	out, err := s.run(ctx, "git", "rev-parse", ref)
	if err != nil {
		// In CI checkout, local branch may not exist; try origin/<ref>
		out, err = s.run(ctx, "git", "rev-parse", "origin/"+ref)
		if err != nil {
			return "", fmt.Errorf("resolving ref %s: %w", ref, err)
		}
	}

	return strings.TrimSpace(string(out)), nil
}

// resolveRef resolves a branch name to a SHA that works with git commands like --merged.
func (s *Service) resolveRef(ctx context.Context, ref string) (string, error) {
	return s.revParse(ctx, ref)
}

func bumpTag(tag string) (string, error) {
	parts, err := parseSemver(strings.TrimPrefix(tag, "v"))
	if err != nil {
		return "", fmt.Errorf("parsing tag %s: %w", tag, err)
	}

	return fmt.Sprintf("v%d.%d.%d", parts[0], parts[1], parts[2]+1), nil
}

func bumpBinaryTag(binary, tag string) (string, error) {
	prefix := binary + "/v"
	if !strings.HasPrefix(tag, prefix) {
		return "", fmt.Errorf("tag %s does not match binary %s", tag, binary)
	}

	parts, err := parseSemver(strings.TrimPrefix(tag, prefix))
	if err != nil {
		return "", fmt.Errorf("parsing binary tag %s: %w", tag, err)
	}

	return fmt.Sprintf("%s/v%d.%d.%d", binary, parts[0], parts[1], parts[2]+1), nil
}

func parseSemver(version string) ([3]int, error) {
	pieces := strings.Split(version, ".")
	if len(pieces) != 3 {
		return [3]int{}, fmt.Errorf("expected major.minor.patch, got %q", version)
	}

	var out [3]int
	for i, piece := range pieces {
		value, err := strconv.Atoi(piece)
		if err != nil {
			return [3]int{}, fmt.Errorf("parsing version part %q: %w", piece, err)
		}
		out[i] = value
	}

	return out, nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return out, nil
}
