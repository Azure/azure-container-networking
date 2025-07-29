package main

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const (
	errInvalidConfig = "expected error for invalid config file, got nil"
	errUnexpected    = "unexpected error: %v"
)

type mockFile struct {
	data []byte
	mode fs.FileMode
}

type mockFS struct {
	files map[string]mockFile
	dirs  map[string][]string // directory to file names
}

func newMockFS() *mockFS {
	return &mockFS{
		files: make(map[string]mockFile),
		dirs:  make(map[string][]string),
	}
}

func (m *mockFS) ReadFile(path string) ([]byte, error) {
	f, ok := m.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return f.data, nil
}

// WriteFile creates a file and directory entry for our mock
func (m *mockFS) WriteFile(path string, data []byte, perm fs.FileMode) error {
	m.files[path] = mockFile{data: data, mode: perm}
	dir := filepath.Dir(path)
	m.dirs[dir] = append(m.dirs[dir], filepath.Base(path))
	return nil
}

func (m *mockFS) ReadDir(dirname string) ([]fs.DirEntry, error) {
	entries := []fs.DirEntry{}
	for _, fname := range m.dirs[dirname] {
		entries = append(entries, mockDirEntry{name: fname})
	}
	return entries, nil
}

func (m *mockFS) DeleteFile(path string) error {
	if _, ok := m.files[path]; !ok {
		return fs.ErrNotExist
	}
	delete(m.files, path)
	return nil
}

type mockDirEntry struct{ name string }

func (m mockDirEntry) Name() string               { return m.name }
func (m mockDirEntry) IsDir() bool                { return false }
func (m mockDirEntry) Type() fs.FileMode          { return 0 }
func (m mockDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestMergeConfig(t *testing.T) {
	type file struct {
		name string
		data string
	}
	type want struct {
		config     *MasqConfig
		expectFile bool
		expectErr  bool
	}
	tests := []struct {
		name  string
		files []file
		want  want
	}{
		{
			name:  "no config files",
			files: nil,
			want: want{
				config:     nil,
				expectFile: false,
				expectErr:  false,
			},
		},
		{
			name: "one valid config",
			files: []file{
				{
					name: "ip-masq-foo.yaml",
					data: `{"nonMasqueradeCIDRs":["10.0.0.0/8"],"masqLinkLocal":true,"masqLinkLocalIPv6":false}`,
				},
			},
			want: want{
				config: &MasqConfig{
					NonMasqueradeCIDRs: []string{"10.0.0.0/8"},
					MasqLinkLocal:      true,
					MasqLinkLocalIPv6:  false,
				},
				expectFile: true,
				expectErr:  false,
			},
		},
		{
			name: "two valid configs merged",
			files: []file{
				{
					name: "ip-masq-a.yaml",
					data: `{"nonMasqueradeCIDRs":["10.0.0.0/8"],"masqLinkLocal":false,"masqLinkLocalIPv6":true}`,
				},
				{
					name: "ip-masq-b.yaml",
					data: `{"nonMasqueradeCIDRs":["192.168.0.0/16"],"masqLinkLocal":true,"masqLinkLocalIPv6":false}`,
				},
			},
			want: want{
				config: &MasqConfig{
					NonMasqueradeCIDRs: []string{"10.0.0.0/8", "192.168.0.0/16"},
					MasqLinkLocal:      true,
					MasqLinkLocalIPv6:  true,
				},
				expectFile: true,
				expectErr:  false,
			},
		},
		{
			name: "two valid configs merged yaml",
			files: []file{
				{
					name: "ip-masq-a.yaml",
					data: `nonMasqueradeCIDRs: ["10.0.0.0/8"]
masqLinkLocal: false
masqLinkLocalIPv6: true`,
				},
				{
					name: "ip-masq-b.yaml",
					data: `nonMasqueradeCIDRs: ["192.168.0.0/16"]
masqLinkLocal: true
masqLinkLocalIPv6: false`,
				},
			},
			want: want{
				config: &MasqConfig{
					NonMasqueradeCIDRs: []string{"10.0.0.0/8", "192.168.0.0/16"},
					MasqLinkLocal:      true,
					MasqLinkLocalIPv6:  true,
				},
				expectFile: true,
				expectErr:  false,
			},
		},
		{
			name: "invalid config file",
			files: []file{
				{
					name: "ip-masq-bad.yaml",
					data: "not valid yaml",
				},
			},
			want: want{
				config:     nil,
				expectFile: false,
				expectErr:  true,
			},
		},
		{
			name: "valid and invalid config files",
			files: []file{
				{
					name: "ip-masq-good.yaml",
					data: `{"nonMasqueradeCIDRs":["10.0.0.0/8"],"masqLinkLocal":true,"masqLinkLocalIPv6":false}`,
				},
				{
					name: "ip-masq-bad.yaml",
					data: "not valid yaml",
				},
			},
			want: want{
				config:     nil,
				expectFile: false,
				expectErr:  true,
			},
		},
		{
			name: "misaligned cidr invalid config file",
			files: []file{
				{
					name: "ip-masq-bad.yaml",
					data: `{"nonMasqueradeCIDRs":["10.0.0.4/8"],"masqLinkLocal":true,"masqLinkLocalIPv6":false}`,
				},
			},
			want: want{
				config:     nil,
				expectFile: false,
				expectErr:  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := newMockFS()
			var configFiles []string
			for _, f := range tt.files {
				full := filepath.Join(*configPath, f.name)
				fs.files[full] = mockFile{data: []byte(f.data)}
				configFiles = append(configFiles, f.name)
			}
			fs.dirs[*configPath] = configFiles

			daemon := &MasqDaemon{}
			err := daemon.mergeConfig(fs)
			if tt.want.expectErr {
				require.Error(t, err, errInvalidConfig)
				return
			}
			require.NoError(t, err, errUnexpected, err)

			mergedPath := filepath.Join(*outputPath, "ip-masq-agent")
			mergedFile, ok := fs.files[mergedPath]
			if tt.want.expectFile {
				require.True(t, ok, "expected merged config file at %q", mergedPath)
				var got MasqConfig
				require.NoError(t, yaml.Unmarshal(mergedFile.data, &got))
				require.True(t, cidrSetEqual(got.NonMasqueradeCIDRs, tt.want.config.NonMasqueradeCIDRs), "unexpected merged CIDRs: got %v, want %v", got.NonMasqueradeCIDRs, tt.want.config.NonMasqueradeCIDRs)
				require.Equal(t, tt.want.config.MasqLinkLocal, got.MasqLinkLocal, "unexpected MasqLinkLocal")
				require.Equal(t, tt.want.config.MasqLinkLocalIPv6, got.MasqLinkLocalIPv6, "unexpected MasqLinkLocalIPv6")
			} else {
				require.False(t, ok, "expected no merged config file, but found one")
			}
		})
	}
}

// cidrSetEqual checks if the two string slices have the same elements regardless of ordering
func cidrSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := set[v]; !ok {
			return false
		}
	}
	return true
}

