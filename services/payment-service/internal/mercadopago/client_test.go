package mercadopago_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/utilar/payment-service/internal/mercadopago"
)

// newTestClient injects a test server URL by temporarily replacing the base URL.
// We use a helper that creates a client pointing to the test server.
func newTestServer(handler http.HandlerFunc) (*httptest.Server, *mercadopago.Client) {
	srv := httptest.NewServer(handler)
	c := mercadopago.NewWithBaseURL("fake-token", srv.URL)
	return srv, c
}

func TestCreatePixPayment(t *testing.T) {
	srv, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/payments" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer fake-token" {
			t.Error("missing or wrong Authorization header")
		}

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["payment_method_id"] != "pix" {
			t.Errorf("expected payment_method_id=pix, got %v", body["payment_method_id"])
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "mp-pay-001",
			"status": "pending",
			"point_of_interaction": map[string]any{
				"transaction_data": map[string]any{
					"qr_code":        "00020101...",
					"qr_code_base64": "base64string",
				},
			},
		})
	})
	defer srv.Close()

	raw, err := client.CreatePixPayment("order-123", 99.90, "test@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal("response is not valid JSON")
	}
	if resp["id"] != "mp-pay-001" {
		t.Errorf("expected id=mp-pay-001, got %v", resp["id"])
	}
}

func TestGetPayment(t *testing.T) {
	srv, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payments/mp-pay-001" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "mp-pay-001", "status": "approved"})
	})
	defer srv.Close()

	raw, err := client.GetPayment("mp-pay-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(raw, &resp)
	if resp["status"] != "approved" {
		t.Errorf("expected status=approved, got %v", resp["status"])
	}
}

func TestClientReturnsErrorOn4xx(t *testing.T) {
	srv, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "bad_request"})
	})
	defer srv.Close()

	_, err := client.CreatePixPayment("order-x", 10.0, "a@b.com")
	if err == nil {
		t.Error("expected error for 4xx response, got nil")
	}
}

// Verify Client struct has the expected exported methods (compile-time shape check).
func TestClientInterface(t *testing.T) {
	c := mercadopago.New("token")
	typ := reflect.TypeOf(c)
	methods := []string{"CreatePixPayment", "CreateBoleto", "GetPayment", "CreatePreference"}
	for _, m := range methods {
		if _, ok := typ.MethodByName(m); !ok {
			t.Errorf("Client missing method: %s", m)
		}
	}
}
