package gitserver

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
)

const flushPkt = "0000"

func pktLine(payload string) []byte {
	length := len(payload) + 4
	return fmt.Appendf(nil, "%04x%s", length, payload)
}

func writeInfoRefs(ctx context.Context, w http.ResponseWriter, dir, service string) error {
	out, err := gitutil.RunOutput(ctx, dir, service, "--stateless-rpc", "--advertise-refs", dir)
	if err != nil {
		return domain.Error(err, "failed to advertise refs", errAttrsService(service)...)
	}

	w.Header().Set("Content-Type", "application/x-git-"+service+"-advertisement")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(pktLine(fmt.Sprintf("# service=git-%s\n", service))); err != nil {
		return fmt.Errorf("write pkt-line header: %w", err)
	}

	if _, err := io.WriteString(w, flushPkt); err != nil {
		return fmt.Errorf("write flush-pkt: %w", err)
	}

	if _, err := io.WriteString(w, out); err != nil {
		return fmt.Errorf("write ref advertisement: %w", err)
	}

	return nil
}

func requestBody(r *http.Request) (io.Reader, func(), error) {
	if r.Header.Get("Content-Encoding") != "gzip" {
		return r.Body, func() {}, nil
	}

	gz, err := gzip.NewReader(r.Body)
	if err != nil {
		return nil, func() {}, domain.Error(err, "failed to decode gzip request body")
	}

	return gz, func() { _ = gz.Close() }, nil
}

func servePack(ctx context.Context, w http.ResponseWriter, r *http.Request, dir, service string) error {
	body, cleanup, err := requestBody(r)
	if err != nil {
		return err
	}
	defer cleanup()

	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, "git", service, "--stateless-rpc", dir)
	cmd.Stdin = body
	cmd.Stdout = w
	cmd.Stderr = &stderr

	w.Header().Set("Content-Type", "application/x-git-"+service+"-result")

	if err := cmd.Run(); err != nil {
		return domain.Error(err, "git pack command failed", append(errAttrsService(service), slog.String("stderr", stderr.String()))...)
	}

	return nil
}

func errAttrsService(service string) []slog.Attr {
	return []slog.Attr{slog.String("service", service)}
}
