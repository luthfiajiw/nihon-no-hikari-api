package model

import "testing"

func TestUpsertLessonProgressRequestValidate(t *testing.T) {
	validStatus := StatusInProgress
	invalidStatus := Status("unknown")

	tests := []struct {
		name    string
		request UpsertLessonProgressRequest
		wantErr bool
	}{
		{name: "valid status", request: UpsertLessonProgressRequest{Status: &validStatus}},
		{name: "missing status", request: UpsertLessonProgressRequest{}, wantErr: true},
		{name: "invalid status", request: UpsertLessonProgressRequest{Status: &invalidStatus}, wantErr: true},
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
