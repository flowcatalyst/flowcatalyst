package secretref

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAWS stands in for both clients: no test calls AWS.
type fakeAWS struct {
	mu      sync.Mutex
	secrets map[string]string
	params  map[string]string
	err     error // returned by every call while set
	calls   atomic.Int64
	gate    chan struct{} // when set, calls block until it is closed
	regions []string
}

func newFake() *fakeAWS {
	return &fakeAWS{secrets: map[string]string{}, params: map[string]string{}}
}

func (f *fakeAWS) SecretsManager(_ context.Context, region string) (SecretsManagerAPI, error) {
	f.mu.Lock()
	f.regions = append(f.regions, region)
	f.mu.Unlock()
	return f, nil
}

func (f *fakeAWS) SSM(_ context.Context, region string) (SSMAPI, error) {
	f.mu.Lock()
	f.regions = append(f.regions, region)
	f.mu.Unlock()
	return f, nil
}

func (f *fakeAWS) wait() error {
	f.calls.Add(1)
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *fakeAWS) GetSecretValue(_ context.Context, in *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if err := f.wait(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.secrets[*in.SecretId]
	if !ok {
		return nil, &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "no such secret"}
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: &v}, nil
}

func (f *fakeAWS) GetParameter(_ context.Context, in *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if err := f.wait(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.params[*in.Name]
	if !ok {
		return nil, &smithy.GenericAPIError{Code: "ParameterNotFound", Message: "no such parameter"}
	}
	if in.WithDecryption == nil || !*in.WithDecryption {
		return nil, errors.New("WithDecryption must be true")
	}
	return &ssm.GetParameterOutput{Parameter: &ssmtypes.Parameter{Value: &v}}, nil
}

func (f *fakeAWS) setErr(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

// clock is a settable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newSvc(allow string, f *fakeAWS) (*Service, *clock) {
	s := New(Config{Allow: splitAllow(allow), TTL: 5 * time.Minute, MaxStale: time.Hour, NegativeTTL: 30 * time.Second}, f)
	c := &clock{t: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	s.now = c.now
	return s, c
}

const allowAll = "aws-sm://prod/,aws-ps:///inhance/,aws-sm://arn:aws:secretsmanager:"

func TestResolveSecretsManagerWhole(t *testing.T) {
	f := newFake()
	f.secrets["prod/db"] = "hunter2"
	s, _ := newSvc(allowAll, f)
	v, err := s.Resolve(context.Background(), "aws-sm://prod/db")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", v)
}

func TestResolveSecretsManagerKey(t *testing.T) {
	f := newFake()
	f.secrets["prod/db"] = `{"user":"u","port":5432,"tls":true,"nothing":null,"obj":{"a":1},"arr":[1]}`
	s, _ := newSvc(allowAll, f)
	ctx := context.Background()

	for ref, want := range map[string]string{
		"aws-sm://prod/db#user": "u",
		"aws-sm://prod/db#port": "5432",
		"aws-sm://prod/db#tls":  "true",
	} {
		v, err := s.Resolve(ctx, ref)
		require.NoError(t, err, ref)
		assert.Equal(t, want, v, ref)
	}
	for _, ref := range []string{"aws-sm://prod/db#missing", "aws-sm://prod/db#nothing", "aws-sm://prod/db#obj", "aws-sm://prod/db#arr", "aws-sm://prod/db#"} {
		_, err := s.Resolve(ctx, ref)
		assert.Equal(t, CodeKeyMissing, CodeOf(err), ref)
	}
	// One AWS call served every key: the whole secret is cached once per name.
	assert.EqualValues(t, 1, f.calls.Load())

	f.secrets["prod/plain"] = "not json"
	_, err := s.Resolve(ctx, "aws-sm://prod/plain#user")
	assert.Equal(t, CodeKeyMissing, CodeOf(err))
}

func TestResolveParameterStore(t *testing.T) {
	f := newFake()
	f.params["/inhance/np/x"] = "param-value"
	s, _ := newSvc(allowAll, f)
	v, err := s.Resolve(context.Background(), "aws-ps:///inhance/np/x")
	require.NoError(t, err)
	assert.Equal(t, "param-value", v)

	_, err = s.Resolve(context.Background(), "aws-ps:///inhance/np/missing")
	assert.Equal(t, CodeNotFound, CodeOf(err))
}

func TestResolveARNRegion(t *testing.T) {
	f := newFake()
	arn := "arn:aws:secretsmanager:eu-west-1:123456789012:secret:prod/db-AbCdEf"
	f.secrets[arn] = "v"
	s, _ := newSvc(allowAll, f)
	_, err := s.Resolve(context.Background(), "aws-sm://"+arn)
	require.NoError(t, err)
	assert.Equal(t, []string{"eu-west-1"}, f.regions)

	f.secrets["prod/db"] = "v"
	_, err = s.Resolve(context.Background(), "aws-sm://prod/db")
	require.NoError(t, err)
	assert.Equal(t, "", f.regions[1], "a bare name uses the default chain's region")
}

func TestCacheHitAndTTLExpiry(t *testing.T) {
	f := newFake()
	f.secrets["prod/db"] = "one"
	s, c := newSvc(allowAll, f)
	ctx := context.Background()
	_, _ = s.Resolve(ctx, "aws-sm://prod/db")
	_, _ = s.Resolve(ctx, "aws-sm://prod/db")
	assert.EqualValues(t, 1, f.calls.Load(), "second read is a cache hit")

	f.mu.Lock()
	f.secrets["prod/db"] = "two"
	f.mu.Unlock()
	c.add(4 * time.Minute)
	v, _ := s.Resolve(ctx, "aws-sm://prod/db")
	assert.Equal(t, "one", v, "still fresh")
	c.add(2 * time.Minute)
	v, _ = s.Resolve(ctx, "aws-sm://prod/db")
	assert.Equal(t, "two", v, "past the TTL: a rotated secret is read")
	assert.EqualValues(t, 2, f.calls.Load())
}

func TestSingleflight(t *testing.T) {
	f := newFake()
	f.secrets["prod/db"] = "v"
	f.gate = make(chan struct{})
	s, _ := newSvc(allowAll, f)

	const n = 20
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			v, err := s.Resolve(context.Background(), "aws-sm://prod/db")
			assert.NoError(t, err)
			assert.Equal(t, "v", v)
		})
	}
	// Let every caller reach the flight before the call returns.
	require.Eventually(t, func() bool { return f.calls.Load() == 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	close(f.gate)
	wg.Wait()
	assert.EqualValues(t, 1, f.calls.Load(), "N concurrent misses make one AWS call")
}

func TestStaleOnErrorWithinMaxStale(t *testing.T) {
	f := newFake()
	f.secrets["prod/db"] = "good"
	s, c := newSvc(allowAll, f)
	ctx := context.Background()
	_, err := s.Resolve(ctx, "aws-sm://prod/db")
	require.NoError(t, err)

	f.setErr(errors.New("connection reset"))
	c.add(10 * time.Minute)
	v, err := s.Resolve(ctx, "aws-sm://prod/db")
	require.NoError(t, err, "a failed refresh serves the last value")
	assert.Equal(t, "good", v)
	calls := f.calls.Load()

	// The failed refresh is remembered: no hammering from a 15 s loop.
	c.add(15 * time.Second)
	v, err = s.Resolve(ctx, "aws-sm://prod/db")
	require.NoError(t, err)
	assert.Equal(t, "good", v)
	assert.Equal(t, calls, f.calls.Load())

	// After the negative window AWS is asked again; once AWS recovers the
	// value is fresh.
	f.setErr(nil)
	c.add(time.Minute)
	f.mu.Lock()
	f.secrets["prod/db"] = "rotated"
	f.mu.Unlock()
	v, _ = s.Resolve(ctx, "aws-sm://prod/db")
	assert.Equal(t, "rotated", v)
}

func TestErrorAfterMaxStale(t *testing.T) {
	f := newFake()
	f.secrets["prod/db"] = "good"
	s, c := newSvc(allowAll, f)
	ctx := context.Background()
	_, _ = s.Resolve(ctx, "aws-sm://prod/db")

	f.setErr(errors.New("connection reset"))
	c.add(5*time.Minute + time.Hour + time.Second)
	_, err := s.Resolve(ctx, "aws-sm://prod/db")
	require.Error(t, err)
	assert.Equal(t, CodeUnavailable, CodeOf(err))
}

func TestNegativeCache(t *testing.T) {
	f := newFake()
	s, c := newSvc(allowAll, f)
	ctx := context.Background()

	_, err := s.Resolve(ctx, "aws-sm://prod/missing")
	assert.Equal(t, CodeNotFound, CodeOf(err))
	_, err = s.Resolve(ctx, "aws-sm://prod/missing")
	assert.Equal(t, CodeNotFound, CodeOf(err))
	assert.EqualValues(t, 1, f.calls.Load(), "a failure with nothing cached is cached for 30 s")

	c.add(31 * time.Second)
	f.mu.Lock()
	f.secrets["prod/missing"] = "now here"
	f.mu.Unlock()
	v, err := s.Resolve(ctx, "aws-sm://prod/missing")
	require.NoError(t, err)
	assert.Equal(t, "now here", v)
}

func TestErrorCodes(t *testing.T) {
	f := newFake()
	s, c := newSvc(allowAll, f)
	ctx := context.Background()
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&smithy.GenericAPIError{Code: "AccessDeniedException"}, CodeDenied},
		{&smithy.GenericAPIError{Code: "ThrottlingException"}, CodeUnavailable},
		{&smithy.GenericAPIError{Code: "InternalServiceError"}, CodeUnavailable},
		{errors.New("dial tcp: i/o timeout"), CodeUnavailable},
	} {
		f.setErr(tc.err)
		c.add(time.Minute)
		_, err := s.Resolve(ctx, "aws-sm://prod/x")
		assert.Equal(t, tc.want, CodeOf(err))
	}
}

