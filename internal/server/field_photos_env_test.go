package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FC_FIELD_PHOTOS is on unless switched off; the limits default to the
// documented values and a bad or non-positive value falls back to them.
func TestFieldPhotoEnvironment(t *testing.T) {
	for _, k := range []string{"FC_FIELD_PHOTOS", "FC_FIELD_PHOTO_MAX_BYTES", "FC_FIELD_PHOTO_MAX_PIXELS", "FC_FIELD_PHOTO_CONCURRENCY"} {
		t.Setenv(k, "")
	}
	c := LoadEnv()
	assert.True(t, c.FieldPhotos, "on by default")
	assert.EqualValues(t, 10<<20, c.FieldPhotoMaxBytes)
	assert.EqualValues(t, 25_000_000, c.FieldPhotoMaxPixels)
	assert.Equal(t, 2, c.FieldPhotoConcurrency)

	for _, off := range []string{"off", "false", "0", "no", "OFF"} {
		t.Setenv("FC_FIELD_PHOTOS", off)
		assert.False(t, LoadEnv().FieldPhotos, off)
	}
	t.Setenv("FC_FIELD_PHOTOS", "on")
	assert.True(t, LoadEnv().FieldPhotos)

	t.Setenv("FC_FIELD_PHOTO_MAX_BYTES", "2048")
	t.Setenv("FC_FIELD_PHOTO_MAX_PIXELS", "1000000")
	t.Setenv("FC_FIELD_PHOTO_CONCURRENCY", "4")
	c = LoadEnv()
	assert.EqualValues(t, 2048, c.FieldPhotoMaxBytes)
	assert.EqualValues(t, 1_000_000, c.FieldPhotoMaxPixels)
	assert.Equal(t, 4, c.FieldPhotoConcurrency)

	t.Setenv("FC_FIELD_PHOTO_MAX_BYTES", "-5")
	t.Setenv("FC_FIELD_PHOTO_CONCURRENCY", "zero")
	c = LoadEnv()
	assert.EqualValues(t, 10<<20, c.FieldPhotoMaxBytes)
	assert.Equal(t, 2, c.FieldPhotoConcurrency)
}
