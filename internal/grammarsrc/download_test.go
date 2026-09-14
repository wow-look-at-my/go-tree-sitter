package grammarsrc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entry is a single member of a tarball the fake codeload serves.
type entry struct {
	name string
	kind byte
	body string
}

func tarball(test *testing.T, entries []entry) []byte {
	test.Helper()
	var buf bytes.Buffer
	zipper := gzip.NewWriter(&buf)
	archive := tar.NewWriter(zipper)
	for _, ent := range entries {
		hdr := &tar.Header{Name: ent.name, Typeflag: ent.kind, Mode: 0o644, Size: int64(len(ent.body))}
		switch ent.kind {
		case tar.TypeDir:
			hdr.Mode, hdr.Size = 0o755, 0
		case tar.TypeSymlink:
			hdr.Linkname, hdr.Size = ent.body, 0
		case tar.TypeXGlobalHeader:
			// git archive opens every tarball with the commit it came from.
			hdr = &tar.Header{Name: ent.name, Typeflag: ent.kind, PAXRecords: map[string]string{"comment": ent.body}}
		}
		require.NoError(test, archive.WriteHeader(hdr))
		if hdr.Size > 0 {
			_, err := archive.Write([]byte(ent.body))
			require.NoError(test, err)
		}
	}
	require.NoError(test, archive.Close())
	require.NoError(test, zipper.Close())
	return buf.Bytes()
}

// serve stands in for codeload for the rest of the test, answering a
// single repository at a single commit, and counts the requests it answers.
func serve(test *testing.T, repo, rev string, body []byte) *int {
	test.Helper()
	hits := new(int)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		*hits++
		if req.URL.Path != "/"+repo+"/tar.gz/"+rev {
			http.NotFound(writer, req)
			return
		}
		writer.Write(body)
	}))
	test.Cleanup(server.Close)
	saved := codeload
	codeload = server.URL
	test.Cleanup(func() { codeload = saved })
	return hits
}

func withoutGit(test *testing.T) {
	test.Setenv("PATH", test.TempDir())
}

func grammarTree(rev string) []entry {
	top := "tree-sitter-demo-" + rev + "/"
	return []entry{
		{name: "pax_global_header", kind: tar.TypeXGlobalHeader, body: rev},
		{name: top, kind: tar.TypeDir},
		{name: top + "src/", kind: tar.TypeDir},
		{name: top + "src/parser.c", kind: tar.TypeReg, body: "/* parser */\n"},
		{name: top + "test/corpus/basic.txt", kind: tar.TypeReg, body: "=====\ncase\n"},
	}
}

// A consumer's copy has no git work tree and no submodule contents, so the
// sources come from codeload at the pinned commit and land where the directive
// reads them.
func TestFetchDownloadsTheTreeWhereThereIsNoGit(test *testing.T) {
	withoutGit(test)
	const rev = "0123456789abcdef0123456789abcdef01234567"
	hits := serve(test, "tree-sitter/tree-sitter-demo", rev, tarball(test, grammarTree(rev)))
	dir := filepath.Join(test.TempDir(), "testdata", "tree-sitter-demo")

	require.NoError(test, Fetch("tree-sitter/tree-sitter-demo", rev, dir))

	parser, err := os.ReadFile(filepath.Join(dir, "src", "parser.c"))
	require.NoError(test, err)
	assert.Equal(test, "/* parser */\n", string(parser))
	corpus, err := os.ReadFile(filepath.Join(dir, "test", "corpus", "basic.txt"))
	require.NoError(test, err)
	assert.Equal(test, "=====\ncase\n", string(corpus))
	info, err := os.Stat(dir)
	require.NoError(test, err)
	assert.Equal(test, os.FileMode(0o755), info.Mode().Perm())
	assert.Equal(test, 1, *hits)

	siblings, err := os.ReadDir(filepath.Dir(dir))
	require.NoError(test, err)
	assert.Len(test, siblings, 1, "the staging directory is left behind")
}

