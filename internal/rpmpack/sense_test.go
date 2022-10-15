package rpmpack

import (
	"testing"
)

func TestNewRelation(t *testing.T) {
	testCases := []struct {
		input, output string
		errExpected   bool
	}{
		{
			input:  "python >= 3.7",
			output: "python>=3.7",
		},
		{
			input:  "python",
			output: "python",
		},
		{
			input:  "python=2",
			output: "python=2",
		},
		{
			input:  "python >=3.5",
			output: "python>=3.5",
		},
		{
			input:       "python >< 3.5",
			output:      "",
			errExpected: true,
		},
		{
			input:       "python <> 3.5",
			output:      "",
			errExpected: true,
		},
		{
			input:       "python == 3.5",
			output:      "",
			errExpected: true,
		},
		{
			input:       "python =< 3.5",
			output:      "",
			errExpected: true,
		},
		{
			input:       "python => 3.5",
			output:      "",
			errExpected: true,
		},
	}

	for _, tc := range testCases {
		testCase := tc
		t.Run(testCase.input, func(tt *testing.T) {
			relation, err := NewRelation(testCase.input)
			switch {
			case testCase.errExpected && err == nil:
				tt.Errorf("%s should have returned an error", testCase.input)
				return
			case !testCase.errExpected && err != nil:
				tt.Errorf("%s should not have returned an error: %v", testCase.input, err)
				return
			case testCase.errExpected && err != nil:
				return
			}

			if relation == nil {
				tt.Errorf("%s should not have returned a nil relation", testCase.input)
				return
			}

			val := relation.String()
			if !testCase.errExpected && val != testCase.output {
				tt.Errorf("%s should have returned %s not %s", testCase.input, testCase.output, val)
			}
		})
	}
}
