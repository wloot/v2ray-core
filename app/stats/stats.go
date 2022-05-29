package stats

//go:generate go run github.com/v2fly/v2ray-core/v5/common/errors/errorgen

import (
	"context"
	"sync"
	"time"

	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/common/errors"
	"github.com/v2fly/v2ray-core/v5/features/stats"
)

type Mapper struct {
	kv map[string](struct {
		up     int64
		down   int64
		stable int64
	})
	mutex sync.Mutex
}

func (m *Mapper) Add(k string, up int64, down int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	now := time.Now().Unix()

	var u int64
	var d int64
	var stable int64
	if v, ok := m.kv[k]; ok {
		u = v.up
		d = v.down
		stable = v.stable
	}
	if stable > 1 && stable <= now-20 {
		if stable >= now-90 {
			stable = 1
		} else {
			stable = now
		}
	}
	if stable == 0 {
		stable = now
	}
	m.kv[k] = struct {
		up     int64
		down   int64
		stable int64
	}{
		up:     u + up,
		down:   d + down,
		stable: stable,
	}
}

func (m *Mapper) GetKeys() []string {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var keys []string
	nonstable := false
	for k, v := range m.kv {
		if v.stable != 1 {
			nonstable = true
			continue
		}
		keys = append(keys, k)
	}
	if nonstable == true {
		keys = append(keys, "")
	}
	return keys
}

func (m *Mapper) GetVaule(key string) (int64, int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	now := time.Now().Unix()

	if key == "" {
		var up int64
		var down int64
		for k, v := range m.kv {
			if v.stable == 1 {
				continue
			}
			up += v.up
			down += v.down
			if v.stable < now-90 {
				delete(m.kv, k)
				continue
			}
			m.kv[k] = struct {
				up     int64
				down   int64
				stable int64
			}{
				up:     0,
				down:   0,
				stable: v.stable,
			}
		}
		return up, down
	}

	var up int64
	var down int64
	if v, ok := m.kv[key]; ok {
		up = v.up
		down = v.down
		delete(m.kv, key)
	}
	return up, down
}

func (m *Manager) RegisterMapper(name string) (stats.Mapper, error) {
	m.access.Lock()
	defer m.access.Unlock()

	if _, found := m.maps[name]; found {
		return nil, newError("Mapper ", name, " already registered.")
	}
	newError("create new mapper ", name).AtDebug().WriteToLog()
	mapper := new(Mapper)
	mapper.kv = make(map[string](struct {
		up     int64
		down   int64
		stable int64
	}))
	m.maps[name] = mapper
	return mapper, nil
}

func (m *Manager) UnregisterMapper(name string) error {
	m.access.Lock()
	defer m.access.Unlock()

	if _, found := m.maps[name]; found {
		newError("remove mapper ", name).AtDebug().WriteToLog()
		delete(m.maps, name)
	}
	return nil
}

func (m *Manager) GetMapper(name string) stats.Mapper {
	m.access.RLock()
	defer m.access.RUnlock()

	if mapper, found := m.maps[name]; found {
		return mapper
	}
	return nil
}

// Manager is an implementation of stats.Manager.
type Manager struct {
	access   sync.RWMutex
	counters map[string]*Counter
	channels map[string]*Channel
	maps     map[string]*Mapper
	running  bool
}

// NewManager creates an instance of Statistics Manager.
func NewManager(ctx context.Context, config *Config) (*Manager, error) {
	m := &Manager{
		counters: make(map[string]*Counter),
		channels: make(map[string]*Channel),
		maps:     make(map[string]*Mapper),
	}

	return m, nil
}

// Type implements common.HasType.
func (*Manager) Type() interface{} {
	return stats.ManagerType()
}

// RegisterCounter implements stats.Manager.
func (m *Manager) RegisterCounter(name string) (stats.Counter, error) {
	m.access.Lock()
	defer m.access.Unlock()

	if _, found := m.counters[name]; found {
		return nil, newError("Counter ", name, " already registered.")
	}
	newError("create new counter ", name).AtDebug().WriteToLog()
	c := new(Counter)
	m.counters[name] = c
	return c, nil
}

// UnregisterCounter implements stats.Manager.
func (m *Manager) UnregisterCounter(name string) error {
	m.access.Lock()
	defer m.access.Unlock()

	if _, found := m.counters[name]; found {
		newError("remove counter ", name).AtDebug().WriteToLog()
		delete(m.counters, name)
	}
	return nil
}

// GetCounter implements stats.Manager.
func (m *Manager) GetCounter(name string) stats.Counter {
	m.access.RLock()
	defer m.access.RUnlock()

	if c, found := m.counters[name]; found {
		return c
	}
	return nil
}

// VisitCounters calls visitor function on all managed counters.
func (m *Manager) VisitCounters(visitor func(string, stats.Counter) bool) {
	m.access.RLock()
	defer m.access.RUnlock()

	for name, c := range m.counters {
		if !visitor(name, c) {
			break
		}
	}
}

// RegisterChannel implements stats.Manager.
func (m *Manager) RegisterChannel(name string) (stats.Channel, error) {
	m.access.Lock()
	defer m.access.Unlock()

	if _, found := m.channels[name]; found {
		return nil, newError("Channel ", name, " already registered.")
	}
	newError("create new channel ", name).AtDebug().WriteToLog()
	c := NewChannel(&ChannelConfig{BufferSize: 64, Blocking: false})
	m.channels[name] = c
	if m.running {
		return c, c.Start()
	}
	return c, nil
}

// UnregisterChannel implements stats.Manager.
func (m *Manager) UnregisterChannel(name string) error {
	m.access.Lock()
	defer m.access.Unlock()

	if c, found := m.channels[name]; found {
		newError("remove channel ", name).AtDebug().WriteToLog()
		delete(m.channels, name)
		return c.Close()
	}
	return nil
}

// GetChannel implements stats.Manager.
func (m *Manager) GetChannel(name string) stats.Channel {
	m.access.RLock()
	defer m.access.RUnlock()

	if c, found := m.channels[name]; found {
		return c
	}
	return nil
}

// Start implements common.Runnable.
func (m *Manager) Start() error {
	m.access.Lock()
	defer m.access.Unlock()
	m.running = true
	errs := []error{}
	for _, channel := range m.channels {
		if err := channel.Start(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return errors.Combine(errs...)
	}
	return nil
}

// Close implement common.Closable.
func (m *Manager) Close() error {
	m.access.Lock()
	defer m.access.Unlock()
	m.running = false
	errs := []error{}
	for name, channel := range m.channels {
		newError("remove channel ", name).AtDebug().WriteToLog()
		delete(m.channels, name)
		if err := channel.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return errors.Combine(errs...)
	}
	return nil
}

func init() {
	common.Must(common.RegisterConfig((*Config)(nil), func(ctx context.Context, config interface{}) (interface{}, error) {
		return NewManager(ctx, config.(*Config))
	}))
}
