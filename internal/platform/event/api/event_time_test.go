package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchItemTime(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	s := func(v string) *string { return &v }

	got, msg := batchItemTime(nil, now)
	assert.Nil(t, got)
	assert.Empty(t, msg)
	got, msg = batchItemTime(s("  "), now)
	assert.Nil(t, got)
	assert.Empty(t, msg)

	got, msg = batchItemTime(s("2026-11-09T11:15:02.481+02:00"), now)
	require.Empty(t, msg)
	assert.Equal(t, time.Date(2026, 11, 9, 9, 15, 2, 481e6, time.UTC), *got)

	_, msg = batchItemTime(s("2026-11-09T10:04:59Z"), now)
	assert.Empty(t, msg, "within the 5-minute skew")
	_, msg = batchItemTime(s("2026-11-09T10:05:01Z"), now)
	assert.NotEmpty(t, msg, "too far ahead")
	_, msg = batchItemTime(s("09/11/2026"), now)
	assert.NotEmpty(t, msg)
}
