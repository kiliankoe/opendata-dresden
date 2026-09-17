package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"golang.org/x/sync/errgroup"
)

// mirrorWorkers stays below describeWorkers because these downloads are whole
// tables rather than pages, and the portal starts refusing connections when
// several of them run at once.
const mirrorWorkers = 2

// statsCSVPath matches the download of a statistics table. Geodata layers
// publish their CSV under /ogcapi instead and are three orders of magnitude
// larger in total, so they stay out of the mirror.
var statsCSVPath = regexp.MustCompile(`/dcat-ap/dataset/([^/]+)/content\.csv$`)

// statsPrefix is the part of the DCAT identifier that every dataset shares
const statsPrefix = "de-sn-dresden-"

// statsFile names the mirror file of a dataset's statistics table, or returns
// an empty string for datasets that publish no such table.
func statsFile(ds *portal.Dataset) string {
	for _, r := range ds.Resources {
		if !strings.EqualFold(r.Format, "CSV") {
			continue
		}
		if m := statsCSVPath.FindStringSubmatch(r.URL); m != nil {
			return strings.TrimPrefix(m[1], statsPrefix) + ".csv"
		}
	}
	return ""
}

// Mirror keeps dir in sync with the statistics tables of the index and
// returns the number of files it wrote. Datasets listed in changed are
// downloaded again, as are those whose file is missing, which is how a run
// that failed halfway resumes. Files of datasets the portal no longer lists
// are removed, so their disappearance shows up in the history.
func Mirror(ctx context.Context, client *portal.Client, idx *Index, changed []string, dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	stale := make(map[string]bool, len(changed))
	for _, id := range changed {
		stale[id] = true
	}

	type job struct {
		dataset *portal.Dataset
		path    string
	}
	var todo []job
	wanted := make(map[string]bool, len(idx.Datasets))
	for i := range idx.Datasets {
		ds := &idx.Datasets[i]
		name := statsFile(ds)
		if name == "" {
			continue
		}
		wanted[name] = true
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil && !stale[ds.ID] {
			continue
		}
		todo = append(todo, job{ds, path})
	}

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(mirrorWorkers)
	for _, j := range todo {
		group.Go(func() error {
			data, err := client.FetchResource(ctx, j.dataset, "CSV", portal.FetchOptions{})
			if err != nil {
				return fmt.Errorf("mirroring %s: %w", j.dataset.ID, err)
			}
			return os.WriteFile(j.path, data, 0o644)
		})
	}
	if err := group.Wait(); err != nil {
		return 0, err
	}
	return len(todo), prune(dir, wanted)
}

func prune(dir string, wanted map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if name := e.Name(); strings.HasSuffix(name, ".csv") && !wanted[name] {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}