func TestAllowList(t *testing.T) {
	s, _ := newSvc("aws-sm://prod/data/,aws-ps:///inhance/np/shared/", newFake())
	require.NoError(t, s.Allowed("aws-sm://prod/data/db#user"))
	require.NoError(t, s.Allowed("aws-ps:///inhance/np/shared/x"))
	assert.Equal(t, CodeNotAllowed, CodeOf(s.Allowed("aws-sm://prod/other")))
	assert.Equal(t, CodeNotAllowed, CodeOf(s.Allowed("aws-ps:///inhance/np/private")))

	// An ARN matches on its text: allowing by name does not allow by ARN.
	arn := "aws-sm://arn:aws:secretsmanager:eu-west-1:1:secret:prod/data/db-x"
	assert.Equal(t, CodeNotAllowed, CodeOf(s.Allowed(arn)))

	empty, _ := newSvc("", newFake())
	err := empty.Allowed("aws-sm://prod/data/db")
	assert.Equal(t, CodeNotAllowed, CodeOf(err))
	assert.Contains(t, err.Error(), "allows no secret references")
}

func TestResolveRefusesWhatIsNotAllowed(t *testing.T) {
	f := newFake()
	f.secrets["other/db"] = "v"
	s, _ := newSvc("aws-sm://prod/", f)
	_, err := s.Resolve(context.Background(), "aws-sm://other/db")
	assert.Equal(t, CodeNotAllowed, CodeOf(err))
	assert.EqualValues(t, 0, f.calls.Load(), "a refused reference never reaches AWS")
}

