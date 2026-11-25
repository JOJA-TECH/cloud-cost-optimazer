package adapters

import "testing"

func TestMapARMToConfigReadingComputeModel(t *testing.T) {
	capacity := 2.0
	minCapacity := 0.5
	tests := []struct {
		name string
		sku  *armSKU
		want string
	}{
		{"basic", &armSKU{Name: "Basic", Tier: "Basic", Capacity: &capacity}, "provisioned"},
		{"serverless", &armSKU{Name: "GP_S_Gen5_2", Tier: "GeneralPurpose", Capacity: &capacity, MinCapacity: &minCapacity}, "serverless"},
		{"no sku", nil, "provisioned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapARMToConfigReading(armDatabaseResponse{SKU: tt.sku})
			if got.ComputeModel == nil || *got.ComputeModel != tt.want {
				t.Fatalf("compute_model = %v, want %q", got.ComputeModel, tt.want)
			}
		})
	}
}
