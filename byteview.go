package geecache

// ByteView 保存字节的不可变视图。
type ByteView struct {
	b []byte
}

// Len 返回视图的长度
func (v ByteView) Len() int64 {
	return int64(len(v.b))
}

//复制函数cloneBytes
func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// ByteSlice 返回数据的字节切片副本
func (v ByteView) ByteSlice() []byte {
	return cloneBytes(v.b)
}

// String 将数据作为字符串返回，必要时进行复制
func (v ByteView) String() string {
	return string(v.b)
}
