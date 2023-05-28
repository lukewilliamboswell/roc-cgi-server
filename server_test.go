package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseURLParameters(t *testing.T) {
	tests := []struct {
		url            string
		pattern        string
		expectedParams map[string]string
	}{
		{
			url:            "/todo",
			pattern:        "/todo",
			expectedParams: map[string]string{},
		}, {
			url:            "/todo/123",
			pattern:        "/todo/{id}",
			expectedParams: map[string]string{"id": "123"},
		},
		{
			url:            "/user/john",
			pattern:        "/user/{username}",
			expectedParams: map[string]string{"username": "john"},
		},
		{
			url:            "/product/abc/123",
			pattern:        "/product/{category}/{id}",
			expectedParams: map[string]string{"category": "abc", "id": "123"},
		},
		{
			url:            "/order/123",
			pattern:        "/order",
			expectedParams: nil,
		},
		{
			url:            "/user",
			pattern:        "/user/{username}",
			expectedParams: nil,
		},
	}

	for _, test := range tests {
		params := parseURLParameters(test.url, test.pattern)
		if test.expectedParams == nil {
			assert.Nil(t, params, "Test failed for URL: %s, Pattern: %s", test.url, test.pattern)
		} else {
			assert.Equal(t, test.expectedParams, params, "Test failed for URL: %s, Pattern: %s", test.url, test.pattern)
		}
	}
}
