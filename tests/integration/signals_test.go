//go:build integration

package integration

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/tests/testutil"
)

func TestServerProcessSignals(t *testing.T) {
	dsn := testutil.StartPostgres(t)
	testutil.Migrate(t, dsn, "up")
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	binary := filepath.Join(work, "server")
	buildCtx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-race", "-o", binary, "./cmd/server")
	build.Dir = root
	if err := build.Run(); err != nil {
		t.Fatal("build server process failed")
	}
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := listener.Addr().String()
			listener.Close()
			cmd := exec.Command(binary)
			cmd.Dir = work
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "APP_") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env, "APP_CONFIG_PATH="+filepath.Join(root, "configs/config.yaml"), "APP_DATABASE_URL="+dsn, "APP_HTTP_ADDR="+addr, "APP_HTTP_SHUTDOWN_TIMEOUT=1s")
			if err := cmd.Start(); err != nil {
				t.Fatal("start server process failed")
			}
			t.Cleanup(func() { _ = cmd.Process.Kill() })
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			client := &http.Client{Timeout: 200 * time.Millisecond}
			ready := false
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				select {
				case <-done:
					t.Fatal("server process exited during startup")
				default:
				}
				res, err := client.Get("http://" + addr + "/readyz")
				if err == nil {
					res.Body.Close()
					if res.StatusCode == http.StatusOK {
						ready = true
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !ready {
				t.Fatal("server process did not become ready with PostgreSQL")
			}
			if err := cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal("server process did not exit cleanly after signal")
				}
			case <-time.After(4 * time.Second):
				t.Fatal("server process ignored shutdown signal")
			}
		})
	}
}
