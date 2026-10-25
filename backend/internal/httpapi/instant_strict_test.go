package httpapi

import (
    "net/http/httptest"
    "strings"
    "testing"
)

func TestDecodeInstantCommandStrictJSON(t *testing.T) {
    cases := []struct {
        name string
        body string
        valid bool
    }{
        {"valid", `{"accept":true}`, true},
        {"extra_field", `{"accept":true,"forged_owner":"someone"}`, false},
        {"second_object", "{\"accept\":true}\n{\"accept\":false}", false},
        {"trailing_garbage", `{"accept":true} garbage`, false},
        {"invalid", `{"accept":`, false},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            request := httptest.NewRequest("POST", "/v1/instant/dispatches/example/answer", strings.NewReader(tc.body))
            var payload struct { Accept *bool `json:"accept"` }
            err := decodeInstantCommand(request, &payload)
            if (err == nil) != tc.valid {
                t.Fatalf("valid=%v err=%v", tc.valid, err)
            }
        })
    }
}
