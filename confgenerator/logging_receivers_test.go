package confgenerator

import (
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// See https://man7.org/linux/man-pages/man2/statfs.2.html
const EXT4_SUPER_MAGIC = 0xef53

func TestDetectNFSPaths(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("This unit test only runs on Linux")
	}

	testCases := []struct {
		name            string
		includePaths    []string
		nfsPaths        []string
		shouldDetectNFS bool
	}{
		{
			name: "direct path",
			includePaths: []string{
				"/a/b/x.log",
			},
			nfsPaths: []string{
				"/a/b",
			},
			shouldDetectNFS: true,
		},
		{
			name: "wildcard file",
			includePaths: []string{
				"/a/b/*.log",
			},
			nfsPaths: []string{
				"/a/b",
			},
			shouldDetectNFS: true,
		},
		{
			name: "wildcard in folder",
			includePaths: []string{
				"/a/b/c*/*.log",
			},
			nfsPaths: []string{
				"/a/b",
			},
			shouldDetectNFS: true,
		},
		// If the NFS folder is part of a wildcard, we will not be able to
		// detect it.
		{
			name: "wildcard in NFS folder",
			includePaths: []string{
				"/a/*/*.log",
			},
			nfsPaths: []string{
				"/a/b",
			},
			shouldDetectNFS: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := LoggingReceiverFiles{
				IncludePaths: tc.includePaths,
			}
			m := r.mixin()
			m.syscallStatfs = pretendStatfs(tc.nfsPaths)
			nfsFound := m.detectNFSPaths()
			if nfsFound != tc.shouldDetectNFS {
				t.Fatalf("expected detectNFSPaths to return %t but returned %t", tc.shouldDetectNFS, nfsFound)
			}
		})
	}
}

func pretendStatfs(paths []string) statfsFunc {
	return func(path string, buf *syscall.Statfs_t) error {
		for _, testPath := range paths {
			if strings.HasPrefix(path, testPath) {
				buf.Type = NFS_SUPER_MAGIC
				return nil
			}
		}
		buf.Type = EXT4_SUPER_MAGIC
		return nil
	}
}
