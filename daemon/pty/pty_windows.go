//go:build windows

package pty

import (
	"io"
	"os/exec"
)

type PtySession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

// StartPty spawns cmd.exe with stdin/stdout pipe streaming for Windows developer compilation support
func StartPty(cmdName string, args []string, rows uint16, cols uint16) (*PtySession, error) {
	cmd := exec.Command("cmd.exe")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &PtySession{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func (ps *PtySession) Read(buf []byte) (int, error) {
	return ps.stdout.Read(buf)
}

func (ps *PtySession) Write(buf []byte) (int, error) {
	return ps.stdin.Write(buf)
}

func (ps *PtySession) Resize(rows uint16, cols uint16) error {
	// Window resize is a no-op on Windows local mock
	return nil
}

func (ps *PtySession) Close() error {
	_ = ps.stdin.Close()
	_ = ps.stdout.Close()
	if ps.cmd.Process != nil {
		_ = ps.cmd.Process.Kill()
	}
	return nil
}
