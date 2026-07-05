package job

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Store persists job metadata as JSON files under the user's config directory.
type Store struct {
	dir string
}

// NewStore returns the default job metadata store.
func NewStore() (*Store, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewStoreAt(filepath.Join(configDir, "thenn", "jobs")), nil
}

// NewStoreAt returns a store rooted at dir. It is primarily useful for tests.
func NewStoreAt(dir string) *Store {
	return &Store{dir: dir}
}

// Dir returns the directory where metadata files are stored.
func (s *Store) Dir() string {
	return s.dir
}

// Path returns the metadata path for label.
func (s *Store) Path(label string) (string, error) {
	if err := ValidateLabel(label); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, label+".json"), nil
}

// Save writes metadata to <config>/thenn/jobs/<label>.json.
func (s *Store) Save(metadata Metadata) error {
	if err := metadata.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path, err := s.Path(metadata.Label)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, metadata.Label+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Load reads metadata for label.
func (s *Store) Load(label string) (Metadata, error) {
	path, err := s.Path(label)
	if err != nil {
		return Metadata{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, err
	}
	if err := metadata.Validate(); err != nil {
		return Metadata{}, fmt.Errorf("invalid job metadata %s: %w", path, err)
	}
	return metadata, nil
}

// Delete removes metadata for label.
func (s *Store) Delete(label string) error {
	path, err := s.Path(label)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// List returns all valid job metadata files sorted by label.
func (s *Store) List() ([]Metadata, error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	jobs := make([]Metadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		label := entry.Name()[:len(entry.Name())-len(".json")]
		metadata, err := s.Load(label)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, metadata)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Label < jobs[j].Label
	})
	return jobs, nil
}
