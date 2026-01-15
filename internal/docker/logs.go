package docker

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

// LogLine represents a single log entry
type LogLine struct {
	ContainerID string
	Timestamp   time.Time
	Stream      string // "stdout", "stderr", or "system"
	Content     string
}

// StreamLogs starts streaming logs for a container and returns channels for log lines and errors
func (c *Client) StreamLogs(ctx context.Context, containerID string) (<-chan LogLine, <-chan error) {
	logChan := make(chan LogLine, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(logChan)
		defer close(errChan)

		// First, check container state and if it uses TTY
		inspect, err := c.cli.ContainerInspect(ctx, containerID)
		if err != nil {
			errChan <- err
			return
		}
		isTTY := inspect.Config.Tty
		isRunning := inspect.State.Running

		// For exited containers, get more lines and don't follow
		tail := "10"
		follow := true
		if !isRunning {
			tail = "50"
			follow = false
		}

		reader, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     follow,
			Tail:       tail,
			Timestamps: true,
		})
		if err != nil {
			errChan <- err
			return
		}
		defer func() { _ = reader.Close() }()

		if isTTY {
			// TTY mode: logs come through directly without multiplexing
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				select {
				case <-ctx.Done():
					return
				default:
				}
				line := scanner.Text()
				logLine := parseLine(containerID, "stdout", line)
				select {
				case <-ctx.Done():
					return
				case logChan <- logLine:
				}
			}
			if err := scanner.Err(); err != nil && err != io.EOF {
				errChan <- err
			}
		} else {
			// Non-TTY mode: Docker multiplexes stdout and stderr with an 8-byte header
			// Header format: [STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4]
			// STREAM_TYPE: 0=stdin, 1=stdout, 2=stderr
			hdr := make([]byte, 8)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Read the header
				_, err := io.ReadFull(reader, hdr)
				if err != nil {
					if err != io.EOF {
						errChan <- err
					}
					return
				}

				// Get stream type
				streamType := "stdout"
				if hdr[0] == 2 {
					streamType = "stderr"
				}

				// Get payload size
				size := int64(hdr[4])<<24 | int64(hdr[5])<<16 | int64(hdr[6])<<8 | int64(hdr[7])

				// Read the payload
				payload := make([]byte, size)
				_, err = io.ReadFull(reader, payload)
				if err != nil {
					if err != io.EOF {
						errChan <- err
					}
					return
				}

				// Parse lines from payload
				scanner := bufio.NewScanner(strings.NewReader(string(payload)))
				for scanner.Scan() {
					line := scanner.Text()
					logLine := parseLine(containerID, streamType, line)
					select {
					case <-ctx.Done():
						return
					case logChan <- logLine:
					}
				}
			}
		}
	}()

	return logChan, errChan
}

// parseLine parses a log line with optional timestamp
func parseLine(containerID, stream, line string) LogLine {
	logLine := LogLine{
		ContainerID: containerID,
		Stream:      stream,
		Timestamp:   time.Now(),
		Content:     line,
	}

	// Try to parse timestamp (format: 2024-01-15T10:30:45.123456789Z)
	if len(line) > 30 && line[4] == '-' && line[7] == '-' && line[10] == 'T' {
		if ts, err := time.Parse(time.RFC3339Nano, line[:30]); err == nil {
			logLine.Timestamp = ts
			logLine.Content = strings.TrimSpace(line[31:])
		}
	}

	return logLine
}
