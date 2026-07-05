package job

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestStoreSaveLoadListDelete(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	now := time.Date(2026, 7, 5, 12, 30, 0, 0, time.UTC)
	schedule, err := NewParsedSchedule("Mon..Fri 09:00")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := NewMetadata("weekday-report", "weekdays at 9", schedule, []string{"notify-send", "report ready"}, "/tmp", now)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Save(metadata); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	path, err := store.Path(metadata.Label)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(store.Dir(), "weekday-report.json") {
		t.Fatalf("Path() = %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want 0600", info.Mode().Perm())
	}

	loaded, err := store.Load(metadata.Label)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(loaded, metadata) {
		t.Fatalf("Load() = %#v, want %#v", loaded, metadata)
	}

	jobs, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !reflect.DeepEqual(jobs, []Metadata{metadata}) {
		t.Fatalf("List() = %#v, want %#v", jobs, []Metadata{metadata})
	}

	if err := store.Delete(metadata.Label); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("metadata file still exists after Delete(): %v", err)
	}
}

func TestLabelValidationAndSanitization(t *testing.T) {
	valid := []string{"backup", "backup-1", "Backup_1", "report.daily"}
	for _, label := range valid {
		t.Run("valid/"+label, func(t *testing.T) {
			if err := ValidateLabel(label); err != nil {
				t.Fatalf("ValidateLabel(%q) error = %v", label, err)
			}
		})
	}

	invalid := []string{"", " backup", "backup ", "../backup", "backup/report", "-backup", ".backup"}
	for _, label := range invalid {
		t.Run("invalid/"+label, func(t *testing.T) {
			if err := ValidateLabel(label); err == nil {
				t.Fatalf("ValidateLabel(%q) succeeded, want error", label)
			}
		})
	}

	tests := map[string]string{
		" Daily Backup! ": "daily-backup",
		"...":             "job",
		"A/B C":           "a-b-c",
	}
	for input, want := range tests {
		t.Run("sanitize/"+input, func(t *testing.T) {
			if got := SanitizeLabel(input); got != want {
				t.Fatalf("SanitizeLabel(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestStoreRejectsUnsafeLabelPath(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if _, err := store.Path("../escape"); err == nil {
		t.Fatal("Path() succeeded for unsafe label, want error")
	}
}
