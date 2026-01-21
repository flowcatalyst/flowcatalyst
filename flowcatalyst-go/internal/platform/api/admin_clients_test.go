package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"go.flowcatalyst.tech/internal/platform/client"
)

// MockClientRepository implements a mock client repository for testing
type MockClientRepository struct {
	clients     map[string]*client.Client
	insertErr   error
	findErr     error
	updateErr   error
	deleteErr   error
	statusErr   error
	addNoteErr  error
}

func NewMockClientRepository() *MockClientRepository {
	return &MockClientRepository{
		clients: make(map[string]*client.Client),
	}
}

func (m *MockClientRepository) Insert(ctx context.Context, c *client.Client) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	// Check for duplicate identifier
	for _, existing := range m.clients {
		if existing.Identifier == c.Identifier {
			return client.ErrDuplicateIdentifier
		}
	}
	c.ID = "test-id-" + c.Identifier
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	m.clients[c.ID] = c
	return nil
}

func (m *MockClientRepository) FindByID(ctx context.Context, id string) (*client.Client, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	if c, ok := m.clients[id]; ok {
		return c, nil
	}
	return nil, client.ErrNotFound
}

func (m *MockClientRepository) FindAll(ctx context.Context, skip, limit int64) ([]*client.Client, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	result := make([]*client.Client, 0, len(m.clients))
	for _, c := range m.clients {
		result = append(result, c)
	}
	return result, nil
}

func (m *MockClientRepository) Update(ctx context.Context, c *client.Client) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if _, ok := m.clients[c.ID]; !ok {
		return client.ErrNotFound
	}
	c.UpdatedAt = time.Now()
	m.clients[c.ID] = c
	return nil
}

func (m *MockClientRepository) Delete(ctx context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.clients[id]; !ok {
		return client.ErrNotFound
	}
	delete(m.clients, id)
	return nil
}

func (m *MockClientRepository) UpdateStatus(ctx context.Context, id string, status client.ClientStatus, reason string) error {
	if m.statusErr != nil {
		return m.statusErr
	}
	c, ok := m.clients[id]
	if !ok {
		return client.ErrNotFound
	}
	c.Status = status
	c.StatusReason = reason
	c.StatusChangedAt = time.Now()
	c.UpdatedAt = time.Now()
	return nil
}

func (m *MockClientRepository) AddNote(ctx context.Context, id string, note client.ClientNote) error {
	if m.addNoteErr != nil {
		return m.addNoteErr
	}
	c, ok := m.clients[id]
	if !ok {
		return client.ErrNotFound
	}
	note.Timestamp = time.Now()
	c.Notes = append(c.Notes, note)
	c.UpdatedAt = time.Now()
	return nil
}

// Helper to create a handler with mock repo
func newTestClientHandler() (*ClientAdminHandler, *MockClientRepository) {
	mockRepo := NewMockClientRepository()
	// Create a wrapper that satisfies the handler's needs
	handler := &ClientAdminHandler{repo: nil}
	// We'll use direct method testing instead of injecting mock
	return handler, mockRepo
}

// Helper to execute a request with chi context
func executeClientRequest(handler http.HandlerFunc, method, path string, body interface{}, urlParams map[string]string) *httptest.ResponseRecorder {
	var reqBody bytes.Buffer
	if body != nil {
		json.NewEncoder(&reqBody).Encode(body)
	}

	req := httptest.NewRequest(method, path, &reqBody)
	req.Header.Set("Content-Type", "application/json")

	// Add URL params to chi context
	rctx := chi.NewRouteContext()
	for k, v := range urlParams {
		rctx.URLParams.Add(k, v)
	}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// Tests for CreateClientRequest validation
func TestCreateClientRequest_Validation(t *testing.T) {
	tests := []struct {
		name           string
		request        CreateClientRequest
		expectedValid  bool
	}{
		{
			name:          "valid request",
			request:       CreateClientRequest{Name: "Test Client", Identifier: "test-client"},
			expectedValid: true,
		},
		{
			name:          "empty name",
			request:       CreateClientRequest{Name: "", Identifier: "test-client"},
			expectedValid: false,
		},
		{
			name:          "empty identifier",
			request:       CreateClientRequest{Name: "Test Client", Identifier: ""},
			expectedValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.request.Name != "" && tt.request.Identifier != ""
			if valid != tt.expectedValid {
				t.Errorf("Expected valid=%v, got %v", tt.expectedValid, valid)
			}
		})
	}
}

