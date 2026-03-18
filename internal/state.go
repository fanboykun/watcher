package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DeployStatus string

const (
	StatusUnknown   DeployStatus = "unknown"
	StatusHealthy   DeployStatus = "healthy"
	StatusDeploying DeployStatus = "deploying"
	StatusFailed    DeployStatus = "failed"
	StatusRollback  DeployStatus = "rollback"
)

// State is the on-disk deployment state written to state.json
type State struct {
	CurrentVersion string       `json:"current_version"`
	Status         DeployStatus `json:"status"`
	LastChecked    string       `json:"last_checked"`
	LastDeployed   string       `json:"last_deployed,omitempty"`
	LastError      string       `json:"last_error,omitempty"`
}

type StateManager struct {
	installDir string
	log        *Logger
}

func NewStateManager(installDir string, log *Logger) *StateManager {
	return &StateManager{installDir: installDir, log: log}
}

func (s *StateManager) statePath() string {
	return filepath.Join(s.installDir, "state.json")
}

func (s *StateManager) versionPath() string {
	return filepath.Join(s.installDir, "version.txt")
}

// ReadVersion returns the currently deployed version, or "" if not yet deployed
func (s *StateManager) ReadVersion() (string, error) {
	data, err := os.ReadFile(s.versionPath())
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read version.txt: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *StateManager) WriteVersion(version string) error {
	return atomicWrite(s.versionPath(), []byte(version+"\n"))
}

func (s *StateManager) ReadState() (*State, error) {
	data, err := os.ReadFile(s.statePath())
	if os.IsNotExist(err) {
		return &State{Status: StatusUnknown}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state.json: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("decode state.json: %w", err)
	}
	return &st, nil
}

func (s *StateManager) writeState(st *State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	return atomicWrite(s.statePath(), data)
}

func (s *StateManager) SetChecked() error {
	st, err := s.ReadState()
	if err != nil {
		return err
	}
	st.LastChecked = now()
	return s.writeState(st)
}

func (s *StateManager) SetDeploying(version string) error {
	st, err := s.ReadState()
	if err != nil {
		return err
	}
	st.Status = StatusDeploying
	st.CurrentVersion = version
	st.LastChecked = now()
	st.LastError = ""
	return s.writeState(st)
}

func (s *StateManager) SetHealthy(version string) error {
	st, err := s.ReadState()
	if err != nil {
		return err
	}
	st.Status = StatusHealthy
	st.CurrentVersion = version
	st.LastDeployed = now()
	st.LastError = ""
	return s.writeState(st)
}

func (s *StateManager) SetFailed(errMsg string) error {
	st, err := s.ReadState()
	if err != nil {
		return err
	}
	st.Status = StatusFailed
	st.LastError = errMsg
	return s.writeState(st)
}

func (s *StateManager) SetRolledBack(version string) error {
	st, err := s.ReadState()
	if err != nil {
		return err
	}
	st.Status = StatusRollback
	st.CurrentVersion = version
	st.LastDeployed = now()
	return s.writeState(st)
}

// atomicWrite writes data to path via tmp + rename to avoid partial writes
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("ensure dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}