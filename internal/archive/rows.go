package archive

import (
	"bufio"
	"bytes"
	"container/heap"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// A geodata layer's CSV is archived as its records rather than its bytes: the
// server returns the records in a new order every night and most layers change
// by a few records at most, so storing each night's download would mostly store
// the same records again. A layer's table is kept in canonical form, the header
// followed by the other records sorted by their bytes, and each version as the
// records that left or joined it since the previous one.

// chunkSize bounds the bytes of records sorted in memory at once. Larger tables
// are sorted in chunks spilled to temporary files. Tests lower it.
var chunkSize = 128 << 20

// errNotTable marks content that is no complete CSV table, which is then
// stored as it came
var errNotTable = errors.New("not a CSV table")

// isTable tells whether a response holds a layer's table worth storing as rows
func isTable(layer bool, format, contentType string) bool {
	media, _, _ := mime.ParseMediaType(contentType)
	return layer && strings.EqualFold(format, "CSV") && media == "text/csv"
}

// tables keeps the latest stored version of every layer table in canonical
// form, as the base of the next delta. Versions written by a run are staged in
// the temporary directory and move into place only once the run's manifest is
// complete, so that no delta is ever based on a version no manifest names.
// The directory can be deleted; the next run then stores full versions.
//
//	state/<sha256 of the URL>/<year>-<rows>.zst
//
// The year is that of the full version the chain of deltas starts from.
type tables struct {
	dir, staged string
	year        int // of the run, which starts a new chain in a new year
	store       *store
}

func openTables(dir string, store *store, year int) *tables {
	return &tables{dir: filepath.Join(dir, "state"), staged: filepath.Join(store.tmp, "state"), year: year, store: store}
}

type version struct {
	path, rows string
	year       int
}

func (t *tables) key(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])
}

// base returns the stored version of url to build on, or none when a new
// chain has to start. After an interrupted commit a URL can have two
// versions; either is named by a manifest, the previous run's is preferred.
func (t *tables) base(url, previousRows string) (*version, error) {
	files, err := filepath.Glob(filepath.Join(t.dir, t.key(url), "*.zst"))
	if err != nil {
		return nil, err
	}
	var found *version
	for _, f := range files {
		year, rows, ok := strings.Cut(strings.TrimSuffix(filepath.Base(f), ".zst"), "-")
		y, err := strconv.Atoi(year)
		if !ok || err != nil || y != t.year {
			continue
		}
		found = &version{path: f, rows: rows, year: y}
		if rows == previousRows {
			break
		}
	}
	return found, nil
}

// update stores the table in content, the download of url, as the changes
// since its base and sets the entry's Rows, Delta and DeltaFrom. It returns
// errNotTable for content that does not split into complete records.
func (t *tables) update(content *staged, url string, previous Entry, e *Entry) error {
	base, err := t.base(url, previous.Rows)
	if err != nil {
		return err
	}
	if base != nil && base.rows == previous.Rows && content.sum() == previous.SHA256 {
		e.Rows = base.rows
		return nil
	}
	err = t.write(content, url, base, e)
	var bad *badBase
	if errors.As(err, &bad) {
		// The base only saves space, so a damaged one costs a full version
		err = t.write(content, url, nil, e)
	}
	return err
}

// badBase marks a stored version that cannot be read back as named
type badBase struct{ err error }

func (e *badBase) Error() string { return "reading the previous version: " + e.err.Error() }