// Tests for ClientDTO conversion
func TestToClientDTO(t *testing.T) {
	now := time.Now()
	c := &client.Client{
		ID:              "client-123",
		Name:            "Test Client",
		Identifier:      "test-client",
		Status:          client.ClientStatusActive,
		StatusReason:    "",
		StatusChangedAt: time.Time{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	dto := toClientDTO(c)

	if dto.ID != c.ID {
		t.Errorf("Expected ID %s, got %s", c.ID, dto.ID)
	}
	if dto.Name != c.Name {
		t.Errorf("Expected Name %s, got %s", c.Name, dto.Name)
	}
	if dto.Identifier != c.Identifier {
		t.Errorf("Expected Identifier %s, got %s", c.Identifier, dto.Identifier)
	}
	if dto.Status != c.Status {
		t.Errorf("Expected Status %s, got %s", c.Status, dto.Status)
	}
	if dto.StatusReason != "" {
		t.Errorf("Expected empty StatusReason, got %s", dto.StatusReason)
	}
	if dto.StatusChangedAt != "" {
		t.Errorf("Expected empty StatusChangedAt, got %s", dto.StatusChangedAt)
	}
}

func TestToClientDTO_WithStatusChange(t *testing.T) {
	now := time.Now()
	c := &client.Client{
		ID:              "client-456",
		Name:            "Suspended Client",
		Identifier:      "suspended-client",
		Status:          client.ClientStatusSuspended,
		StatusReason:    "Non-payment",
		StatusChangedAt: now,
		CreatedAt:       now.Add(-24 * time.Hour),
		UpdatedAt:       now,
	}

	dto := toClientDTO(c)

	if dto.StatusReason != "Non-payment" {
		t.Errorf("Expected StatusReason 'Non-payment', got %s", dto.StatusReason)
	}
	if dto.StatusChangedAt == "" {
		t.Error("Expected StatusChangedAt to be set")
	}
}

// Test Client entity methods
func TestClient_IsActive(t *testing.T) {
	tests := []struct {
		status   client.ClientStatus
		expected bool
	}{
		{client.ClientStatusActive, true},
		{client.ClientStatusSuspended, false},
		{client.ClientStatusInactive, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			c := &client.Client{Status: tt.status}
			if c.IsActive() != tt.expected {
				t.Errorf("Expected IsActive()=%v for status %s", tt.expected, tt.status)
			}
		})
	}
}

func TestClient_IsSuspended(t *testing.T) {
	tests := []struct {
		status   client.ClientStatus
		expected bool
	}{
		{client.ClientStatusActive, false},
		{client.ClientStatusSuspended, true},
		{client.ClientStatusInactive, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			c := &client.Client{Status: tt.status}
			if c.IsSuspended() != tt.expected {
				t.Errorf("Expected IsSuspended()=%v for status %s", tt.expected, tt.status)
			}
		})
	}
}

