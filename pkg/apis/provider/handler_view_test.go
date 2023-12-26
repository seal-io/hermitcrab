package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_regexValidArchive(t *testing.T) {
	testCases := []struct {
		given    string
		expected bool
	}{
		{
			given:    "terraform-provider-foo_1.2.3_darwin_amd64.zip",
			expected: true,
		},
		{
			given:    "terraform-provider-foo_1.2.3_darwin_amd64",
			expected: false,
		},
		{
			given:    "terraform-provider-foo_1.2.3_darwin_amd64.zip.zip",
			expected: false,
		},
		{
			given:    "terraform-provider-foo_darwin_amd64.zip.zip",
			expected: false,
		},
		{
			given:    "terraform-provider-foo__darwin_amd64.zip.zip",
			expected: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.given, func(t *testing.T) {
			ps := regexValidArchive.FindStringSubmatch(tc.given)
			assert.Equal(t, tc.expected, len(ps) == 5)
		})
	}
}
