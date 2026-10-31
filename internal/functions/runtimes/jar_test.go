package runtimes

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

const goodJvmDescribe = `{"abi":1,"jvm":{"entrypoint":"com.example.HelloFunction"},` +
	`"endpoints":[{"method":"GET","path":"/hello/{name}","auth":"platform"}]}`

// buildJar writes a jar (zip) with the given entries, in order.
func buildJar(t *testing.T, entries ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e[0])
		require.NoError(t, err)
		_, err = w.Write([]byte(e[1]))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func describeEntry(doc string) [2]string { return [2]string{JarDescribePath, doc} }

func problems(t *testing.T, err error) string {
	t.Helper()
	de, ok := errors.AsType[*abi.DescribeError](err)
	require.True(t, ok, "want *abi.DescribeError, got %T: %v", err, err)
	return strings.Join(de.Problems, "; ")
}

func TestDescribeJar_Good(t *testing.T) {
	jar := buildJar(t,
		[2]string{"META-INF/MANIFEST.MF", "Manifest-Version: 1.0\n"},
		[2]string{"com/example/HelloFunction.class", "\xca\xfe\xba\xbe"},
		describeEntry(goodJvmDescribe))
	doc, err := DescribeJar(jar)
	require.NoError(t, err)
	assert.JSONEq(t, goodJvmDescribe, string(doc), "the stored describe is the document, jvm block included")

	// The Loader reaches it through Describe for the jvm runtime, with no engine.
	doc, err = (&Loader{}).Describe(t.Context(), JVM, jar, 0)
	require.NoError(t, err)
	assert.JSONEq(t, goodJvmDescribe, string(doc))
}

func TestDescribeJar_Refused(t *testing.T) {
	huge := `{"abi":1,"jvm":{"entrypoint":"a.B"},"config":["` + strings.Repeat("x", MaxJarDescribeBytes) + `"]}`
	manyEntries := make([][2]string, 0, MaxJarEntries+2)
	for i := range MaxJarEntries + 1 {
		manyEntries = append(manyEntries, [2]string{"e/" + strings.Repeat("a", i%7) + string(rune('a'+i%26)) + strings.Repeat("b", i/26%50) + "/" + itoa(i), ""})
	}
	manyEntries = append(manyEntries, describeEntry(goodJvmDescribe))

	tests := []struct {
		name string
		jar  []byte
		want string
	}{
		{"not a zip", []byte("this is not a jar"), "not a jar"},
		{"empty", nil, "not a jar"},
		{"wasm bytes", []byte("\x00asm\x01\x00\x00\x00"), "not a jar"},
		{"no describe entry", buildJar(t, [2]string{"a.txt", "x"}), "has no " + JarDescribePath},
		{"describe not json", buildJar(t, describeEntry("not json")), "not a describe document"},
		{"describe has unknown field", buildJar(t, describeEntry(`{"abi":1,"nope":1,"jvm":{"entrypoint":"a.B"},"endpoints":[{"path":"/x","auth":"none"}]}`)), "not a describe document"},
		{"bad abi", buildJar(t, describeEntry(`{"abi":2,"jvm":{"entrypoint":"a.B"},"endpoints":[{"path":"/x","auth":"none"}]}`)), "abi: 2 is not supported"},
		{"no endpoints", buildJar(t, describeEntry(`{"abi":1,"jvm":{"entrypoint":"a.B"}}`)), "endpoints: at least one"},
		{"no jvm block", buildJar(t, describeEntry(`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}]}`)), "jvm.entrypoint: required"},
		{"empty entrypoint", buildJar(t, describeEntry(`{"abi":1,"jvm":{"entrypoint":""},"endpoints":[{"path":"/x","auth":"none"}]}`)), "not a Java binary class name"},
		{"slash entrypoint", buildJar(t, describeEntry(`{"abi":1,"jvm":{"entrypoint":"com/example/Hello"},"endpoints":[{"path":"/x","auth":"none"}]}`)), "not a Java binary class name"},
		{"digit-first entrypoint", buildJar(t, describeEntry(`{"abi":1,"jvm":{"entrypoint":"com.1x.Hello"},"endpoints":[{"path":"/x","auth":"none"}]}`)), "not a Java binary class name"},
		{"trailing-dot entrypoint", buildJar(t, describeEntry(`{"abi":1,"jvm":{"entrypoint":"com.example."},"endpoints":[{"path":"/x","auth":"none"}]}`)), "not a Java binary class name"},
		{"unknown jvm field", buildJar(t, describeEntry(`{"abi":1,"jvm":{"entrypoint":"a.B","x":1},"endpoints":[{"path":"/x","auth":"none"}]}`)), "not a describe document"},
		{"describe over the cap", buildJar(t, describeEntry(huge)), "uncompressed (at most"},
		{"too many entries", buildJar(t, manyEntries...), "entries (at most"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DescribeJar(tc.jar)
			require.Error(t, err)
			assert.Contains(t, problems(t, err), tc.want)
		})
	}
}

func TestDescribeJar_AcceptsBinaryClassNames(t *testing.T) {
	for _, name := range []string{"Hello", "com.example.Hello", "com.example.Outer$Inner", "_a.$b.C_1"} {
		jar := buildJar(t, describeEntry(`{"abi":1,"jvm":{"entrypoint":"`+name+`"},"endpoints":[{"path":"/x","auth":"none"}]}`))
		_, err := DescribeJar(jar)
		assert.NoError(t, err, name)
	}
}

// A compressible bomb is refused by the entry's declared size, before any of
// it is decompressed. (The read itself is also capped, for a header that lies.)
func TestDescribeJar_BombRefusedBySize(t *testing.T) {
	jar := buildJar(t, describeEntry(strings.Repeat(" ", 4*MaxJarDescribeBytes)+goodJvmDescribe))
	require.Less(t, len(jar), 64<<10, "the bomb is tiny compressed")
	_, err := DescribeJar(jar)
	require.Error(t, err)
	assert.Contains(t, problems(t, err), "uncompressed")
}

func TestJVMRuntimeTag(t *testing.T) {
	assert.True(t, ValidTag(JVM))
	assert.True(t, Publishable(JVM))
	assert.False(t, Valid(JVM), "the in-process runner cannot prepare a jvm version")
	assert.True(t, Publishable(Wasm) && Publishable(JS) && Publishable(""))
	assert.False(t, Publishable("python"))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}
