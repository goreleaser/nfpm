// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpmpack

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDirIndex(t *testing.T) {
	testCases := []struct {
		name        string
		before      []string
		dir         string
		wantGet     uint32
		wantAllDirs []string
	}{{
		name:        "first",
		dir:         "/first",
		wantGet:     0,
		wantAllDirs: []string{"/first"},
	}, {
		name:        "second",
		dir:         "second",
		before:      []string{"first"},
		wantGet:     1,
		wantAllDirs: []string{"first", "second"},
	}, {
		name:        "repeat",
		dir:         "second",
		before:      []string{"first", "second", "third"},
		wantGet:     1,
		wantAllDirs: []string{"first", "second", "third"},
	}}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			d := newDirIndex()
			for _, b := range tc.before {
				d.Get(b)
			}
			if got := d.Get(tc.dir); got != tc.wantGet {
				t.Errorf("d.Get(%q) = %d, want: %d", tc.dir, got, tc.wantGet)
			}
			if df := cmp.Diff(tc.wantAllDirs, d.AllDirs()); df != "" {
				t.Errorf("d.AllDirs() diff (want->got):\n%s", df)
			}
		})
	}
}
