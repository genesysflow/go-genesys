package queue

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// registry maps job names to factories so queued payloads can be
// deserialized back into concrete job types by workers.
var (
	registryMu sync.RWMutex
	factories  = make(map[string]func() Job)
	typeNames  = make(map[reflect.Type]string)
)

// RegisterJob registers a job factory under an explicit name. Workers must
// register every job type they process before starting.
func RegisterJob(name string, factory func() Job) {
	registryMu.Lock()
	defer registryMu.Unlock()
	factories[name] = factory
	typeNames[reflect.TypeOf(factory())] = name
}

// Register registers a job type using its JobName() or reflect-derived name:
//
//	queue.Register[SendEmailJob]()
func Register[T any]() {
	var probe T
	job, ok := any(&probe).(Job)
	if !ok {
		panic(fmt.Sprintf("queue: *%T does not implement queue.Job", probe))
	}
	RegisterJob(nameForJob(job), func() Job {
		var instance T
		return any(&instance).(Job)
	})
}

// nameForJob resolves the stable name for a job instance: JobName()
// first, then a name given to RegisterJob, then the reflect-derived
// package path.
func nameForJob(job Job) string {
	if named, ok := job.(Nameable); ok {
		return named.JobName()
	}
	registryMu.RLock()
	name, ok := typeNames[reflect.TypeOf(job)]
	registryMu.RUnlock()
	if ok {
		return name
	}
	t := reflect.TypeOf(job)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.PkgPath() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.Name()
}

// payload is the serialized envelope stored by queue drivers.
type payload struct {
	Job  string          `json:"job"`
	Data json.RawMessage `json:"data"`
}

// marshalJob serializes a job with its registered name.
func marshalJob(job Job) (name string, body []byte, err error) {
	name = nameForJob(job)
	data, err := json.Marshal(job)
	if err != nil {
		return "", nil, fmt.Errorf("queue: cannot serialize job %s: %w", name, err)
	}
	body, err = json.Marshal(payload{Job: name, Data: data})
	return name, body, err
}

// unmarshalJob reconstructs a job from a stored payload envelope.
func unmarshalJob(body []byte) (Job, string, error) {
	var env payload
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, "", fmt.Errorf("queue: corrupt payload: %w", err)
	}
	job, err := makeJob(env.Job, env.Data)
	return job, env.Job, err
}

// makeJob instantiates a registered job and fills it from JSON data.
func makeJob(name string, data []byte) (Job, error) {
	registryMu.RLock()
	factory, ok := factories[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("queue: job %q is not registered - call queue.Register before starting the worker", name)
	}
	job := factory()
	if len(data) > 0 {
		if err := json.Unmarshal(data, job); err != nil {
			return nil, fmt.Errorf("queue: cannot deserialize job %q: %w", name, err)
		}
	}
	return job, nil
}
