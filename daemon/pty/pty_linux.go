//go:build !windows

package pty

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type PtySession struct {
	ptyFile *os.File
	cmd     *exec.Cmd
}

// StartPty spawns a shell process under a PTY session (Linux / Unix-compatible)
func StartPty(cmdName string, args []string, rows uint16, cols uint16) (*PtySession, error) {
	cmd := exec.Command(cmdName, args...)

	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
	if err != nil {
		return nil, err
	}

	return &PtySession{
		ptyFile: ptyFile,
		cmd:     cmd,
	}, nil
}

func (ps *PtySession) Read(buf []byte) (int, error) {
	return ps.ptyFile.Read(buf)
}

func (ps *PtySession) Write(buf []byte) (int, error) {
	return ps.ptyFile.Write(buf)
}

// Resize resizes the PTY window geometry using system ioctl commands
func (ps *PtySession) Resize(rows uint16, cols uint16) error {
	return pty.Setsize(ps.ptyFile, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

func (ps *PtySession) Close() error {
	_ = ps.ptyFile.Close()
	if ps.cmd.Process != nil {
		_ = ps.cmd.Process.Kill()
	}
	return nil
}
