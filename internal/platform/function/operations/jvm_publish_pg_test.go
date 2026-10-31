//go:build integration

package operations_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

const jvmDescribeDoc = `{"abi":1,"jvm":{"entrypoint":"com.example.HelloFunction"},` +
	`"endpoints":[{"method":"GET","path":"/hello/{name}","auth":"platform"}]}`

// jarWith builds a jar whose describe entry is describe ("" omits the entry);
// salt makes the digest unique per call.
func jarWith(t *testing.T, describe, salt string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("com/example/HelloFunction.class")
	require.NoError(t, err)
	_, _ = w.Write([]byte("\xca\xfe\xba\xbe" + salt))
	if describe != "" {
		w, err = zw.Create(runtimes.JarDescribePath)
		require.NoError(t, err)
		_, _ = w.Write([]byte(describe))
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func (f *testFunctionFixture) tryPublish(digest, runtime string) (operations.PublishResult, error) {
	f.t.Helper()
	return usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.Publish(f.repo, f.artifacts, f.loader),
		operations.PublishCommand{FunctionID: f.fn.ID, Digest: digest, Runtime: runtime}, testpg.TestEC())
}

// A jar publishes as a jvm version: read without executing anything, stored
// with its describe (jvm block included) so the Java host can read the
// entrypoint from the desired document.
func TestPublish_JarHappyPath(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_opjvmpub00001", "opjvmpubapp", "jvm-fn")
	digest := putArtifactBytes(t, f.artifacts, jarWith(t, jvmDescribeDoc, f.fn.ID))

	res := f.publish(digest, runtimes.JVM)
	assert.True(t, res.Created)
	assert.Equal(t, "jvm", res.Version.Runtime)
	assert.EqualValues(t, 1, res.Version.ABI)

	stored, err := f.repo.GetVersionByNumber(context.Background(), f.fn.ID, res.Version.Number)
	require.NoError(t, err)
	var d struct {
		JVM struct {
			Entrypoint string `json:"entrypoint"`
		} `json:"jvm"`
	}
	require.NoError(t, json.Unmarshal(stored.Describe, &d))
	assert.Equal(t, "com.example.HelloFunction", d.JVM.Entrypoint)

	// Idempotent replay; a second jar is a second version of the same runtime.
	again := f.publish(digest, runtimes.JVM)
	assert.False(t, again.Created)
	second := f.publish(putArtifactBytes(t, f.artifacts, jarWith(t, jvmDescribeDoc, "v2")), runtimes.JVM)
	assert.True(t, second.Created)
	assert.EqualValues(t, 2, second.Version.Number)
}

func TestPublish_BadJarIsDescribeInvalid(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_opjvmbad00001", "opjvmbadapp", "jvm-bad-fn")
	cases := map[string][]byte{
		"not a zip":        []byte("definitely not a jar"),
		"missing describe": jarWith(t, "", "a"),
		"bad describe":     jarWith(t, "{nope", "b"),
		"missing jvm":      jarWith(t, `{"abi":1,"endpoints":[{"path":"/x","auth":"none"}]}`, "c"),
		"bad entrypoint":   jarWith(t, `{"abi":1,"jvm":{"entrypoint":"com/example/X"},"endpoints":[{"path":"/x","auth":"none"}]}`, "d"),
		"oversized entry":  jarWith(t, `{"abi":1,"jvm":{"entrypoint":"a.B"},"config":["`+strings.Repeat("x", runtimes.MaxJarDescribeBytes)+`"]}`, "e"),
		"emits not owned":  jarWith(t, `{"abi":1,"jvm":{"entrypoint":"a.B"},"endpoints":[{"path":"/x","auth":"none"}],"emits":["other:a:b:c"]}`, "f"),
	}
	for name, jar := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.tryPublish(putArtifactBytes(t, f.artifacts, jar), runtimes.JVM)
			if name == "emits not owned" {
				testpg.RequireUsecaseError(t, err, usecase.KindValidation, "EMIT_NOT_OWNED")
				return
			}
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, "DESCRIBE_INVALID")
		})
	}
	versions, err := f.repo.ListVersionsByFunction(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Empty(t, versions, "nothing was stored")
}

// A jvm block belongs to a jvm function only.
func TestPublish_JVMBlockOnJSRuntimeRefused(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_opjvmblk00001", "opjvmblkapp", "jvm-blk-fn")
	_, err := f.tryPublish(putArtifactBytes(t, f.artifacts, jsArtifact(jvmDescribeDoc)), runtimes.JS)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "DESCRIBE_INVALID")
}

// A function is a JVM function or not from its first version; crossing that
// line later is RUNTIME_MISMATCH, in both directions. wasm and js, served by
// the same in-process runner, may still follow each other.
func TestPublish_RuntimeMismatch(t *testing.T) {
	t.Parallel()
	js := newTestFunctionFixture(t, "app_opjvmmis00001", "opjvmmisapp", "mis-js-fn")
	js.publish(putArtifactBytes(t, js.artifacts, jsArtifact(`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}]}`)), runtimes.JS)
	_, err := js.tryPublish(putArtifactBytes(t, js.artifacts, jarWith(t, jvmDescribeDoc, "m1")), runtimes.JVM)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "RUNTIME_MISMATCH")
	// js → wasm is not a mismatch (this module then fails to describe, for its own reasons).
	_, err = js.tryPublish(putArtifactBytes(t, js.artifacts, []byte("\x00asm\x01\x00\x00\x00")), "")
	if ue := usecase.AsError(err); ue != nil {
		require.NotEqual(t, "RUNTIME_MISMATCH", ue.Code, "wasm after js must not be a runtime mismatch: %v", err)
	}

	jvm := newTestFunctionFixture(t, "app_opjvmmis00002", "opjvmmis2app", "mis-jvm-fn")
	jvm.publish(putArtifactBytes(t, jvm.artifacts, jarWith(t, jvmDescribeDoc, "m2")), runtimes.JVM)
	_, err = jvm.tryPublish(putArtifactBytes(t, jvm.artifacts, jsArtifact(`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}]}`)), runtimes.JS)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "RUNTIME_MISMATCH")
	_, err = jvm.tryPublish(putArtifactBytes(t, jvm.artifacts, []byte("\x00asm\x01\x00\x00\x00")), "")
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "RUNTIME_MISMATCH")
}

func TestPublish_InvalidRuntimeNamesJVM(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_opjvminv00001", "opjvminvapp", "inv-fn")
	_, err := f.tryPublish(strings.Repeat("a", 64), "python")
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_RUNTIME")
}
