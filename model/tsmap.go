package model

import (
	"sync"
)

type ThreadSafeMap[K comparable, V any] struct {
	data sync.Map
}

// NewThreadSafeMap 创建一个新的 ThreadSafeMap
func NewThreadSafeMap[K comparable, V any]() *ThreadSafeMap[K, V] {
	return &ThreadSafeMap[K, V]{}
}

// Set 设置键值对
func (tsm *ThreadSafeMap[K, V]) Set(key K, value V) {
	tsm.data.Store(key, value)
}

// Get 获取键对应的值
func (tsm *ThreadSafeMap[K, V]) Get(key K) (V, bool) {
	value, ok := tsm.data.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	return value.(V), true
}

// Delete 删除键
func (tsm *ThreadSafeMap[K, V]) Delete(key K) {
	tsm.data.Delete(key)
}

// Len 返回 map 的长度
func (tsm *ThreadSafeMap[K, V]) Len() int {
	length := 0
	tsm.data.Range(func(_, _ any) bool {
		length++
		return true
	})
	return length
}

// Exists 检查键是否存在
func (tsm *ThreadSafeMap[K, V]) Exists(key K) bool {
	_, ok := tsm.data.Load(key)
	return ok
}
