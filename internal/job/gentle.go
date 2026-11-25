package job

import (
	"sync"
	"time"
)

// Gentle, when set, reads one file at a time and holds reads to about
// GentleRate bytes a second, so a drive stays responsive for the Mac
// and the Mac for whoever is editing while Lumpy works.
var (
	Gentle     bool
	GentleRate int64 = 150 << 20
)

var (
	paceMu   sync.Mutex
	paceAt   time.Time
	paceDebt int64
)

// pace is called after each chunk read. With Gentle on it sleeps just
// long enough that the bytes read in the last second stay under the rate.
func pace(n int) {
	if !Gentle || n <= 0 {
		return
	}
	paceMu.Lock()
	defer paceMu.Unlock()
	now := time.Now()
	if paceAt.IsZero() || now.Sub(paceAt) > time.Second {
		paceAt, paceDebt = now, 0
	}
	paceDebt += int64(n)
	want := time.Duration(float64(paceDebt) / float64(GentleRate) * float64(time.Second))
	if d := want - now.Sub(paceAt); d > 0 {
		time.Sleep(d)
	}
}
