package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

func writeTestJar(t *testing.T, describe string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if describe != "" {
		w, err := zw.Create("META-INF/flowcatalyst/function.json")
		require.NoError(t, err)
		_, _ = w.Write([]byte(describe))
	}
	require.NoError(t, zw.Close())
	p := filepath.Join(t.TempDir(), "hello.jar")
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0o600))
	return p
}

func TestRuntimeForJar(t *testing.T) {
	assert.Equal(t, "jvm", runtimeFor("target/hello-1.0-all.jar"))
	assert.Equal(t, "js", runtimeFor("dist/function.js"))
	assert.Equal(t, "wasm", runtimeFor("function.wasm"))
}

// A jar is described from its zip, with no engine, and an unpublishable one
// says why.
func TestDescribeArtifact_Jar(t *testing.T) {
	good := writeTestJar(t, `{"abi":1,"jvm":{"entrypoint":"com.example.Hello"},"endpoints":[{"method":"GET","path":"/hello","auth":"platform"}]}`)
	d, doc, err := describeArtifact(t.Context(), good, "jvm")
	require.NoError(t, err)
	require.Len(t, d.Endpoints, 1)
	require.NotNil(t, d.JVM)
	assert.Equal(t, "com.example.Hello", d.JVM.Entrypoint)
	assert.Contains(t, string(doc), "com.example.Hello")

	_, _, err = describeArtifact(t.Context(), writeTestJar(t, ""), "jvm")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "function.json")
}

// The upload carries a jar's content type and the publish names the jvm runtime.
func TestPublishBytes_Jar(t *testing.T) {
	var uploadType, publishBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch r.Method {
		case http.MethodPut:
			uploadType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusCreated)
		case http.MethodPost:
			publishBody = string(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"number":1,"digest":"d","runtime":"jvm"}`))
		}
	}))
	defer srv.Close()
	a := &fnAPI{c: fnCredentials{PlatformURL: srv.URL}, target: &fnTarget{}, tokens: staticToken("t"), http: srv.Client()}
	v, err := a.publishBytes(t.Context(), &fnFunction{ID: "fn1"}, []byte("PK"), "d", "jvm")
	require.NoError(t, err)
	assert.Equal(t, "jvm", v.Runtime)
	assert.Equal(t, "application/java-archive", uploadType)
	assert.JSONEq(t, `{"digest":"d","runtime":"jvm"}`, publishBody)
}
