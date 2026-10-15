// Package smart reads a drive's own health report (SMART) through
// smartctl, when smartmontools is installed.
package smart

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Health is what a drive says about itself, boiled down.
type Health struct {
	ReadAt int64 `json:"readAt"`
	// Passed is the drive's own verdict; nil when it gave none.
	Passed      *bool  `json:"passed,omitempty"`
	Model       string `json:"model,omitempty"`
	Serial      string `json:"serial,omitempty"`
	Hours       int64  `json:"hours,omitempty"`       // powered on, in all
	Temp        int    `json:"temp,omitempty"`        // °C now
	Reallocated int64  `json:"reallocated,omitempty"` // sectors moved to spares (ATA)
	Pending     int64  `json:"pending,omitempty"`     // sectors waiting to be (ATA)
	Uncorrect   int64  `json:"uncorrect,omitempty"`   // uncorrectable sectors (ATA) or media errors (NVMe)
	CRC         int64  `json:"crc,omitempty"`         // interface CRC errors: a cable or bridge (ATA)
	PercentUsed int    `json:"percentUsed,omitempty"` // of the rated write life (NVMe)
	// Warnings are the lines worth showing, worst first.
	Warnings []string `json:"warnings,omitempty"`
	// Error says why nothing could be read.
	Error string `json:"error,omitempty"`
}

// Bad reports whether the drive is failing by its own verdict.
func (h *Health) Bad() bool { return h != nil && h.Passed != nil && !*h.Passed }

// Program finds smartctl: Homebrew's, MacPorts' or one on the PATH.
// Tests replace it.
var Program = func() string {
	for _, p := range []string{"/opt/homebrew/bin/smartctl", "/usr/local/bin/smartctl", "/opt/local/bin/smartctl"} {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	p, _ := exec.LookPath("smartctl")
	return p
}

// Read asks the whole disk (disk4, or /dev/disk4) for its health. It
// never fails: what went wrong is in the result's Error.
func Read(ctx context.Context, disk string) *Health {
	h := &Health{ReadAt: time.Now().Unix()}
	prog := Program()
	if prog == "" {
		h.Error = "smartmontools is not installed"
		return h
	}
	if disk == "" {
		h.Error = "no disk device for this volume"
		return h
	}
	if !strings.HasPrefix(disk, "/dev/") {
		disk = "/dev/" + disk
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// A non-zero exit is normal: smartctl flags a failing drive that way.
	out, _ := exec.CommandContext(ctx, prog, "-j", "-a", disk).Output()
	if len(out) == 0 {
		h.Error = "smartctl gave no answer"
		return h
	}
	p, err := Parse(out)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	p.ReadAt = h.ReadAt
	return p
}

type raw struct {
	Smartctl struct {
		Messages []struct {
			String   string `json:"string"`
			Severity string `json:"severity"`
		} `json:"messages"`
	} `json:"smartctl"`
	Model  string `json:"model_name"`
	Serial string `json:"serial_number"`
	Status *struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	PowerOn *struct {
		Hours int64 `json:"hours"`
	} `json:"power_on_time"`
	Temperature *struct {
		Current int `json:"current"`
	} `json:"temperature"`
	ATA *struct {
		Table []struct {
			ID  int `json:"id"`
			Raw struct {
				Value int64 `json:"value"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
	NVMe *struct {
		CriticalWarning int   `json:"critical_warning"`
		Temperature     int   `json:"temperature"`
		PercentageUsed  int   `json:"percentage_used"`
		MediaErrors     int64 `json:"media_errors"`
		PowerOnHours    int64 `json:"power_on_hours"`
	} `json:"nvme_smart_health_information_log"`
}

// Parse reads smartctl's JSON output (smartctl -j -a).
func Parse(out []byte) (*Health, error) {
	var r raw
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("smartctl's answer isn't JSON: %v", err)
	}
	h := &Health{Model: r.Model, Serial: r.Serial}
	if r.Status == nil && r.ATA == nil && r.NVMe == nil {
		msg := "the drive gave no SMART data"
		for _, m := range r.Smartctl.Messages {
			if m.Severity == "error" {
				msg = m.String
				break
			}
		}
		if strings.Contains(msg, "Unable to detect device type") || strings.Contains(msg, "Operation not supported") || strings.Contains(msg, "Unknown USB bridge") {
			msg = "This drive doesn't answer SMART over its connection; most USB enclosures don't on macOS"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	if r.Status != nil {
		p := r.Status.Passed
		h.Passed = &p
	}
	if r.PowerOn != nil {
		h.Hours = r.PowerOn.Hours
	}
	if r.Temperature != nil {
		h.Temp = r.Temperature.Current
	}
	if r.ATA != nil {
		for _, a := range r.ATA.Table {
			switch a.ID {
			case 5:
				h.Reallocated = a.Raw.Value
			case 197:
				h.Pending = a.Raw.Value
			case 198:
				h.Uncorrect = a.Raw.Value
			case 199:
				// The raw value packs other counters above the low 32 bits
				// on some drives.
				h.CRC = a.Raw.Value & 0xffffffff
			}
		}
	}
	if r.NVMe != nil {
		h.PercentUsed = r.NVMe.PercentageUsed
		h.Uncorrect = r.NVMe.MediaErrors
		if h.Hours == 0 {
			h.Hours = r.NVMe.PowerOnHours
		}
		if h.Temp == 0 {
			h.Temp = r.NVMe.Temperature
		}
		if r.NVMe.CriticalWarning != 0 {
			h.Warnings = append(h.Warnings, "The drive raised a critical warning")
		}
	}
	if h.Bad() {
		h.Warnings = append([]string{"The drive reports it is failing"}, h.Warnings...)
	}
	if h.Reallocated > 0 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("%d reallocated sector%s", h.Reallocated, plural(h.Reallocated)))
	}
	if h.Pending > 0 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("%d sector%s pending reallocation", h.Pending, plural(h.Pending)))
	}
	if h.Uncorrect > 0 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("%d uncorrectable error%s", h.Uncorrect, plural(h.Uncorrect)))
	}
	if h.PercentUsed >= 90 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("%d%% of the rated write life used", h.PercentUsed))
	}
	if h.Temp >= 55 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("Running hot: %d °C", h.Temp))
	}
	if h.CRC > 0 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("%d interface CRC error%s: suspect the cable or enclosure", h.CRC, plural(h.CRC)))
	}
	return h, nil
}

func plural(n int64) string {
	if n == 1 {
		return ""
	}
	return "s"
}
