//go:build windows

package windows

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"wartungsremote/internal/platform"
)

// winEvent mirrors the subset of the `wevtutil qe ... /f:xml` per-event XML
// fragment we need; unknown fields are ignored.
type winEvent struct {
	XMLName xml.Name `xml:"Event"`
	System  struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		Level       string `xml:"Level"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
		Channel string `xml:"Channel"`
	} `xml:"System"`
	RenderingInfo struct {
		Message string `xml:"Message"`
	} `xml:"RenderingInfo"`
	EventData struct {
		Data []string `xml:"Data"`
	} `xml:"EventData"`
}

// defaultChannel is used since docs/API.md's log query has no channel
// selector yet; "System" covers the maintenance-relevant events (services,
// drivers, hardware) most directly. Application-channel support can be
// added as an explicit query parameter later.
const defaultChannel = "System"

func (p *Provider) QueryLogs(ctx context.Context, q platform.LogQuery) ([]platform.LogEntry, error) {
	limit := q.Limit
	if limit <= 0 || limit > 5000 {
		limit = 500
	}

	xpath := buildXPath(q)
	args := []string{"qe", defaultChannel, "/c:" + strconv.Itoa(limit), "/rd:true", "/f:xml"}
	if xpath != "" {
		args = append(args, "/q:"+xpath)
	}

	cmd := exec.CommandContext(ctx, "wevtutil", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("windows: wevtutil: %w", err)
	}

	events, err := parseEvents(out)
	if err != nil {
		return nil, fmt.Errorf("windows: parse event log XML: %w", err)
	}

	entries := make([]platform.LogEntry, 0, len(events))
	for _, ev := range events {
		msg := ev.RenderingInfo.Message
		if msg == "" {
			msg = strings.Join(ev.EventData.Data, " ")
		}
		if q.Query != "" && !strings.Contains(strings.ToLower(msg), strings.ToLower(q.Query)) {
			continue
		}
		entries = append(entries, platform.LogEntry{
			Time:    parseWinTime(ev.System.TimeCreated.SystemTime),
			Level:   levelFromWinLevel(ev.System.Level),
			Source:  firstNonEmpty(ev.System.Provider.Name, ev.System.Channel),
			Message: msg,
		})
	}
	return entries, nil
}

// buildXPath translates the time-range/level filters into a wevtutil
// structured query; free-text search is applied client-side above since
// XPath can't easily match against the rendered message text.
func buildXPath(q platform.LogQuery) string {
	var conds []string
	if q.Since != nil {
		conds = append(conds, fmt.Sprintf("TimeCreated[@SystemTime>='%s']", q.Since.UTC().Format(time.RFC3339)))
	}
	if q.Until != nil {
		conds = append(conds, fmt.Sprintf("TimeCreated[@SystemTime<='%s']", q.Until.UTC().Format(time.RFC3339)))
	}
	if lvl := winLevelFilter(q.Level); lvl != "" {
		conds = append(conds, "Level="+lvl)
	}
	if len(conds) == 0 {
		return ""
	}
	return "*[System[" + strings.Join(conds, " and ") + "]]"
}

func winLevelFilter(level string) string {
	switch level {
	case "error":
		return "2"
	case "warning":
		return "3"
	case "info":
		return "4"
	default:
		return ""
	}
}

func levelFromWinLevel(level string) string {
	switch level {
	case "1", "2":
		return "error"
	case "3":
		return "warning"
	case "4", "0":
		return "info"
	default:
		return "unknown"
	}
}

func parseWinTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseEvents decodes the concatenated (non-well-formed-as-a-whole) stream
// of <Event>...</Event> fragments wevtutil prints, one XML document at a
// time.
func parseEvents(out []byte) ([]winEvent, error) {
	dec := xml.NewDecoder(bytes.NewReader(out))
	var events []winEvent
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "Event" {
			continue
		}
		var ev winEvent
		if err := dec.DecodeElement(&ev, &se); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, nil
}
