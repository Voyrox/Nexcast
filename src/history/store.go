package history

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxDailyFiles   = 7
	dayFormat       = "2006-01-02"
	dataFileSuffix  = ".bin"
	defaultDataPath = "history"
)

func NewStore(dir string) *Store {
	if strings.TrimSpace(dir) == "" {
		dir = defaultDataPath
	}
	return &Store{dir: dir}
}

func (s *Store) SaveSnapshot(snapshot ClusterSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	if err := s.pruneLocked(); err != nil {
		return err
	}

	timestamp := snapshot.Timestamp.UTC().Truncate(time.Hour)
	snapshot.Timestamp = timestamp
	dayKey := timestamp.Format(dayFormat)
	hourKey := timestamp.Hour()

	current, err := s.readDayLocked(dayKey)
	if err != nil {
		return err
	}
	if current.Snapshots == nil {
		current.Snapshots = make(map[int]ClusterSnapshot)
	}
	current.Date = dayKey
	current.Snapshots[hourKey] = snapshot

	if err := s.writeDayLocked(dayKey, current); err != nil {
		return err
	}

	return s.pruneLocked()
}

func (s *Store) Load() (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Response{}, err
	}
	if err := s.pruneLocked(); err != nil {
		return Response{}, err
	}

	files, err := s.listFilesLocked()
	if err != nil {
		return Response{}, err
	}

	resp := Response{Days: make([]DayHistory, 0, len(files))}
	for _, file := range files {
		current, err := s.readDayFromPathLocked(file.path)
		if err != nil {
			return Response{}, err
		}
		day := DayHistory{Date: current.Date, Snapshots: sortSnapshots(current.Snapshots)}
		resp.Days = append(resp.Days, day)
	}
	resp.Latest = latestSnapshot(resp.Days)
	return resp, nil
}

func (s *Store) readDayLocked(dayKey string) (dayFile, error) {
	path := filepath.Join(s.dir, dayKey+dataFileSuffix)
	return s.readDayFromPathLocked(path)
}

func (s *Store) readDayFromPathLocked(path string) (dayFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			date := strings.TrimSuffix(filepath.Base(path), dataFileSuffix)
			return dayFile{Date: date, Snapshots: map[int]ClusterSnapshot{}}, nil
		}
		return dayFile{}, err
	}

	var current dayFile
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&current); err != nil {
		return dayFile{}, fmt.Errorf("decode history %s: %w", path, err)
	}
	if current.Snapshots == nil {
		current.Snapshots = make(map[int]ClusterSnapshot)
	}
	if strings.TrimSpace(current.Date) == "" {
		current.Date = strings.TrimSuffix(filepath.Base(path), dataFileSuffix)
	}
	return current, nil
}

func (s *Store) writeDayLocked(dayKey string, current dayFile) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(current); err != nil {
		return err
	}
	path := filepath.Join(s.dir, dayKey+dataFileSuffix)
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func (s *Store) pruneLocked() error {
	files, err := s.listFilesLocked()
	if err != nil {
		return err
	}
	if len(files) <= maxDailyFiles {
		return nil
	}
	for _, file := range files[:len(files)-maxDailyFiles] {
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *Store) listFilesLocked() ([]datedFile, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]datedFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), dataFileSuffix) {
			continue
		}
		dayKey := strings.TrimSuffix(entry.Name(), dataFileSuffix)
		date, err := time.Parse(dayFormat, dayKey)
		if err != nil {
			continue
		}
		files = append(files, datedFile{
			date: date,
			path: filepath.Join(s.dir, entry.Name()),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].date.Before(files[j].date)
	})
	return files, nil
}

func sortSnapshots(items map[int]ClusterSnapshot) []ClusterSnapshot {
	hours := make([]int, 0, len(items))
	for hour := range items {
		hours = append(hours, hour)
	}
	sort.Ints(hours)

	snapshots := make([]ClusterSnapshot, 0, len(hours))
	for _, hour := range hours {
		snapshots = append(snapshots, items[hour])
	}
	return snapshots
}

func latestSnapshot(days []DayHistory) *ClusterSnapshot {
	for i := len(days) - 1; i >= 0; i-- {
		day := days[i]
		if len(day.Snapshots) == 0 {
			continue
		}
		latest := day.Snapshots[len(day.Snapshots)-1]
		return &latest
	}
	return nil
}
