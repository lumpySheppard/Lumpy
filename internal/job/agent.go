package job

import (
	"context"
	"fmt"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/scan"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// AgentRun logs every drive mounted since the last run and runs any full
// check queued for it. It repeats until the set of mounted drives stops
// changing, so drives plugged in while it works are picked up too.
func AgentRun(ctx context.Context, db *catalog.DB, r *Report, notify func(title, msg string)) error {
	return AgentRunWith(ctx, db, r, notify, nil)
}

// AgentRunWith is AgentRun, but hands each queued check to startCheck
// (when not nil) instead of running it in place. The app uses this to run
// checks as jobs it can show.
func AgentRunWith(ctx context.Context, db *catalog.DB, r *Report, notify func(title, msg string), startCheck func(v *volume.Volume)) error {
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
			agentDrive(ctx, db, v, r, notify, startCheck)
		}
	}
	return nil
}

func agentDrive(ctx context.Context, db *catalog.DB, v *volume.Volume, r *Report, notify func(title, msg string), startCheck func(v *volume.Volume)) {
	r.Info("%s  %s mounted at %s", time.Now().Format("2006-01-02 15:04:05"), v.Name, v.Mount)
	if d, err := db.Drive(v.ID); err == nil && d.NeverLog {
		r.Info("%s is set to never be logged; skipped", d.Title())
		return
	} else if err != nil && !AutoLog {
		r.Info("%s is new and automatic logging is off (Settings); not logged", v.Name)
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
	if startCheck != nil {
		startCheck(v)
		return
	}
	QueuedCheck(ctx, db, v, r, notify)
}

// QueuedCheck runs the full check queued for a drive that has just been
// plugged in and posts a notification with the outcome. The check lists
// the drive itself, so files that disappeared are compared with what was
// recorded before and counted as failures.
func QueuedCheck(ctx context.Context, db *catalog.DB, v *volume.Volume, r *Report, notify func(title, msg string)) {
	d, err := db.Drive(v.ID)
	if err != nil {
		r.Info("lumpy: %v", err)
		return
	}
	name := d.Title()
	notify("Lumpy", fmt.Sprintf("Checking %s (%s). Keep it plugged in.", name, Bytes(d.Bytes)))
	before := r.Failures()
	if err := HashDrive(ctx, db, v, true, r); err != nil {
		// Unplugged or interrupted: the check stays queued for next time.
		r.Info("Check stopped: %v. It stays queued and runs again next time %s is plugged in.", err, name)
		db.Log(v.ID, "verified", "check interrupted: "+err.Error()+"; still queued", 0)
		notify("Lumpy: check of "+name+" stopped", "It will run again next time the drive is plugged in.")
		return
	}
	if n := r.Failures() - before; n > 0 {
		notify("Lumpy: "+name+" FAILED", fmt.Sprintf("%d problem(s) found. Open Lumpy for details.", n))
	} else {
		notify("Lumpy: "+name+" verified", "Every file matched its recorded checksum.")
	}
}

// AutoLog says whether drives seen for the first time are logged by
// themselves; off, only drives already in the library are kept up to
// date. AutoMetadata says whether scans read clip metadata.
var (
	AutoLog      = true
	AutoMetadata = true
)

// ApplyPrefs puts the library's preferences into effect for this process.
func ApplyPrefs(p catalog.Prefs) {
	AutoLog = p.AutoLog
	AutoMetadata = p.AutoMetadata
	volume.Hide.Internal = p.HideInternal
	volume.Hide.Images = p.HideImages
	scan.SetExtraIgnores(p.ExcludeList())
}