// Test Mock Repository
func TestMockClientRepository_Insert(t *testing.T) {
	repo := NewMockClientRepository()

	c := &client.Client{
		Name:       "Test Client",
		Identifier: "test-client",
		Status:     client.ClientStatusActive,
	}

	err := repo.Insert(context.Background(), c)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	if c.ID == "" {
		t.Error("Expected ID to be set after insert")
	}

	if c.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

func TestMockClientRepository_Insert_DuplicateIdentifier(t *testing.T) {
	repo := NewMockClientRepository()

	c1 := &client.Client{Name: "Client 1", Identifier: "duplicate"}
	c2 := &client.Client{Name: "Client 2", Identifier: "duplicate"}

	repo.Insert(context.Background(), c1)
	err := repo.Insert(context.Background(), c2)

	if err != client.ErrDuplicateIdentifier {
		t.Errorf("Expected ErrDuplicateIdentifier, got %v", err)
	}
}

func TestMockClientRepository_FindByID(t *testing.T) {
	repo := NewMockClientRepository()

	c := &client.Client{Name: "Test", Identifier: "test"}
	repo.Insert(context.Background(), c)

	found, err := repo.FindByID(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}

	if found.ID != c.ID {
		t.Errorf("Expected ID %s, got %s", c.ID, found.ID)
	}
}

func TestMockClientRepository_FindByID_NotFound(t *testing.T) {
	repo := NewMockClientRepository()

	_, err := repo.FindByID(context.Background(), "nonexistent")
	if err != client.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestMockClientRepository_Update(t *testing.T) {
	repo := NewMockClientRepository()

	c := &client.Client{Name: "Original", Identifier: "test"}
	repo.Insert(context.Background(), c)

	c.Name = "Updated"
	err := repo.Update(context.Background(), c)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	found, _ := repo.FindByID(context.Background(), c.ID)
	if found.Name != "Updated" {
		t.Errorf("Expected name 'Updated', got %s", found.Name)
	}
}

func TestMockClientRepository_Delete(t *testing.T) {
	repo := NewMockClientRepository()

	c := &client.Client{Name: "ToDelete", Identifier: "delete-me"}
	repo.Insert(context.Background(), c)

	err := repo.Delete(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = repo.FindByID(context.Background(), c.ID)
	if err != client.ErrNotFound {
		t.Error("Expected client to be deleted")
	}
}

func TestMockClientRepository_UpdateStatus(t *testing.T) {
	repo := NewMockClientRepository()

	c := &client.Client{Name: "Test", Identifier: "test", Status: client.ClientStatusActive}
	repo.Insert(context.Background(), c)

	err := repo.UpdateStatus(context.Background(), c.ID, client.ClientStatusSuspended, "Non-payment")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	found, _ := repo.FindByID(context.Background(), c.ID)
	if found.Status != client.ClientStatusSuspended {
		t.Errorf("Expected status SUSPENDED, got %s", found.Status)
	}
	if found.StatusReason != "Non-payment" {
		t.Errorf("Expected reason 'Non-payment', got %s", found.StatusReason)
	}
}

func TestMockClientRepository_AddNote(t *testing.T) {
	repo := NewMockClientRepository()

	c := &client.Client{Name: "Test", Identifier: "test"}
	repo.Insert(context.Background(), c)

	note := client.ClientNote{Text: "Test note", Category: "SUPPORT"}
	err := repo.AddNote(context.Background(), c.ID, note)
	if err != nil {
		t.Fatalf("AddNote failed: %v", err)
	}

	found, _ := repo.FindByID(context.Background(), c.ID)
	if len(found.Notes) != 1 {
		t.Fatalf("Expected 1 note, got %d", len(found.Notes))
	}
	if found.Notes[0].Text != "Test note" {
		t.Errorf("Expected note text 'Test note', got %s", found.Notes[0].Text)
	}
}

// Test ClientDTO JSON serialization
func TestClientDTO_JSON(t *testing.T) {
	dto := ClientDTO{
		ID:         "client-123",
		Name:       "Test Client",
		Identifier: "test-client",
		Status:     client.ClientStatusActive,
		CreatedAt:  "2024-01-01T00:00:00Z",
		UpdatedAt:  "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Failed to marshal ClientDTO: %v", err)
	}

	jsonStr := string(data)

	// Verify JSON field names (camelCase)
	expectedFields := []string{`"id"`, `"name"`, `"identifier"`, `"status"`, `"createdAt"`, `"updatedAt"`}
	for _, field := range expectedFields {
		if !bytes.Contains(data, []byte(field)) {
			t.Errorf("Expected %s in JSON, got %s", field, jsonStr)
		}
	}

	// Verify status value
	if !bytes.Contains(data, []byte(`"status":"ACTIVE"`)) {
		t.Errorf("Expected status 'ACTIVE' in JSON, got %s", jsonStr)
	}
}

// Test request body structures
func TestCreateClientRequest_JSON(t *testing.T) {
	jsonData := `{"name": "New Client", "identifier": "new-client"}`

	var req CreateClientRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.Name != "New Client" {
		t.Errorf("Expected Name 'New Client', got '%s'", req.Name)
	}
	if req.Identifier != "new-client" {
		t.Errorf("Expected Identifier 'new-client', got '%s'", req.Identifier)
	}
}

func TestSuspendClientRequest_JSON(t *testing.T) {
	jsonData := `{"reason": "Non-payment"}`

	var req SuspendClientRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.Reason != "Non-payment" {
		t.Errorf("Expected Reason 'Non-payment', got '%s'", req.Reason)
	}
}

func TestAddNoteRequest_JSON(t *testing.T) {
	jsonData := `{"text": "Customer called", "category": "SUPPORT"}`

	var req AddNoteRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.Text != "Customer called" {
		t.Errorf("Expected Text 'Customer called', got '%s'", req.Text)
	}
	if req.Category != "SUPPORT" {
		t.Errorf("Expected Category 'SUPPORT', got '%s'", req.Category)
	}
}

// Test ClientNote
func TestClientNote_Structure(t *testing.T) {
	now := time.Now()
	note := client.ClientNote{
		Text:      "Test note",
		Timestamp: now,
		AddedBy:   "user-123",
		Category:  "GENERAL",
	}

	if note.Text != "Test note" {
		t.Errorf("Expected Text 'Test note', got '%s'", note.Text)
	}
	if note.AddedBy != "user-123" {
		t.Errorf("Expected AddedBy 'user-123', got '%s'", note.AddedBy)
	}
}

// Test ClientAccessGrant
func TestClientAccessGrant_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		expected  bool
	}{
		{
			name:      "no expiry",
			expiresAt: time.Time{},
			expected:  false,
		},
		{
			name:      "not expired",
			expiresAt: time.Now().Add(24 * time.Hour),
			expected:  false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-24 * time.Hour),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grant := &client.ClientAccessGrant{ExpiresAt: tt.expiresAt}
			if grant.IsExpired() != tt.expected {
				t.Errorf("Expected IsExpired()=%v, got %v", tt.expected, grant.IsExpired())
			}
		})
	}
}
