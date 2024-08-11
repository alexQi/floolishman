package model

type RingBuffer struct {
	data   []float64
	size   int
	cursor int
	count  int
}

// NewRingBuffer 创建一个新的固定长度的循环数组
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		data:   make([]float64, size),
		size:   size,
		cursor: 0,
		count:  0,
	}
}

// Add 向循环数组添加新数据
func (rb *RingBuffer) Add(value float64) {
	if rb.count < rb.size {
		rb.count++
	}
	rb.data[rb.cursor] = value
	rb.cursor = (rb.cursor + 1) % rb.size
}

// GetAll 返回数组中的所有元素，顺序是从最旧的元素到最新的元素
func (rb *RingBuffer) Count() int {
	return rb.count
}

// GetAll 返回数组中的所有元素，顺序是从最旧的元素到最新的元素
func (rb *RingBuffer) GetAll() []float64 {
	if rb.count == rb.size {
		return append(rb.data[rb.cursor:], rb.data[:rb.cursor]...)
	}
	return rb.data[:rb.count]
}

// GetFirst 获取当前数组的开头元素（最旧的元素）
func (rb *RingBuffer) GetFirst() float64 {
	if rb.count == 0 {
		return -1 // 表示数组为空
	}
	return rb.data[rb.cursor%rb.count]
}

// GetLast 获取当前数组的结尾元素（最新的元素）
func (rb *RingBuffer) GetLast() float64 {
	if rb.count == 0 {
		return -1 // 表示数组为空
	}
	lastIndex := (rb.cursor - 1 + rb.size) % rb.size
	return rb.data[lastIndex]
}
