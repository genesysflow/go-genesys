package support

// Pipeline passes a value through a series of stages, stopping at the
// first failure - Laravel's Pipeline, for the cases where a transform is
// a list of steps rather than one function:
//
//	body, err := support.Pipe(raw).
//	    Through(normalize, sanitize, validate).
//	    Run()
type Pipeline[T any] struct {
	value  T
	stages []func(T) (T, error)
}

// Pipe starts a pipeline over a value.
func Pipe[T any](value T) *Pipeline[T] {
	return &Pipeline[T]{value: value}
}

// Through adds stages, run in the order given.
func (p *Pipeline[T]) Through(stages ...func(T) (T, error)) *Pipeline[T] {
	p.stages = append(p.stages, stages...)
	return p
}

// Run passes the value through every stage. A stage that fails stops the
// pipeline: the stages after it never see a value the failing stage did
// not produce.
func (p *Pipeline[T]) Run() (T, error) {
	value := p.value
	for _, stage := range p.stages {
		next, err := stage(value)
		if err != nil {
			var zero T
			return zero, err
		}
		value = next
	}
	return value, nil
}

// Then runs the pipeline and hands the result to a final function.
func (p *Pipeline[T]) Then(fn func(T) error) error {
	value, err := p.Run()
	if err != nil {
		return err
	}
	return fn(value)
}
