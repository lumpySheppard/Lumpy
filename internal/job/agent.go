package job

import (
	"context"
	"fmt"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// AgentRun logs every drive mounted since the last run and runs any full
// check queued for it. It repeats until the set of mounted drives stops
// changing, so drives plugged in while it works are picked up too.
func AgentRun(ctx context.Context, db *catalog.DB, r *Report, notify func(title, msg string)) error {
	for round := 0; round < 50; round++ {
		vols, err := volume.Mounted()
		if err != nil {
			return err
		}
		was, err := db.WasMounted()
		if err != nil {
			return err
		}
		now := map[string]string{}
		var fresh []*volume.Volume
		for _, v := range vols {
			now[v.ID] = v.Mount
			if was[v.ID] != v.Mount {
				fresh = append(fresh, v)
			}
		}
		if err := db.SetMounted(now); err != nil {
			return err
		}
		if len(fresh) == 0 {
			return nil
		}
		for _, v := range fresh {
			if err := ctx.Err(); err != nil {
				return err
			}
			agentDrive(ctx, db, v, r, notify)
		}
	}
	return nil
}

func agentDrive(ctx context.Context, db *catalog.DB, v *volume.Volume, r *Report, notify func(title, msg string)) {
	r.Info("%s  %s mounted at %s", time.Now().Format("2006-01-02 15:04:05"), v.Name, v.Mount)
	if d, err := db.Drive(v.ID); err == nil && d.NeverLog {
		r.Info("%s is set to never be logged; skipped", d.Title())
		return
	}
	if err := db.SaveDrive(v); err != nil {
		r.Info("lumpy: %v", err)
		return
	}
	db.Log(v.ID, "mounted", "mounted at "+v.Mount, 0)
	d, err := db.Drive(v.ID)
	if err != nil {
		r.Info("lumpy: %v", err)
		return
	}
	if d.VerifyRequested.IsZero() {
		res, err := CatalogDrive(ctx, db, v, r)
		if err != nil {
			r.Info("lumpy: logging %s: %v", v.Name, err)
			return
		}
		if res.Removed > 0 {
			notify("Lumpy: files gone from "+v.Name, fmt.Sprintf("%d file(s) recorded earlier are no longer on this drive.", res.Removed))
		}
		return
	}
	// A queued check lists the drive itself, so files that disappeared are
	// compared with what was recorded before and counted as failures.
	notify("Lumpy", fmt.Sprintf("Checking %s (%s). Keep it plugged in.", v.Name, Bytes(d.Bytes)))
	before := r.Failures()
	if err := HashDrive(ctx, db, v, true, r); err != nil {
		// Unplugged or interrupted: the check stays queued for next time.
		db.Log(v.ID, "verified", "check interrupted: "+err.Error()+"; still queued", 0)
		notify("Lumpy: check of "+v.Name+" stopped", "It will run again next time the drive is plugged in.")
		return
	}
	if n := r.Failures() - before; n > 0 {
		notify("Lumpy: "+v.Name+" FAILED", fmt.Sprintf("%d problem(s) found. Run `lumpy drive %s` for details.", n, v.Name))
	} else {
		notify("Lumpy: "+v.Name+" verified", "Every file matched its recorded checksum.")
	}
}
