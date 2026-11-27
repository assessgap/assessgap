package e2e

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/assessgap/assessgap/internal/fixture"
)

// bin is the binary built once for the package, in a temporary directory.
var bin string

// repoRoot is the module root, two levels up from this file.
var repoRoot = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}()

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "assessgap-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	defer os.RemoveAll(dir)
	bin = filepath.Join(dir, "assessgap")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/assessgap")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: go build: %v\n%s", err, out)
		return 1
	}
	return fixture.Main(m)
}

// result is one finished run of the binary.
type result struct {
	code           int
	stdout, stderr string
}

// assessgap runs the binary with args in dir (the repository root if
// empty). ASSESSGAP_* variables are dropped from the environment, then env
// is added.
func assessgap(t *testing.T, dir string, env []string, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	if dir == "" {
		cmd.Dir = repoRoot
	}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ASSESSGAP_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("assessgap %s: %v", strings.Join(args, " "), err)
	}
	return result{code, out.String(), errb.String()}
}

// server is a running "assessgap serve".
type server struct {
	base   string
	cmd    *exec.Cmd
	done   chan error // the process's exit
	exited bool

	mu  sync.Mutex
	log bytes.Buffer // everything the server wrote to stderr
}

// logText returns the server's stderr so far.
func (s *server) logText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.log.String()
}

// startServe runs "assessgap serve" on a random local port and waits until
// it logs the address it bound.
func startServe(t *testing.T, args ...string) *server {
	t.Helper()
	args = append([]string{"serve", "-addr", "127.0.0.1:0"}, args...)
	cmd := exec.Command(bin, args...)
	cmd.Dir = repoRoot
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &server{cmd: cmd, done: make(chan error, 1)}
	addr := make(chan string, 1)
	go func() {
		const marker = "msg=listening addr="
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			line := sc.Text()
			s.mu.Lock()
			s.log.WriteString(line + "\n")
			s.mu.Unlock()
			if _, rest, ok := strings.Cut(line, marker); ok {
				a, _, _ := strings.Cut(rest, " ")
				select {
				case addr <- a:
				default:
				}
			}
		}
		close(addr)
		s.done <- cmd.Wait()
	}()
	t.Cleanup(func() {
		if !s.exited {
			_ = cmd.Process.Kill()
			if err := <-s.done; err != nil {
				t.Logf("serve killed at cleanup: %v", err)
			}
		}
	})
	select {
	case a, ok := <-addr:
		if !ok {
			t.Fatalf("serve exited before listening:\n%s", s.logText())
		}
		s.base = "http://" + a
	case <-time.After(30 * time.Second):
		t.Fatalf("serve did not start in 30s:\n%s", s.logText())
	}
	return s
}

// stop sends SIGINT and returns how the server exited.
func (s *server) stop(t *testing.T) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no SIGINT on Windows")
	}
	if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-s.done:
		s.exited = true
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not exit 15s after SIGINT")
		return nil
	}
}