func (t *tables) write(content *staged, url string, base *version, e *Entry) error {
	src, err := content.reader()
	if err != nil {
		return err
	}
	header, sorted, cleanup, err := sortRecords(bufio.NewReader(src), t.store.tmp)
	if err != nil {
		return err
	}
	defer cleanup()

	var old recordSource = emptySource{}
	if base != nil {
		c, err := openCanonical(base.path)
		if err != nil {
			return &badBase{err}
		}
		defer c.close()
		old = newChecked(c, base.rows)
	}
	delta, err := t.store.stage()
	if err != nil {
		return err
	}
	defer func() { delta.discard() }()
	next, err := newState(t.store.tmp)
	if err != nil {
		return err
	}
	defer next.discard()
	if err := diff(old, header, sorted, delta, next); err != nil {
		return err
	}
	if err := next.close(); err != nil {
		return err
	}

	e.Rows = next.sum()
	if base != nil && e.Rows == base.rows {
		return nil
	}
	// A delta larger than the table, as from a column rewritten in every
	// record, would only lengthen the chain
	if base != nil && delta.size > next.size {
		delta.discard()
		if delta, err = t.store.stage(); err != nil {
			return err
		}
		if err := full(next.file.Name(), delta); err != nil {
			return err
		}
		base = nil
	}
	if e.Delta, err = t.store.keep(delta); err != nil {
		return err
	}
	if base != nil {
		e.DeltaFrom = base.rows
	}
	return next.move(filepath.Join(t.staged, t.key(url), fmt.Sprintf("%d-%s.zst", t.year, e.Rows)))
}

// full writes the stored table at path as a delta against the empty table
func full(path string, w io.Writer) error {
	c, err := openCanonical(path)
	if err != nil {
		return err
	}
	defer c.close()
	out := bufio.NewWriter(w)
	_, _ = out.Write(c.header)
	_ = out.WriteByte('\n')
	for {
		record, err := c.records.next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		_ = out.WriteByte('+')
		_, _ = out.Write(record)
		_ = out.WriteByte('\n')
	}
	return out.Flush()
}

// state is a table's canonical form being compressed into a temporary file,
// hashed and measured as it is written. It is rewritten every night and never
// published, so it gets a faster level than objects.
type state struct {
	file    *os.File
	encoder *zstd.Encoder
	hash    hash.Hash
	size    int64
}

