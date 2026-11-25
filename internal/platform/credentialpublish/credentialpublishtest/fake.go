// Package credentialpublishtest is an in-memory Parameter Store for tests of
// credential publishing (no test calls AWS). It models what the publisher
// depends on: names, values, types, tags, ParameterAlreadyExists, the refusal
// of tags together with Overwrite, ParameterNotFound (InvalidResourceId for a
// tag read), and failures a test injects for a path.
package credentialpublishtest

import (
	"context"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// Param is a stored parameter.
type Param struct {
	Value string
	Type  ssmtypes.ParameterType
	KeyID string
	Tags  map[string]string
}

// SSM is the fake credentialpublish.SSMWriteAPI.
type SSM struct {
	mu       sync.Mutex
	params   map[string]*Param
	failures map[string]error
	// Puts and Deletes count the calls that reached the store (a failure
	// injected for the path is not counted).
	Puts, Deletes int
}

// New is an empty store.
func New() *SSM {
	return &SSM{params: map[string]*Param{}, failures: map[string]error{}}
}

// Err is an AWS API error with the code the real client would carry.
func Err(code, message string) error {
	return &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultServer}
}

// FailPath makes every call for path fail with err until ClearFailure.
func (f *SSM) FailPath(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[path] = err
}

// ClearFailure removes the failure injected for path.
func (f *SSM) ClearFailure(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.failures, path)
}

// Seed creates a parameter as something other than FlowCatalyst did (or, with
// tags, as it did).
func (f *SSM) Seed(path, value string, tags map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.params[path] = &Param{Value: value, Type: ssmtypes.ParameterTypeString, Tags: tags}
}

// Get is the parameter at path.
func (f *SSM) Get(path string) (Param, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.params[path]
	if !ok {
		return Param{}, false
	}
	return *p, true
}

// Names are the stored parameter names, sorted.
func (f *SSM) Names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.params))
	for n := range f.params {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (f *SSM) PutParameter(_ context.Context, in *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := aws.ToString(in.Name)
	if err := f.failures[name]; err != nil {
		return nil, err
	}
	overwrite := aws.ToBool(in.Overwrite)
	if overwrite && len(in.Tags) > 0 {
		return nil, Err("InvalidParameters", "tags and overwrite can't be used together")
	}
	cur, exists := f.params[name]
	if exists && !overwrite {
		return nil, Err("ParameterAlreadyExists", "The parameter already exists.")
	}
	f.Puts++
	p := &Param{Value: aws.ToString(in.Value), Type: in.Type, KeyID: aws.ToString(in.KeyId), Tags: map[string]string{}}
	if exists {
		p.Tags = cur.Tags
	}
	for _, t := range in.Tags {
		p.Tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	f.params[name] = p
	return &ssm.PutParameterOutput{Version: 1}, nil
}

func (f *SSM) DeleteParameter(_ context.Context, in *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := aws.ToString(in.Name)
	if err := f.failures[name]; err != nil {
		return nil, err
	}
	if _, ok := f.params[name]; !ok {
		return nil, Err("ParameterNotFound", "Parameter not found.")
	}
	f.Deletes++
	delete(f.params, name)
	return &ssm.DeleteParameterOutput{}, nil
}

func (f *SSM) ListTagsForResource(_ context.Context, in *ssm.ListTagsForResourceInput, _ ...func(*ssm.Options)) (*ssm.ListTagsForResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := aws.ToString(in.ResourceId)
	if err := f.failures[name]; err != nil {
		return nil, err
	}
	p, ok := f.params[name]
	if !ok {
		return nil, Err("InvalidResourceId", "The resource ID is not valid.")
	}
	out := &ssm.ListTagsForResourceOutput{}
	for k, v := range p.Tags {
		out.TagList = append(out.TagList, ssmtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return out, nil
}
