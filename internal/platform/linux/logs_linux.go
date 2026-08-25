//go:build linux

package linux

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"wartungsremote/internal/platform"
)

// journalEntry mirrors the subset of fields journalctl -o json emits that
// we care about; unknown fields are ignored.
type journalEntry struct {
	RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
	Message           any    `json:"MESSAGE"` // string or []byte-ish array for binary payloads
	Priority          string `json:"PRIORITY"`
	SyslogIdentifier  string `json:"SYSLOG_IDENTIFIER"`
	Unit              string `json:"_SYSTEMD_UNIT"`
}

func (p *Provider) QueryLogs(ctx context.Context, q platform.LogQuery) ([]platform.LogEntry, error) {
	limit := q.Limit
	if limit <= 0 || limit > 5000 {
		limit = 500
	}

	args := []string{"--no-pager", "-o", "json", "-n", strconv.Itoa(limit)}
	if q.Since != nil {
		args = append(args, "--since", q.Since.Format("2006-01-02 15:04:05"))
	}
	if q.Until != nil {
		args = append(args, "--until", q.Until.Format("2006-01-02 15:04:05"))
	}
	if q.Query != "" {
		args = append(args, "-g", q.Query)
	}
	if prio := journalPriority(q.Level); prio != "" {
		args = append(args, "-p", prio)
	}

	cmd := exec.CommandContext(ctx, "journalctl", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("linux: journalctl: %w", err)
	}

	var entries []platform.LogEntry
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var je journalEntry
		if err := json.Unmarshal(scanner.Bytes(), &je); err != nil {
			continue
		}
		entries = append(entries, platform.LogEntry{
			Time:    parseJournalTime(je.RealtimeTimestamp),
			Level:   levelFromPriority(je.Priority),
			Source:  firstNonEmpty(je.Unit, je.SyslogIdentifier),
			Message: fmt.Sprintf("%v", je.Message),
		})
	}
	return entries, nil
}

func journalPriority(level string) string {
	switch level {
	case "error":
		return "3" // 0-3: emerg..err
	case "warning":
		return "4" // warning
	case "info":
		return "6" // info
	default:
		return ""
	}
}

func levelFromPriority(p string) string {
	n, err := strconv.Atoi(p)
	if err != nil {
		return "unknown"
	}
	switch {
	case n <= 3:
		return "error"
	case n == 4:
		return "warning"
	default:
		return "info"
	}
}

func parseJournalTime(microsSinceEpoch string) time.Time {
	us, err := strconv.ParseInt(microsSinceEpoch, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMicro(us).UTC()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
