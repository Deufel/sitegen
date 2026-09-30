package thing

// Add answers a plus b.
func Add(a, b int) int { return a + b }

// Counter counts.
type Counter struct{ n int }

// NewCounter starts at zero.
func NewCounter() *Counter { return &Counter{} }

// Inc adds one and answers the count.
func (c *Counter) Inc() int { c.n++; return c.n }

func Undocumented() {}
