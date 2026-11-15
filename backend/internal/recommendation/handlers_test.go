package recommendation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlersReturnStorageUnavailableWithoutService(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{"generate", http.MethodPost, handleGenerate(nil)},
		{"evaluate", http.MethodPost, handleEvaluate(nil)},
		{"evaluate existing", http.MethodPost, handleEvaluateExisting(nil)},
		{"list", http.MethodGet, handleList(nil)},
		{"get", http.MethodGet, handleGet(nil)},
		{"evidence", http.MethodGet, handleEvidence(nil)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, "/", nil)
			test.handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatalf("decode response body: %v", err)
			}
			if body.Code != "RECOMMENDATION_STORAGE_UNAVAILABLE" {
				t.Fatalf("response code = %q, want RECOMMENDATION_STORAGE_UNAVAILABLE", body.Code)
			}
		})
	}
}