func TestMergeCIDRs(t *testing.T) {
	a := []string{"10.0.0.0/8", "192.168.1.0/24"}
	b := []string{"192.168.1.0/24", "172.16.0.0/12"}
	got := mergeCIDRs(a, b)
	want := map[string]struct{}{
		"10.0.0.0/8":     {},
		"192.168.1.0/24": {},
		"172.16.0.0/12":  {},
	}
	require.Equal(t, len(want), len(got), "expected %d, got %d", len(want), len(got))
	for _, cidr := range got {
		_, ok := want[cidr]
		require.True(t, ok, "unexpected CIDR: %s", cidr)
	}
}

func TestValidateCIDR(t *testing.T) {
	valid := []string{"10.0.0.0/8", "192.168.1.0/24", "2001:db8::/32"}
	for _, cidr := range valid {
		require.NoError(t, validateCIDR(cidr), "expected valid for %q", cidr)
	}
	invalid := []string{"10.0.0.1/8", "notacidr", "10.0.0.0/33"}
	for _, cidr := range invalid {
		require.Error(t, validateCIDR(cidr), "expected error for %q", cidr)
	}
}

func TestMergeConfigAddAndRemove(t *testing.T) {
	fs := newMockFS()
	cfgA := `{"nonMasqueradeCIDRs":["10.0.0.0/8"],"masqLinkLocal":false,"masqLinkLocalIPv6":true}`
	cfgB := `{"nonMasqueradeCIDRs":["192.168.0.0/16"],"masqLinkLocal":true,"masqLinkLocalIPv6":false}`
	fs.files[filepath.Join(*configPath, "ip-masq-a.yaml")] = mockFile{data: []byte(cfgA)}
	fs.files[filepath.Join(*configPath, "ip-masq-b.yaml")] = mockFile{data: []byte(cfgB)}
	fs.dirs[*configPath] = []string{"ip-masq-a.yaml", "ip-masq-b.yaml"}

	daemon := &MasqDaemon{}
	// merge with both configs present
	err := daemon.mergeConfig(fs)
	require.NoError(t, err)
	mergedPath := filepath.Join(*outputPath, "ip-masq-agent")
	mergedFile, ok := fs.files[mergedPath]
	require.True(t, ok, "expected merged config file at %q", mergedPath)
	var got MasqConfig
	require.NoError(t, yaml.Unmarshal(mergedFile.data, &got))
	require.True(t, cidrSetEqual(got.NonMasqueradeCIDRs, []string{"10.0.0.0/8", "192.168.0.0/16"}), "unexpected merged CIDRs: %v", got.NonMasqueradeCIDRs)
	require.True(t, got.MasqLinkLocal, "expected MasqLinkLocal true")
	require.True(t, got.MasqLinkLocalIPv6, "expected MasqLinkLocalIPv6 true")

	// remove both config files
	delete(fs.files, filepath.Join(*configPath, "ip-masq-a.yaml"))
	delete(fs.files, filepath.Join(*configPath, "ip-masq-b.yaml"))
	fs.dirs[*configPath] = []string{}

	// merge again, should remove merged config file
	err = daemon.mergeConfig(fs)
	require.NoError(t, err)
	_, ok = fs.files[mergedPath]
	require.False(t, ok, "expected merged config file to be deleted, but it still exists")
}
