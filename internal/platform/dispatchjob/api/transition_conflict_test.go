package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestTransitionConflict(t *testing.T) {
	// The SQL guard's refusal, however it is wrapped on the way up, is a 409.
	for _, in := range []error{dispatchjob.ErrTransitionRefused, fmt.Errorf("persist: %w", dispatchjob.ErrTransitionRefused)} {
		out := transitionConflict(in)
		var ue *usecase.Error
		require.True(t, errors.As(out, &ue), "%v", out)
		require.Equal(t, "STATUS_CHANGED", ue.Code)
	}
	// Anything else passes through unchanged.
	other := errors.New("boom")
	require.Same(t, other, transitionConflict(other))
	require.NoError(t, transitionConflict(nil))
}