func newState(dir string) (*state, error) {
	f, err := os.CreateTemp(dir, "state-")
	if err != nil {
		return nil, err
	}
	encoder, err := zstd.NewWriter(f, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &state{file: f, encoder: encoder, hash: sha256.New()}, nil
}

func (s *state) Write(p []byte) (int, error) {
	n, err := s.encoder.Write(p)
	s.hash.Write(p[:n])
	s.size += int64(n)
	return n, err
}

func (s *state) sum() string { return hex.EncodeToString(s.hash.Sum(nil)) }

func (s *state) close() error {
	if err := s.encoder.Close(); err != nil {
		return err
	}
	return s.file.Sync()
}

// move puts the state where tables.commit finds it
func (s *state) move(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.Rename(s.file.Name(), path)
}

func (s *state) discard() {
	_ = s.encoder.Close()
	_ = s.file.Close()
	_ = os.Remove(s.file.Name())
}

// checked passes on the records of a stored version and fails with badBase
// when they do not hash to the version's rows
type checked struct {
	source *canonical
	hash   hash.Hash
	rows   string
}

func newChecked(c *canonical, rows string) *checked {
	h := sha256.New()
	h.Write(c.header)
	h.Write([]byte{'\n'})
	return &checked{source: c, hash: h, rows: rows}
}

func (c *checked) next() ([]byte, error) {
	record, err := c.source.records.next()
	switch {
	case err == io.EOF && hex.EncodeToString(c.hash.Sum(nil)) != c.rows:
		return nil, &badBase{errors.New("its records do not match its name")}
	case err == io.EOF:
		return nil, err
	case err != nil:
		return nil, &badBase{err}
	}
	c.hash.Write(record)
	c.hash.Write([]byte{'\n'})
	return record, nil
}

// commit moves the versions staged by a run into place and drops the ones
// they replace. It runs once the run's manifest is complete.
func (t *tables) commit() error {
	keys, err := os.ReadDir(t.staged)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	for _, key := range keys {
		files, err := os.ReadDir(filepath.Join(t.staged, key.Name()))
		if err != nil {
			return err
		}
		dir := filepath.Join(t.dir, key.Name())
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, f := range files {
			if err := os.Rename(filepath.Join(t.staged, key.Name(), f.Name()), filepath.Join(dir, f.Name())); err != nil {
				return err
			}
		}
		current, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, f := range current {
			if !slices.ContainsFunc(files, func(s os.DirEntry) bool { return s.Name() == f.Name() }) {
				if err := os.Remove(filepath.Join(dir, f.Name())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// diff writes the canonical form of the new table to next and its changes
// against the records of the base, removed, to delta. Both list the
// header first; delta then holds "-" or "+" and a record per line, in the
// order of the records.
func diff(removed recordSource, header []byte, sorted recordSource, delta, next io.Writer) error {
	deltaOut, nextOut := bufio.NewWriter(delta), bufio.NewWriter(next)
	write := func(w *bufio.Writer, prefix string, record []byte) {
		_, _ = w.WriteString(prefix)
		_, _ = w.Write(record)
		_ = w.WriteByte('\n')
	}
	write(deltaOut, "", header)
	write(nextOut, "", header)

	a, errA := removed.next()
	b, errB := sorted.next()
	for errA == nil || errB == nil {
		switch {
		case errB != nil || errA == nil && bytes.Compare(a, b) < 0:
			write(deltaOut, "-", a)
			a, errA = removed.next()
		case errA != nil || bytes.Compare(a, b) > 0:
			write(deltaOut, "+", b)
			write(nextOut, "", b)
			b, errB = sorted.next()
		default:
			write(nextOut, "", b)
			a, errA = removed.next()
			b, errB = sorted.next()
		}
		if errA != nil && errA != io.EOF {
			return errA
		}
		if errB != nil && errB != io.EOF {
			return errB
		}
	}
	if err := deltaOut.Flush(); err != nil {
		return err
	}
	return nextOut.Flush()
}

// Table writes the version of a geodata layer's table that a manifest names
// by its Rows to w, in canonical form: the header, then the other records
// sorted by their bytes, each ending in "\n". It applies the version's chain
// of deltas, starting from the full version, in temporary files.
func Table(dir, rows string, w io.Writer) error {
	deltas, err := deltasByRows(filepath.Join(dir, "runs"))
	if err != nil {
		return err
	}
	var chain []Entry
	for at := rows; ; {
		e, ok := deltas[at]
		if !ok {
			return fmt.Errorf("no archived table with rows %s", at)
		}
		chain = append(chain, e)
		if e.DeltaFrom == "" {
			break
		}
		if len(chain) > len(deltas) {
			return fmt.Errorf("deltas of rows %s form a cycle", rows)
		}
		at = e.DeltaFrom
	}
	objects := &store{dir: filepath.Join(dir, "objects")}

	var current string // canonical form of the version applied last
	defer func() { _ = os.Remove(current) }()
	for i := len(chain) - 1; i >= 0; i-- {
		out, err := os.CreateTemp("", "od3-table-")
		if err != nil {
			return err
		}
		hash := sha256.New()
		if err := apply(objects, current, chain[i].Delta, io.MultiWriter(out, hash)); err != nil {
			_ = out.Close()
			_ = os.Remove(out.Name())
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		_ = os.Remove(current)
		current = out.Name()
		if hex.EncodeToString(hash.Sum(nil)) != chain[i].Rows {
			return fmt.Errorf("applying delta %s does not yield rows %s", chain[i].Delta, chain[i].Rows)
		}
	}
	f, err := os.Open(current)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(w, f)
	return err
}

// deltasByRows maps the Rows of every stored table version to the first
// entry that stored it, or to a later full version
func deltasByRows(dir string) (map[string]Entry, error) {
	// Glob would find nothing in a mistyped directory
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	manifests, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	deltas := map[string]Entry{}
	for _, m := range manifests {
		f, err := os.Open(m)
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(f)
		for {
			var e Entry
			if err := decoder.Decode(&e); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				_ = f.Close()
				return nil, fmt.Errorf("reading %s: %w", m, err)
			}
			// The first entry builds on a version stored before it, so
			// following DeltaFrom always leads back in time
			if known, ok := deltas[e.Rows]; e.Delta != "" && (!ok || known.DeltaFrom != "" && e.DeltaFrom == "") {
				deltas[e.Rows] = e
			}
		}
		_ = f.Close()
	}
	return deltas, nil
}

// apply writes the canonical table that results from the delta object to w,
// with base the file of the canonical table it applies to, empty for a full
// version
func apply(objects *store, base, delta string, w io.Writer) error {
	d, err := openCanonical(objects.path(delta))
	if err != nil {
		return err
	}
	defer d.close()
	var old recordSource = emptySource{}
	if base != "" {
		f, err := os.Open(base)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		records := &splitter{r: bufio.NewReader(f)}
		if _, err := records.next(); err != nil { // the base's header
			return err
		}
		old = records
	}

	out := bufio.NewWriter(w)
	write := func(record []byte) {
		_, _ = out.Write(record)
		_ = out.WriteByte('\n')
	}
	write(d.header)
	a, errA := old.next()
	for {
		line, err := d.records.next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if len(line) == 0 || line[0] != '+' && line[0] != '-' {
			return fmt.Errorf("delta %s holds a line without + or -", delta)
		}
		record := line[1:]
		for errA == nil && bytes.Compare(a, record) < 0 {
			write(a)
			a, errA = old.next()
		}
		if errA != nil && errA != io.EOF {
			return errA
		}
		if line[0] == '+' {
			write(record)
			continue
		}
		if errA != nil || !bytes.Equal(a, record) {
			return fmt.Errorf("delta %s removes a record its base lacks", delta)
		}
		a, errA = old.next()
	}
	for errA == nil {
		write(a)
		a, errA = old.next()
	}
	if errA != io.EOF {
		return errA
	}
	return out.Flush()
}

// canonical reads a zstd file of a header followed by records, a stored
// table or delta
type canonical struct {
	file    *os.File
	decoder *zstd.Decoder
	header  []byte
	records *splitter
}

func openCanonical(path string) (*canonical, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	decoder, err := zstd.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	c := &canonical{file: f, decoder: decoder, records: &splitter{r: bufio.NewReader(decoder)}}
	header, err := c.records.next()
	if err != nil {
		c.close()
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	c.header = slices.Clone(header)
	return c, nil
}

func (c *canonical) close() {
	c.decoder.Close()
	_ = c.file.Close()
}

// recordSource yields records in order. A record stays valid until the next
// call; io.EOF ends the records.
type recordSource interface {
	next() ([]byte, error)
}

type emptySource struct{}

func (emptySource) next() ([]byte, error) { return nil, io.EOF }

// splitter yields the records of a CSV: the lines, except that a line break
// inside a quoted field belongs to the record. The "\n" is dropped, a "\r"
// before it is kept.
type splitter struct {
	r      *bufio.Reader
	record []byte
}

func (s *splitter) next() ([]byte, error) {
	s.record = s.record[:0]
	quoted := false
	for {
		line, err := s.r.ReadSlice('\n')
		s.record = append(s.record, line...)
		quoted = quoted != (bytes.Count(line, []byte{'"'})%2 == 1)
		switch {
		case err == bufio.ErrBufferFull:
			continue
		case err == io.EOF:
			if len(s.record) == 0 {
				return nil, io.EOF
			}
			if quoted {
				return nil, errNotTable
			}
			return s.record, nil
		case err != nil:
			return nil, err
		case !quoted:
			return s.record[:len(s.record)-1], nil
		}
	}
}

// sortRecords reads a CSV's header and returns it with its other records in
// sorted order. Records beyond chunkSize are sorted in chunks written to
// temporary files in dir and merged; cleanup removes those.
func sortRecords(src *bufio.Reader, dir string) ([]byte, recordSource, func(), error) {
	records := &splitter{r: src}
	header, err := records.next()
	if err == io.EOF {
		return nil, nil, nil, errNotTable
	} else if err != nil {
		return nil, nil, nil, err
	}
	header = slices.Clone(header)

	var runs []*os.File
	cleanup := func() {
		for _, r := range runs {
			_ = r.Close()
			_ = os.Remove(r.Name())
		}
	}
	var chunk memoryRun
	for {
		record, err := records.next()
		if err != nil && err != io.EOF {
			cleanup()
			return nil, nil, nil, err
		}
		if err == nil {
			chunk.add(record)
		}
		if len(chunk.data) < chunkSize && err == nil {
			continue
		}
		chunk.sort()
		if err == io.EOF && len(runs) == 0 {
			return header, &chunk, cleanup, nil
		}
		run, spillErr := chunk.spill(dir)
		if run != nil {
			runs = append(runs, run)
		}
		if spillErr != nil {
			cleanup()
			return nil, nil, nil, spillErr
		}
		chunk = memoryRun{}
		if err == io.EOF {
			break
		}
	}

	merged := &merger{}
	for _, run := range runs {
		r := &fileRun{r: bufio.NewReader(run)}
		record, err := r.next()
		if err == io.EOF {
			continue
		} else if err != nil {
			cleanup()
			return nil, nil, nil, err
		}
		merged.heads = append(merged.heads, head{record, r})
	}
	heap.Init(merged)
	return header, merged, cleanup, nil
}

// memoryRun holds a chunk of records back to back
type memoryRun struct {
	data   []byte
	starts []int
	at     int
}

func (m *memoryRun) add(record []byte) {
	m.starts = append(m.starts, len(m.data))
	m.data = append(m.data, record...)
}

func (m *memoryRun) record(i int) []byte {
	end := len(m.data)
	if i+1 < len(m.starts) {
		end = m.starts[i+1]
	}
	return m.data[m.starts[i]:end]
}

// sort orders the records by rewriting data in their sorted order
func (m *memoryRun) sort() {
	order := make([]int, len(m.starts))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int { return bytes.Compare(m.record(a), m.record(b)) })
	data, starts := make([]byte, 0, len(m.data)), make([]int, 0, len(m.starts))
	for _, i := range order {
		starts = append(starts, len(data))
		data = append(data, m.record(i)...)
	}
	m.data, m.starts = data, starts
}

func (m *memoryRun) next() ([]byte, error) {
	if m.at == len(m.starts) {
		return nil, io.EOF
	}
	m.at++
	return m.record(m.at - 1), nil
}

// spill writes the sorted records to a temporary file, each preceded by its
// length, since records may hold line breaks
func (m *memoryRun) spill(dir string) (*os.File, error) {
	f, err := os.CreateTemp(dir, "sort-")
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriter(f)
	for i := range m.starts {
		record := m.record(i)
		_, _ = w.Write(binary.AppendUvarint(nil, uint64(len(record))))
		_, _ = w.Write(record)
	}
	if err := w.Flush(); err != nil {
		return f, err
	}
	_, err = f.Seek(0, io.SeekStart)
	return f, err
}

type fileRun struct {
	r      *bufio.Reader
	record []byte
}

func (f *fileRun) next() ([]byte, error) {
	n, err := binary.ReadUvarint(f.r)
	if err != nil {
		return nil, err
	}
	f.record = slices.Grow(f.record[:0], int(n))[:n]
	if _, err := io.ReadFull(f.r, f.record); err != nil {
		return nil, err
	}
	return f.record, nil
}

type head struct {
	record []byte
	run    *fileRun
}

// merger yields the records of sorted runs in sorted order. It advances the
// run of the record it returned on the following call, so that the record
// stays valid until then.
type merger struct {
	heads []head
	taken *fileRun
}

func (m *merger) next() ([]byte, error) {
	if m.taken != nil {
		record, err := m.taken.next()
		switch {
		case err == io.EOF:
			heap.Pop(m)
		case err != nil:
			return nil, err
		default:
			m.heads[0].record = record
			heap.Fix(m, 0)
		}
		m.taken = nil
	}
	if len(m.heads) == 0 {
		return nil, io.EOF
	}
	m.taken = m.heads[0].run
	return m.heads[0].record, nil
}

func (m *merger) Len() int           { return len(m.heads) }
func (m *merger) Less(i, j int) bool { return bytes.Compare(m.heads[i].record, m.heads[j].record) < 0 }
func (m *merger) Swap(i, j int)      { m.heads[i], m.heads[j] = m.heads[j], m.heads[i] }
func (m *merger) Push(x any)         { m.heads = append(m.heads, x.(head)) }
func (m *merger) Pop() any {
	last := m.heads[len(m.heads)-1]
	m.heads = m.heads[:len(m.heads)-1]
	return last
}
