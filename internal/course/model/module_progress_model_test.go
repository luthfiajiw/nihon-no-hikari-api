package model

import "testing"

func TestUpsertModuleProgressRequestValidate(t *testing.T) {
	validStatus := StatusCompleted
	invalidStatus := Status("unknown")
	validScore := int16(90)
	invalidScore := int16(101)

	tests := []struct {
		name    string
		request UpsertModuleProgressRequest
		wantErr bool
	}{
		{name: "status only", request: UpsertModuleProgressRequest{Status: &validStatus}},
		{name: "best score only", request: UpsertModuleProgressRequest{BestScore: &validScore}},
		{name: "status and best score", request: UpsertModuleProgressRequest{Status: &validStatus, BestScore: &validScore}},
		{name: "empty body", request: UpsertModuleProgressRequest{}, wantErr: true},
		{name: "invalid status", request: UpsertModuleProgressRequest{Status: &invalidStatus}, wantErr: true},
		{name: "score above maximum", request: UpsertModuleProgressRequest{BestScore: &invalidScore}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
