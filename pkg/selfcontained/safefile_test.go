package selfcontained

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeRegularFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRegularFile_AcceptsRegularFile(t *testing.T) {
	root := t.TempDir()
	writeRegularFile(t, filepath.Join(root, "sub", "file.md"), "x")
	if err := CheckRegularFile(root, "sub/file.md"); err != nil {
		t.Fatalf("regular file: %v", err)
	}
}

func TestCheckRegularFile_AcceptsHardLinkedRegularFile(t *testing.T) {
	root := t.TempDir()
	writeRegularFile(t, filepath.Join(root, "a.md"), "x")
	if err := os.Link(filepath.Join(root, "a.md"), filepath.Join(root, "b.md")); err != nil {
		t.Skipf("hard link unsupported: %v", err)
	}
	if err := CheckRegularFile(root, "b.md"); err != nil {
		t.Fatalf("hard-linked regular file: %v", err)
	}
}

func TestCheckRegularFile_RejectsSymlinkToFileInsideRoot(t *testing.T) {
	root := t.TempDir()
	writeRegularFile(t, filepath.Join(root, "real.md"), "x")
	if err := os.Symlink(filepath.Join(root, "real.md"), filepath.Join(root, "link.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	assertCheckFails(t, root, "link.md", "symlink")
}

func TestCheckRegularFile_RejectsSymlinkToFileOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeRegularFile(t, filepath.Join(outside, "secret.md"), "x")
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "link.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	assertCheckFails(t, root, "link.md", "symlink")
}

func TestCheckRegularFile_RejectsSymlinkInParentDirectory(t *testing.T) {
	root := t.TempDir()
	realDir := t.TempDir()
	writeRegularFile(t, filepath.Join(realDir, "file.md"), "x")
	if err := os.Symlink(realDir, filepath.Join(root, "dirlink")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	assertCheckFails(t, root, "dirlink/file.md", "symlink")
}

func TestCheckRegularFile_RejectsDirectoryUsedAsFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	assertCheckFails(t, root, "adir", "regular file")
}

func TestCheckRegularFile_RejectsMissingFile(t *testing.T) {
	assertCheckFails(t, t.TempDir(), "no-such.md", "")
}

func TestCheckRegularFile_RejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../escape.md", "a/../../escape.md", "/abs.md"} {
		if err := CheckRegularFile(root, rel); err == nil {
			t.Errorf("CheckRegularFile(%q): expected error", rel)
		}
	}
}

func TestCheckRegularFile_RejectsFIFOWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	fifo := filepath.Join(root, "pipe.md")
	if err := mkfifo(fifo); err != nil {
		t.Skipf("fifo unsupported: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- CheckRegularFile(root, "pipe.md") }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("fifo: expected error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CheckRegularFile blocked on FIFO")
	}
}

func TestCheckRegularFile_RejectsSocket(t *testing.T) {
	root := t.TempDir()
	sock := filepath.Join(root, "sock.md")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix socket unsupported: %v", err)
	}
	defer func() { _ = ln.Close() }()
	assertCheckFails(t, root, "sock.md", "regular file")
}

func TestReadFileSafe_ReadsRegularFile(t *testing.T) {
	root := t.TempDir()
	writeRegularFile(t, filepath.Join(root, "file.md"), "content")
	data, err := ReadFileSafe(root, "file.md")
	if err != nil {
		t.Fatalf("ReadFileSafe: %v", err)
	}
	if string(data) != "content" {
		t.Fatalf("data = %q, want %q", data, "content")
	}
}

func TestReadFileSafe_RejectsFIFOWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	fifo := filepath.Join(root, "pipe.md")
	if err := mkfifo(fifo); err != nil {
		t.Skipf("fifo unsupported: %v", err)
	}
	done := make(chan struct{}, 1)
	var data []byte
	var err error
	go func() {
		data, err = ReadFileSafe(root, "pipe.md")
		done <- struct{}{}
	}()
	select {
	case <-done:
		if err == nil {
			t.Fatal("ReadFileSafe on FIFO: expected error")
		}
		if data != nil {
			t.Fatalf("data = %q, want nil", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadFileSafe blocked on FIFO")
	}
}

func TestDestRegistry(t *testing.T) {
	tests := []struct {
		name    string
		add     []string
		wantErr string
	}{
		{name: "distinct files ok", add: []string{"SKILL.md", "deps/skills/a/SKILL.md", "deps/prompts/b/PROMPT.md"}},
		{name: "same path in different dirs ok", add: []string{"a/SKILL.md", "b/SKILL.md"}},
		{name: "duplicate rejected", add: []string{"SKILL.md", "SKILL.md"}, wantErr: "duplicate destination"},
		{name: "file then dir collision", add: []string{"a", "a/b.md"}, wantErr: "collides with an existing file"},
		{name: "dir then file collision", add: []string{"a/b.md", "a"}, wantErr: "collides with an existing directory"},
		{name: "nested file then ancestor file", add: []string{"a/b/c.md", "a/b"}, wantErr: "collides with an existing directory"},
		{name: "parent escape rejected", add: []string{"../out.md"}, wantErr: "beneath the root"},
		{name: "absolute rejected", add: []string{"/out.md"}, wantErr: "absolute"},
		{name: "reserved prefix rejected", add: []string{".striatum/provenance.json"}, wantErr: "reserved"},
		{name: "reserved prefix nested rejected", add: []string{".striatum/x/y.json"}, wantErr: "reserved"},
		{name: "backslash rejected", add: []string{`a\b.md`}, wantErr: "backslashes"},
		{name: "empty-ish rejected", add: []string{"."}, wantErr: "beneath the root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewDestRegistry()
			var err error
			for _, d := range tt.add {
				if err = r.Add(d); err != nil {
					break
				}
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Add(%v): unexpected error %v", tt.add, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Add(%v): expected error containing %q", tt.add, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Add(%v) err = %q, want containing %q", tt.add, err, tt.wantErr)
			}
		})
	}
}

func assertCheckFails(t *testing.T, root, rel, wantContain string) {
	t.Helper()
	err := CheckRegularFile(root, rel)
	if err == nil {
		t.Fatalf("CheckRegularFile(%q): expected error", rel)
	}
	if wantContain != "" && !strings.Contains(err.Error(), wantContain) {
		t.Fatalf("CheckRegularFile(%q) err = %q, want containing %q", rel, err, wantContain)
	}
}
