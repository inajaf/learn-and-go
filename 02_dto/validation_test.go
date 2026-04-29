package dto_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dto "learning_path/02_dto"
)

// =============================================================================
// CreateOrderRequest validation tests
// =============================================================================

func TestCreateOrderRequest_Validate(t *testing.T) {
	tests := []struct {
		name       string
		req        dto.CreateOrderRequest
		wantErr    bool
		wantFields []string // which fields should contain errors
	}{
		{
			name:    "valid request",
			req:     dto.CreateOrderRequest{CustomerID: "cust-1", Amount: 100.0},
			wantErr: false,
		},
		{
			name:       "empty customer_id",
			req:        dto.CreateOrderRequest{CustomerID: "", Amount: 100.0},
			wantErr:    true,
			wantFields: []string{"customer_id"},
		},
		{
			name:       "whitespace-only customer_id",
			req:        dto.CreateOrderRequest{CustomerID: "   ", Amount: 100.0},
			wantErr:    true,
			wantFields: []string{"customer_id"},
		},
		{
			name:       "zero amount",
			req:        dto.CreateOrderRequest{CustomerID: "cust-1", Amount: 0},
			wantErr:    true,
			wantFields: []string{"amount"},
		},
		{
			name:       "negative amount",
			req:        dto.CreateOrderRequest{CustomerID: "cust-1", Amount: -50},
			wantErr:    true,
			wantFields: []string{"amount"},
		},
		{
			name:       "amount too large",
			req:        dto.CreateOrderRequest{CustomerID: "cust-1", Amount: 2_000_000},
			wantErr:    true,
			wantFields: []string{"amount"},
		},
		{
			name:       "multiple errors at once",
			req:        dto.CreateOrderRequest{CustomerID: "", Amount: -1},
			wantErr:    true,
			wantFields: []string{"customer_id", "amount"},
		},
		{
			name: "customer_id too long",
			req: dto.CreateOrderRequest{
				CustomerID: strings.Repeat("a", 101),
				Amount:     50.0,
			},
			wantErr:    true,
			wantFields: []string{"customer_id"},
		},
		{
			name:    "minimal valid amount",
			req:     dto.CreateOrderRequest{CustomerID: "c", Amount: 0.01},
			wantErr: false,
		},
		{
			name:    "max valid amount",
			req:     dto.CreateOrderRequest{CustomerID: "c", Amount: 1_000_000},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)

			// 👉 Check that the error is indeed a ValidationError
			var ve *dto.ValidationError
			require.ErrorAs(t, err, &ve)

			// Check that the errors contain the expected fields
			gotFields := make([]string, len(ve.Fields))
			for i, f := range ve.Fields {
				gotFields[i] = f.Field
			}
			for _, wantField := range tt.wantFields {
				assert.Contains(t, gotFields, wantField,
					"expected an error for field %q", wantField)
			}
		})
	}
}

// =============================================================================
// UpdateOrderRequest validation tests
// =============================================================================

func TestUpdateOrderRequest_Validate(t *testing.T) {
	neg := -10.0
	zero := 0.0
	big := 2_000_000.0
	valid := 50.0

	tests := []struct {
		name    string
		req     dto.UpdateOrderRequest
		wantErr bool
	}{
		{name: "nil amount (no update)", req: dto.UpdateOrderRequest{Amount: nil}, wantErr: false},
		{name: "valid amount", req: dto.UpdateOrderRequest{Amount: &valid}, wantErr: false},
		{name: "negative amount", req: dto.UpdateOrderRequest{Amount: &neg}, wantErr: true},
		{name: "zero amount", req: dto.UpdateOrderRequest{Amount: &zero}, wantErr: true},
		{name: "too large amount", req: dto.UpdateOrderRequest{Amount: &big}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// JSON edge-cases: null vs absent vs zero
// =============================================================================

func TestJSON_NullVsAbsentVsZero(t *testing.T) {
	// 👉 Three different JSON inputs → three different Amount states in UpdateOrderRequest:
	//
	//   {"amount": null}    → Amount = nil    (explicit null)
	//   {}                  → Amount = nil    (field absent)
	//   {"amount": 0}       → Amount = &0     (zero sent)
	//
	// Pointer fields let us distinguish "not sent" from "zero sent".

	tests := []struct {
		name      string
		jsonInput string
		wantNil   bool
		wantValue *float64
	}{
		{
			name:      "amount is null",
			jsonInput: `{"amount": null}`,
			wantNil:   true,
		},
		{
			name:      "amount is absent",
			jsonInput: `{}`,
			wantNil:   true,
		},
		{
			name:      "amount is zero",
			jsonInput: `{"amount": 0}`,
			wantNil:   false,
			wantValue: ptrFloat(0),
		},
		{
			name:      "amount is positive",
			jsonInput: `{"amount": 42.5}`,
			wantNil:   false,
			wantValue: ptrFloat(42.5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req dto.UpdateOrderRequest
			err := json.Unmarshal([]byte(tt.jsonInput), &req)
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, req.Amount)
			} else {
				require.NotNil(t, req.Amount)
				assert.Equal(t, *tt.wantValue, *req.Amount)
			}
		})
	}
}

func TestJSON_CreateOrderRequest_Deserialize(t *testing.T) {
	tests := []struct {
		name       string
		jsonInput  string
		wantErr    bool
		wantCustID string
		wantAmount float64
	}{
		{
			name:       "normal JSON",
			jsonInput:  `{"customer_id": "cust-1", "amount": 99.99}`,
			wantCustID: "cust-1",
			wantAmount: 99.99,
		},
		{
			name:       "missing amount → zero value",
			jsonInput:  `{"customer_id": "cust-1"}`,
			wantCustID: "cust-1",
			wantAmount: 0, // 👉 float64 zero value
		},
		{
			name:       "missing customer_id → empty string",
			jsonInput:  `{"amount": 50}`,
			wantCustID: "", // 👉 string zero value
			wantAmount: 50,
		},
		{
			name:       "empty JSON object",
			jsonInput:  `{}`,
			wantCustID: "",
			wantAmount: 0,
		},
		{
			name:      "invalid JSON",
			jsonInput: `{invalid`,
			wantErr:   true,
		},
		{
			name:       "extra unknown fields are ignored",
			jsonInput:  `{"customer_id": "c1", "amount": 10, "unknown": true}`,
			wantCustID: "c1",
			wantAmount: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req dto.CreateOrderRequest
			err := json.Unmarshal([]byte(tt.jsonInput), &req)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCustID, req.CustomerID)
			assert.Equal(t, tt.wantAmount, req.Amount)
		})
	}
}

// =============================================================================
// ValidationError formatting
// =============================================================================

func TestValidationError_ErrorMessage(t *testing.T) {
	ve := &dto.ValidationError{
		Fields: []dto.FieldError{
			{Field: "customer_id", Message: "required field"},
			{Field: "amount", Message: "must be greater than 0"},
		},
	}

	msg := ve.Error()
	assert.Contains(t, msg, "customer_id")
	assert.Contains(t, msg, "amount")
	assert.Contains(t, msg, "validation failed")
}

func TestValidationError_JSONSerializable(t *testing.T) {
	// 👉 FieldError has json tags — it can be returned to the client as is
	ve := &dto.ValidationError{
		Fields: []dto.FieldError{
			{Field: "amount", Message: "required field"},
		},
	}

	data, err := json.Marshal(ve.Fields)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"field":"amount"`)
	assert.Contains(t, string(data), `"message":"required field"`)
}

func ptrFloat(v float64) *float64 { return &v }
