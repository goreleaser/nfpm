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
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLead(t *testing.T) {
	// Only check that the length is always right
	names := []string{
		"a",
		"ab",
		"abcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabc",
		"abcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabca",
		"abcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcab",
		"abcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabcabc",
	}
	for _, n := range names {
		if got := len(lead(n, "1-2")); got != 0x60 {
			t.Errorf("len(lead(%s)) = %#x, want %#x", n, got, 0x60)
		}
	}
}

func TestEntry(t *testing.T) {
	testCases := []struct {
		name           string
		value          interface{}
		tag            int
		offset         int
		wantIndexBytes string
		wantData       string
	}{{
		name:           "simple int",
		value:          []int32{0x42},
		tag:            0x010d,
		offset:         5,
		wantIndexBytes: "0000010d000000040000000500000001",
		wantData:       "00000042",
	}, {
		name:           "simple string",
		value:          "simple string",
		tag:            0x010e,
		offset:         0x111,
		wantIndexBytes: "0000010e000000060000011100000001",
		wantData:       "73696d706c6520737472696e6700",
	}, {
		name:           "string array",
		value:          []string{"string", "array"},
		tag:            0x010f,
		offset:         0x222,
		wantIndexBytes: "0000010f000000080000022200000002",
		wantData:       "737472696e6700617272617900",
	}}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var e IndexEntry
			switch v := tc.value.(type) {
			case []string:
				e = EntryStringSlice(v)
			case string:
				e = EntryString(v)
			case []int32:
				e = EntryInt32(v)
			}
			gotBytes := e.indexBytes(tc.tag, tc.offset)
			if d := cmp.Diff(tc.wantIndexBytes, fmt.Sprintf("%x", gotBytes)); d != "" {
				t.Errorf("entry.indexBytes() unexpected value (want->got):\n%s", d)
			}
			if d := cmp.Diff(tc.wantData, fmt.Sprintf("%x", e.data)); d != "" {
				t.Errorf("entry.data unexpected value (want->got):\n%s", d)
			}
		})
	}
}

func TestIndex(t *testing.T) {
	i := newIndex(0x3e)
	i.AddEntries(map[int]IndexEntry{
		0x1111: EntryUint16([]uint16{0x4444, 0x8888, 0xcccc}),
		0x2222: EntryUint32([]uint32{0x3333, 0x5555}),
	})
	got, err := i.Bytes()
	if err != nil {
		t.Errorf("i.Bytes() returned error: %v", err)
	}
	want := "8eade80100000000" + // header lead
		"0000000300000020" + // count and size
		"0000003e000000070000001000000010" + // eigen header entry
		"00001111000000030000000000000003" +
		"00002222000000040000000800000002" +
		"44448888cccc00000000333300005555" + // values, with padding
		"0000003e00000007ffffffd000000010" // eigen header value
	if d := cmp.Diff(want, fmt.Sprintf("%x", got)); d != "" {
		t.Errorf("i.Bytes() unexpected value (want-> got): \n%s", d)
	}
}
