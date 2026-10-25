package auditlog

import (
	"reflect"
	"testing"
)

func TestByteOffsetsPreserveCRLFAndUTF8(t *testing.T) {
	lines, offsets, next := splitCompleteLinesWithOffsets("第一\r\n\r\nsecond\n\xe4\xb8", 100)
	if !reflect.DeepEqual(lines, []string{"第一", "second"}) {
		t.Fatalf("lines: %v", lines)
	}
	if !reflect.DeepEqual(offsets, []int64{100, 110}) || next != 117 {
		t.Fatalf("offsets %v next %d", offsets, next)
	}
}
