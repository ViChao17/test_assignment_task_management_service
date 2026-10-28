package api

import (
	"encoding/json"
	"testing"
)

func ptrInt64(value int64) *int64 { return &value }

func TestUpdateTaskRequestAssignee(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantID  *int64
		wantSet bool
		wantErr bool
	}{
		{name: "omitted", body: `{}`, wantSet: false},
		{name: "explicit null", body: `{"assignee_id":null}`, wantSet: true},
		{name: "positive value", body: `{"assignee_id":42}`, wantID: ptrInt64(42), wantSet: true},
		{name: "zero", body: `{"assignee_id":0}`, wantSet: true, wantErr: true},
		{name: "negative", body: `{"assignee_id":-1}`, wantSet: true, wantErr: true},
		{name: "string", body: `{"assignee_id":"42"}`, wantSet: true, wantErr: true},
		{name: "float", body: `{"assignee_id":1.5}`, wantSet: true, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var request updateTaskRequest
			if err := json.Unmarshal([]byte(test.body), &request); err != nil {
				t.Fatalf("unmarshal request: %v", err)
			}

			id, set, err := request.assignee()
			if (err != nil) != test.wantErr {
				t.Fatalf("assignee() error = %v, wantErr %v", err, test.wantErr)
			}
			if set != test.wantSet {
				t.Fatalf("assignee() set = %v, want %v", set, test.wantSet)
			}
			if test.wantID == nil {
				if id != nil {
					t.Fatalf("assignee() id = %v, want nil", *id)
				}
				return
			}
			if id == nil || *id != *test.wantID {
				t.Fatalf("assignee() id = %v, want %v", id, *test.wantID)
			}
		})
	}
}
