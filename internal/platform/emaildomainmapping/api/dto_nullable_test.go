package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The PUT body tells an absent primaryClientId / requiredOidcTenantId (leave
// it) from an explicit null (clear it); plain pointers could not, so every
// update that did not resend them cleared them (owner decision #38).
func TestUpdateMappingRequestTellsAbsentFromNull(t *testing.T) {
	cases := []struct {
		body string
		want *string
	}{
		{`{}`, nil},
		{`{"primaryClientId":null}`, new("")},
		{`{"primaryClientId":""}`, new("")},
		{`{"primaryClientId":"clt_x"}`, new("clt_x")},
	}
	for _, tc := range cases {
		var req UpdateMappingRequest
		require.NoError(t, json.Unmarshal([]byte(tc.body), &req), tc.body)
		cmd := req.toCommand("edm_1")
		assert.Equal(t, tc.want, cmd.PrimaryClientID, tc.body)
	}
	var req UpdateMappingRequest
	require.NoError(t, json.Unmarshal([]byte(`{"requiredOidcTenantId":null}`), &req))
	assert.Equal(t, new(""), req.toCommand("edm_1").RequiredOIDCTenantID)
	assert.Nil(t, req.toCommand("edm_1").PrimaryClientID)
	assert.Error(t, json.Unmarshal([]byte(`{"primaryClientId":42}`), &req))
}
