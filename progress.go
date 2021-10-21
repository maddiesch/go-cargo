package cargo

// ProgressHandler defines the interface for listening for download progress
// updates.
type ProgressHandler interface {
	// Expected will be called once before the request begins reading the body.
	// The value passed will be the parsed value from the HTTP header field
	// 'Content-Length'. If 'Content-Length' is missing or contains an invalid
	// integer value, -1 will be given.
	Expected(int64)

	// Receive will be called every time Cargo reads data from the HTTP request.
	// The value passed will be the amount of data received for that read.
	Receive(int)
}

// ProgressHandlerFunc provides a basic ProgressHandler that will call the given
// function for each value update. The function will receive the expected number
// of bytes to read, and the total number of bytes read up to this point.
func ProgressHandlerFunc(fn func(int64, int64)) ProgressHandler {
	return &progressHandlerFuncImpl{fn: fn}
}

type progressHandlerFuncImpl struct {
	expected int64
	count    int64
	fn       func(int64, int64)
}

func (p *progressHandlerFuncImpl) Expected(i int64) {
	p.expected = i
	p.fn(p.expected, p.count)
}

func (p *progressHandlerFuncImpl) Receive(i int) {
	p.count += int64(i)
	p.fn(p.expected, p.count)
}