func TestCheckWrite(t *testing.T) {
	s, _ := newSvc("aws-sm://prod/", newFake())
	for _, ok := range []string{"plain secret", "encrypted:abc", "literal:dev", "aws-sm://prod/db", "aws-sm://prod/db#k"} {
		assert.NoError(t, s.CheckWrite(ok), ok)
	}
	for _, v := range []string{"gcp-sm://p/s", "vault://x#y", "env://HOME"} {
		err := s.CheckWrite(v)
		require.Error(t, err, v)
		assert.Contains(t, err.Error(), CodeUnsupported)
		assert.Contains(t, err.Error(), "aws-sm://")
		assert.Contains(t, err.Error(), "aws-ps://")
	}
	err := s.CheckWrite("aws-ps:///x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeNotAllowed)
	assert.Contains(t, err.Error(), "aws-sm://prod/")
}

// The resolved value never reaches a log line or an error.
func TestValueNeverLogged(t *testing.T) {
	const secret = "s3cr3t-VALUE-do-not-log"
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	f := newFake()
	f.secrets["prod/db"] = `{"password":"` + secret + `"}`
	s, c := newSvc(allowAll, f)
	ctx := context.Background()

	v, err := s.Resolve(ctx, "aws-sm://prod/db#password")
	require.NoError(t, err)
	require.Equal(t, secret, v)

	// Provoke the stale-serve WARN and the error paths.
	f.setErr(errors.New("boom"))
	c.add(10 * time.Minute)
	_, err = s.Resolve(ctx, "aws-sm://prod/db#password")
	require.NoError(t, err)
	_, err = s.Resolve(ctx, "aws-sm://prod/db#nokey")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
	_, err = s.Resolve(ctx, "aws-sm://prod/db#password")
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "serving the last value", "the stale serve is logged")
	assert.False(t, strings.Contains(buf.String(), secret), "a resolved value must not appear in the logs")
}

func TestToUsecase(t *testing.T) {
	assert.Contains(t, ToUsecase(&Error{Code: CodeNotFound, Ref: "aws-sm://a", Msg: "m"}).Error(), CodeNotFound)
	assert.True(t, IsUnavailable(ToUsecase(&Error{Code: CodeUnavailable, Ref: "r", Msg: "m"})))
	assert.False(t, IsUnavailable(ToUsecase(&Error{Code: CodeDenied, Ref: "r", Msg: "m"})))
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("FC_SECRET_REFERENCE_ALLOW", " aws-sm://a/ , ,aws-ps:///b/ ")
	t.Setenv("FC_SECRET_REFERENCE_TTL", "90s")
	t.Setenv("FC_SECRET_REFERENCE_MAX_STALE", "nonsense")
	c := ConfigFromEnv()
	assert.Equal(t, []string{"aws-sm://a/", "aws-ps:///b/"}, c.Allow)
	assert.Equal(t, 90*time.Second, c.TTL)
	assert.Equal(t, DefaultMaxStale, c.MaxStale)
}
