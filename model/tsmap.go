package model

import (
	"sync"
)

type lockShard[K comparable] struct {
	mu sync.RWMutex
	m  map[K]*sync.RWMutex
}

type ThreadSafeMap[K comparable, V any] struct {
	data    map[K]V
	lockMap lockShard[K]
}

func NewThreadSafeMap[K comparable, V any]() *ThreadSafeMap[K, V] {
	return &ThreadSafeMap[K, V]{
		data: make(map[K]V),
		lockMap: lockShard[K]{
			m: make(map[K]*sync.RWMutex),
		},
	}
}

// getLock 返回指定键的锁，若不存在则创建新锁
func (tsm *ThreadSafeMap[K, V]) getLock(key K) *sync.RWMutex {
	tsm.lockMap.mu.Lock()
	defer tsm.lockMap.mu.Unlock()

	if lock, exists := tsm.lockMap.m[key]; exists {
		return lock
	}

	lock := &sync.RWMutex{}
	tsm.lockMap.m[key] = lock
	return lock
}

// Set 设置键值对
func (tsm *ThreadSafeMap[K, V]) Set(key K, value V) {
	lock := tsm.getLock(key)
	lock.Lock()
	defer lock.Unlock()

	tsm.data[key] = value
}

// Get 获取键对应的值
func (tsm *ThreadSafeMap[K, V]) Get(key K) (V, bool) {
	lock := tsm.getLock(key)
	lock.RLock()
	defer lock.RUnlock()

	value, ok := tsm.data[key]
	return value, ok
}

// Delete 删除键
func (tsm *ThreadSafeMap[K, V]) Delete(key K) {
	lock := tsm.getLock(key)
	lock.Lock()
	defer lock.Unlock()

	delete(tsm.data, key)
	delete(tsm.lockMap.m, key)
}

// Len 返回 map 的长度
func (tsm *ThreadSafeMap[K, V]) Len() int {
	tsm.lockMap.mu.RLock()
	defer tsm.lockMap.mu.RUnlock()

	return len(tsm.data)
}

// Exists 检查键是否存在
func (tsm *ThreadSafeMap[K, V]) Exists(key K) bool {
	lock := tsm.getLock(key)
	lock.RLock()
	defer lock.RUnlock()

	_, ok := tsm.data[key]
	return ok
}
