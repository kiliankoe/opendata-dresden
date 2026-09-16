// Package index builds and reads the snapshot of the portal's datasets that
// is committed to the repository and refreshed nightly. It adds the
// descriptions the portal's search does not return.
package index

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"golang.org/x/sync/errgroup"
)

// listLimit is the number of datasets requested at once. The portal's paging
// returns nothing past a few hundred results, so everything is fetched in one
// request and the request is repeated with the exact total should it be short.
const listLimit = 2000

// describeWorkers bounds the parallel information page requests to keep the
// load on the portal low
const describeWorkers = 4

type Index struct {
	Datasets []portal.Dataset `json:"datasets"`
}

// Read loads an index file. A missing file yields an empty index.
func Read(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Index{}, nil
	}
	if err != nil {
		return nil, err
	}
	return parse(data)
}

func parse(data []byte) (*Index, error) {
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// Write stores the index sorted by dataset ID, so that successive versions
// diff cleanly in git.
func (idx *Index) Write(path string) error {
	slices.SortFunc(idx.Datasets, func(a, b portal.Dataset) int { return strings.Compare(a.ID, b.ID) })
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Build lists every dataset of the portal. Details are fetched only for
// datasets that are new or changed since previous; the others keep theirs.
// It returns the new index and the number of new or changed datasets.
func Build(ctx context.Context, client *portal.Client, previous *Index) (*Index, int, error) {
	known := make(map[string]portal.Dataset, len(previous.Datasets))
	for _, ds := range previous.Datasets {
		known[ds.ID] = ds
	}

	all, err := client.SearchDatasets(ctx, "", listLimit, 0)
	if err != nil {
		return nil, 0, err
	}
	if all.Total > len(all.Datasets) {
		if all, err = client.SearchDatasets(ctx, "", all.Total, 0); err != nil {
			return nil, 0, err
		}
	}
	datasets := all.Datasets

	refreshed := 0
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(describeWorkers)
	for i := range datasets {
		ds := &datasets[i]
		if old, ok := known[ds.ID]; ok && old.Updated == ds.Updated {
			ds.Description, ds.Origin = old.Description, old.Origin
			continue
		}
		refreshed++
		group.Go(func() error { return client.Describe(ctx, ds) })
	}
	if err := group.Wait(); err != nil {
		return nil, 0, err
	}

	idx := &Index{Datasets: datasets}
	slices.SortFunc(idx.Datasets, func(a, b portal.Dataset) int { return strings.Compare(a.ID, b.ID) })
	return idx, refreshed, nil
}
