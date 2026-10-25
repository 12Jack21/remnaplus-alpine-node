package xray

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	managedAccessLogName = "access.log"
	managedErrorLogName  = "error.log"
)

type managedLogPump struct {
	pipePath string
	stop     chan struct{}
	done     chan struct{}
}

func configureManagedLogPaths(config map[string]any, logDir string) []*managedLogPump {
	logConfig, ok := config["log"].(map[string]any)
	if !ok {
		return nil
	}
	pumps := make([]*managedLogPump, 0, 2)
	for _, item := range []struct {
		key  string
		name string
	}{
		{key: "access", name: managedAccessLogName},
		{key: "error", name: managedErrorLogName},
	} {
		path, ok := logConfig[item.key].(string)
		if !ok || path != filepath.Join(logDir, item.name) {
			continue
		}
		pipePath := filepath.Join(logDir, "."+item.name+".pipe")
		logConfig[item.key] = pipePath
		pumps = append(pumps, &managedLogPump{pipePath: pipePath, stop: make(chan struct{}), done: make(chan struct{})})
	}
	return pumps
}

func (p *managedLogPump) start(activePath string) error {
	if err := os.Remove(p.pipePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := syscall.Mkfifo(p.pipePath, 0o640); err != nil {
		return fmt.Errorf("create managed log fifo: %w", err)
	}
	writer, err := openSegmentedLogWriter(activePath, maxLogSize)
	if err != nil {
		_ = os.Remove(p.pipePath)
		return err
	}
	fd, err := syscall.Open(p.pipePath, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		_ = writer.Close()
		_ = os.Remove(p.pipePath)
		return err
	}
	go func() {
		defer close(p.done)
		defer writer.Close()
		defer os.Remove(p.pipePath)
		defer syscall.Close(fd)
		buffer := make([]byte, 64*1024)
		stopping := false
		for {
			select {
			case <-p.stop:
				stopping = true
			default:
			}
			n, readErr := syscall.Read(fd, buffer)
			if n > 0 {
				if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				if errors.Is(readErr, syscall.EAGAIN) || errors.Is(readErr, syscall.EWOULDBLOCK) {
					if stopping {
						return
					}
					select {
					case <-p.stop:
						return
					case <-time.After(20 * time.Millisecond):
						continue
					}
				}
				if !errors.Is(readErr, io.EOF) {
					return
				}
				return
			}
		}
	}()
	return nil
}

func (p *managedLogPump) close() {
	select {
	case <-p.done:
		return
	default:
	}
	close(p.stop)
	<-p.done
}
