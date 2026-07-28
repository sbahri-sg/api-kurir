package cache

import (
	"context"
	"sync"
	"time"
)

type memoryItem struct {
	value     []byte
	expiresAt time.Time
	createdAt time.Time
}

type Memory struct {
	mu         sync.Mutex
	items      map[string]memoryItem
	maxEntries int
	now        func() time.Time
}

func NewMemory(maxEntries int) *Memory {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &Memory{
		items:      make(map[string]memoryItem, maxEntries),
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

func (m *Memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	if !item.expiresAt.IsZero() && !m.now().Before(item.expiresAt) {
		delete(m.items, key)
		return nil, false, nil
	}
	return clone(item.value), true, nil
}

func (m *Memory) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	if _, exists := m.items[key]; !exists && len(m.items) >= m.maxEntries {
		m.evictOne(now)
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}
	m.items[key] = memoryItem{
		value:     clone(value),
		expiresAt: expiresAt,
		createdAt: now,
	}
	return nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.items, key)
	m.mu.Unlock()
	return nil
}

func (m *Memory) evictOne(now time.Time) {
	var oldestKey string
	var oldestAt time.Time
	for key, item := range m.items {
		if !item.expiresAt.IsZero() && !now.Before(item.expiresAt) {
			delete(m.items, key)
			return
		}
		if oldestKey == "" || item.createdAt.Before(oldestAt) {
			oldestKey = key
			oldestAt = item.createdAt
		}
	}
	if oldestKey != "" {
		delete(m.items, oldestKey)
	}
}

func clone(value []byte) []byte {
	if value == nil {
		return nil
	}
	result := make([]byte, len(value))
	copy(result, value)
	return result
}
