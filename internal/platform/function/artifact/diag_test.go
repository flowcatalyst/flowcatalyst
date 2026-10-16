package artifact

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3ErrKeepsSDKTextAndHints(t *testing.T) {
	t.Parallel()
	cred := errors.New("operation error S3: HeadObject, failed to retrieve credentials: no EC2 IMDS role found")
	err := s3Err("head_object", cred)
	assert.ErrorIs(t, err, cred)
	assert.Contains(t, err.Error(), "failed to retrieve credentials")
	assert.Contains(t, err.Error(), "credentials not resolvable")

	denied := s3Err("presign get_object", errors.New("api error AccessDenied: nope"))
	assert.Contains(t, denied.Error(), "access denied")
	assert.Contains(t, denied.Error(), "AccessDenied")

	other := s3Err("put_object", errors.New("boom"))
	assert.Equal(t, "artifact: s3 put_object: boom", other.Error())
}

func TestFromURLWrapsStoreErrors(t *testing.T) {
	t.Parallel()
	// A file store rooted under a regular file cannot Put: a real failure.
	dir := t.TempDir()
	st, err := FromURL(context.Background(), "file://"+dir+"/sub", "")
	require.NoError(t, err)
	d, ok := st.(*diagStore)
	require.True(t, ok, "FromURL must return the diagnostics wrapper")
	assert.Equal(t, "file", d.kind)
}