// A submodule that was never initialized is an empty directory, which is what
// the download fills.
func TestFetchFillsAnEmptySubmoduleDirectory(test *testing.T) {
	withoutGit(test)
	const rev = "89abcdef0123456789abcdef0123456789abcdef"
	serve(test, "tree-sitter/tree-sitter-demo", rev, tarball(test, grammarTree(rev)))
	dir := filepath.Join(test.TempDir(), "tree-sitter-demo")
	require.NoError(test, os.Mkdir(dir, 0o755))

	require.NoError(test, Fetch("tree-sitter/tree-sitter-demo", rev, dir))

	assert.FileExists(test, filepath.Join(dir, "src", "parser.c"))
}

func TestFetchIsANoOpOnceTheTreeIsThere(test *testing.T) {
	withoutGit(test)
	hits := serve(test, "tree-sitter/tree-sitter-demo", "feed", nil)
	dir := test.TempDir()
	require.NoError(test, os.MkdirAll(filepath.Join(dir, "tsx", "src"), 0o755))
	require.NoError(test, os.WriteFile(filepath.Join(dir, "tsx", "src", "parser.c"), nil, 0o644))

	require.NoError(test, Fetch("tree-sitter/tree-sitter-demo", "feed", dir))

	assert.Zero(test, *hits)
}

// A failed request leaves nothing behind that the next run would take for the
// sources.
func TestFetchReportsACommitCodeloadDoesNotHave(test *testing.T) {
	withoutGit(test)
	serve(test, "tree-sitter/tree-sitter-demo", "feed", nil)
	dir := filepath.Join(test.TempDir(), "tree-sitter-demo")

	err := Fetch("tree-sitter/tree-sitter-demo", "beef", dir)

	require.Error(test, err)
	assert.Contains(test, err.Error(), "404")
	assert.NoDirExists(test, dir)
}

// An archive that cannot be unpacked whole is not unpacked at all.
func TestDownloadRefusesAnArchiveItCannotReproduce(test *testing.T) {
	for _, row := range []struct {
		name    string
		entries []entry
		want    string
	}{
		{
			name: "an entry that climbs out of the tree",
			entries: []entry{
				{name: "top/src/parser.c", kind: tar.TypeReg, body: "ok"},
				{name: "top/../../escape.c", kind: tar.TypeReg, body: "no"},
			},
			want: "refusing entry",
		},
		{
			name: "a symbolic link",
			entries: []entry{
				{name: "top/src/parser.c", kind: tar.TypeReg, body: "ok"},
				{name: "top/src/alias.c", kind: tar.TypeSymlink, body: "parser.c"},
			},
			want: "refusing entry",
		},
	} {
		test.Run(row.name, func(test *testing.T) {
			withoutGit(test)
			serve(test, "tree-sitter/tree-sitter-demo", "feed", tarball(test, row.entries))
			root := test.TempDir()
			dir := filepath.Join(root, "tree-sitter-demo")

			err := Fetch("tree-sitter/tree-sitter-demo", "feed", dir)

			require.Error(test, err)
			assert.Contains(test, err.Error(), row.want)
			assert.NoDirExists(test, dir)
			left, err := os.ReadDir(root)
			require.NoError(test, err)
			assert.Empty(test, left, "the staging directory is left behind")
		})
	}
}

func TestDownloadRefusesABodyThatIsNotGzip(test *testing.T) {
	withoutGit(test)
	serve(test, "tree-sitter/tree-sitter-demo", "feed", []byte("<html>rate limited</html>"))
	dir := filepath.Join(test.TempDir(), "tree-sitter-demo")

	err := Fetch("tree-sitter/tree-sitter-demo", "feed", dir)

	require.Error(test, err)
	assert.NoDirExists(test, dir)
}

// Files already in the directory are somebody's, and the download does not
// replace them.
func TestDownloadLeavesAPopulatedDirectoryAlone(test *testing.T) {
	const rev = "feed"
	serve(test, "tree-sitter/tree-sitter-demo", rev, tarball(test, grammarTree(rev)))
	dir := test.TempDir()
	require.NoError(test, os.WriteFile(filepath.Join(dir, "mine.txt"), []byte("keep"), 0o644))

	err := download("tree-sitter/tree-sitter-demo", rev, dir)

	require.Error(test, err)
	kept, err := os.ReadFile(filepath.Join(dir, "mine.txt"))
	require.NoError(test, err)
	assert.Equal(test, "keep", string(kept))
}
