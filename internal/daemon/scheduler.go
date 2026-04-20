package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SchedulerInfo describes a running scheduler and its current state.
// Persisted as JSON alongside a PID file under ~/.auralens/schedulers/<name>/.
type SchedulerInfo struct {
	Name            string     `json:"name"`
	Engine          string     `json:"engine"`
	EnginePath      string     `json:"engine_path,omitempty"`
	EngineModel     string     `json:"engine_model,omitempty"`
	Prompt          string     `json:"prompt,omitempty"`
	SkillFile       string     `json:"skill_file,omitempty"`
	WorkDir         string     `json:"work_dir,omitempty"`
	IntervalSecs    int        `json:"interval_secs"`
	PID             int        `json:"pid"`
	StartedAt       time.Time  `json:"started_at"`
	Round           int        `json:"round"`
	LastCompletedAt *time.Time `json:"last_completed_at,omitempty"`
	// CurrentWorkerID is non-empty while a worker round is actively running.
	CurrentWorkerID string `json:"current_worker_id,omitempty"`
}

// SchedulerState manages the on-disk state for a single named scheduler.
type SchedulerState struct {
	dir  string
	name string
}

func schedulersBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".auralens", "schedulers"), nil
}

// NewSchedulerState creates (and ensures the directory for) a named scheduler.
func NewSchedulerState(name string) (*SchedulerState, error) {
	base, err := schedulersBaseDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &SchedulerState{dir: dir, name: name}, nil
}

func (s *SchedulerState) pidPath() string  { return filepath.Join(s.dir, "scheduler.pid") }
func (s *SchedulerState) infoPath() string { return filepath.Join(s.dir, "scheduler.json") }
func (s *SchedulerState) logPath() string  { return filepath.Join(s.dir, "scheduler.log") }

// LogPath returns the absolute path to the scheduler log file.
func (s *SchedulerState) LogPath() string { return s.logPath() }

// Name returns the scheduler's name.
func (s *SchedulerState) Name() string { return s.name }

func (s *SchedulerState) WritePID(pid int) error {
	return os.WriteFile(s.pidPath(), []byte(strconv.Itoa(pid)), 0600)
}

func (s *SchedulerState) ReadPID() (int, error) {
	data, err := os.ReadFile(s.pidPath())
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid file: %w", err)
	}
	return pid, nil
}

func (s *SchedulerState) ClearPID() error {
	err := os.Remove(s.pidPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsRunning checks if the scheduler process is still alive.
func (s *SchedulerState) IsRunning() (bool, int, error) {
	pid, err := s.ReadPID()
	if err != nil {
		return false, 0, err
	}
	if pid == 0 {
		return false, 0, nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0, nil
	}
	if err := processAlive(proc); err != nil {
		return false, pid, nil
	}
	return true, pid, nil
}

// WriteInfo persists the scheduler metadata and runtime state.
func (s *SchedulerState) WriteInfo(info *SchedulerInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.infoPath(), data, 0600)
}

// ReadInfo loads the scheduler metadata from disk.
func (s *SchedulerState) ReadInfo() (*SchedulerInfo, error) {
	data, err := os.ReadFile(s.infoPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var info SchedulerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// OpenLog opens the scheduler log in append mode.
func (s *SchedulerState) OpenLog() (*os.File, error) {
	return os.OpenFile(s.logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
}

// Cleanup removes the scheduler's state directory.
func (s *SchedulerState) Cleanup() error {
	return os.RemoveAll(s.dir)
}

// ListSchedulers returns SchedulerState entries for all known schedulers.
func ListSchedulers() ([]*SchedulerState, error) {
	base, err := schedulersBaseDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var schedulers []*SchedulerState
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		schedulers = append(schedulers, &SchedulerState{
			dir:  filepath.Join(base, e.Name()),
			name: e.Name(),
		})
	}
	return schedulers, nil
}
