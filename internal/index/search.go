package index

import (
	"slices"
	"strings"

	"github.com/kiliankoe/opendata-dresden/internal/portal"
)

// Search ranks datasets by how many of the query's words appear in their
// title, then their topics, source and portal, then their description. Every word
// has to match somewhere. An empty query lists all datasets by title.
func (idx *Index) Search(query string, limit, offset int) *portal.SearchResult {
	terms := strings.Fields(fold(query))
	type hit struct {
		dataset portal.Dataset
		score   int
	}
	var hits []hit
	for _, ds := range idx.Datasets {
		score, ok := score(ds, terms)
		if ok {
			hits = append(hits, hit{ds, score})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int {
		if a.score != b.score {
			return b.score - a.score
		}
		return strings.Compare(fold(a.dataset.Title), fold(b.dataset.Title))
	})

	result := &portal.SearchResult{Total: len(hits), Datasets: []portal.Dataset{}}
	for _, h := range hits[min(offset, len(hits)):] {
		if len(result.Datasets) == limit {
			break
		}
		result.Datasets = append(result.Datasets, h.dataset)
	}
	return result
}

func score(ds portal.Dataset, terms []string) (int, bool) {
	fields := []struct {
		text   string
		weight int
	}{
		{ds.Title, 3},
		{strings.Join(ds.Topics, " ") + " " + ds.Source + " " + ds.Portal, 2},
		{ds.Description + " " + ds.Origin, 1},
	}
	total := 0
	for _, term := range terms {
		if term == strings.ToLower(ds.ID) {
			total += 10
			continue
		}
		matched := false
		for _, f := range fields {
			if strings.Contains(fold(f.text), term) {
				total += f.weight
				matched = true
			}
		}
		if !matched {
			return 0, false
		}
	}
	return total, true
}

// fold lowercases and spells out umlauts, so that "strasse" finds "Straße"
// and vice versa
func fold(s string) string {
	return folder.Replace(strings.ToLower(s))
}

var folder = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")
