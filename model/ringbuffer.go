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

// First 获取当前数组的开头元素（最旧的元素）
func (rb *RingBuffer) First() float64 {
	if rb.count == 0 {
		return -1 // 表示数组为空
	}
	return rb.data[rb.cursor%rb.count]
}

// GetPrevious 获取当前索引往前指定 n 的值
func (rb *RingBuffer) Last(n int) float64 {
	if n < 0 || n >= rb.count {
		return -1 // 表示索引无效
	}
	index := (rb.cursor - 1 - n + rb.size) % rb.size
	return rb.data[index]
}

// Clear 清空循环数组中的所有元素
func (rb *RingBuffer) Clear() {
	rb.data = make([]float64, rb.size)
	rb.cursor = 0
	rb.count = 0
}
