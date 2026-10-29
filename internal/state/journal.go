package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepRunning StepStatus = "running"
	StepDone    StepStatus = "done"
	StepFailed  StepStatus = "failed"
	StepSkipped StepStatus = "skipped"
)

type Journal struct {
	Dir   string
	RunID string
	Steps map[string]StepStatus
	mu    sync.Mutex
}

type stepsFile struct {
	Finished bool                  `json:"finished"`
	Steps    map[string]StepStatus `json:"steps"`
}

func runsDir(stateDir string) string { return filepath.Join(stateDir, "runs") }

// NewJournal cria runs/<id>/ com o plano serializado.
func NewJournal(stateDir string, now time.Time, planJSON []byte) (*Journal, error) {
	base := now.Format("20060102-150405")
	id := base
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(runsDir(stateDir), id)); errors.Is(err, fs.ErrNotExist) {
			break
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
	j := &Journal{Dir: filepath.Join(runsDir(stateDir), id), RunID: id, Steps: map[string]StepStatus{}}
	if err := os.MkdirAll(filepath.Join(j.Dir, "logs"), 0o755); err != nil {
		return nil, err
	}
	if err := WriteFileAtomic(filepath.Join(j.Dir, "plan.json"), planJSON, 0o644); err != nil {
		return nil, err
	}
	return j, j.persist(false)
}

func (j *Journal) Set(stepID string, s StepStatus) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Steps[stepID] = s
	return j.persistLocked(false)
}

func (j *Journal) Get(stepID string) StepStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.Steps[stepID]
}

func (j *Journal) Finish() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.persistLocked(true)
}

func (j *Journal) persist(finished bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.persistLocked(finished)
}

func (j *Journal) persistLocked(finished bool) error {
	return writeJSONAtomic(filepath.Join(j.Dir, "steps.json"), stepsFile{Finished: finished, Steps: j.Steps})
}

// LogWriter abre (em modo append) o log de um item neste run.
func (j *Journal) LogWriter(itemID string) (io.WriteCloser, error) {
	return os.OpenFile(filepath.Join(j.Dir, "logs", itemID+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

func (j *Journal) LogPath(itemID string) string { return filepath.Join(j.Dir, "logs", itemID+".log") }

// LatestUnfinished devolve o run mais recente que não terminou, com o plano salvo.
func LatestUnfinished(stateDir string) (*Journal, []byte, error) {
	entries, err := os.ReadDir(runsDir(stateDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name() > entries[b].Name() })
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(runsDir(stateDir), e.Name())
		data, err := os.ReadFile(filepath.Join(dir, "steps.json"))
		if err != nil {
			continue
		}
		var sf stepsFile
		if json.Unmarshal(data, &sf) != nil || sf.Finished {
			return nil, nil, nil
		}
		plan, err := os.ReadFile(filepath.Join(dir, "plan.json"))
		if err != nil {
			return nil, nil, err
		}
		if sf.Steps == nil {
			sf.Steps = map[string]StepStatus{}
		}
		return &Journal{Dir: dir, RunID: e.Name(), Steps: sf.Steps}, plan, nil
	}
	return nil, nil, nil
}

// Runs lista os ids dos runs, do mais recente ao mais antigo.
func Runs(stateDir string) ([]string, error) {
	entries, err := os.ReadDir(runsDir(stateDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids, nil
}
