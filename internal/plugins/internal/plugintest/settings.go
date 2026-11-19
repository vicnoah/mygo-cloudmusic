package plugintest

import (
	"encoding/json"
	"sync"
)

// Settings is an in-memory instance settings store.
type Settings struct {
	mu     sync.Mutex
	data   []byte
	onSave func()
}

func NewSettings(initial any, onSave func()) *Settings {
	store := &Settings{onSave: onSave}
	if initial != nil {
		data, err := json.Marshal(initial)
		if err != nil {
			panic(err)
		}
		store.data = data
	}
	return store
}

func (s *Settings) Load(dst any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		return nil
	}
	return json.Unmarshal(s.data, dst)
}

func (s *Settings) Save(src any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.data = data
	s.mu.Unlock()
	if s.onSave != nil {
		s.onSave()
	}
	return nil
}
