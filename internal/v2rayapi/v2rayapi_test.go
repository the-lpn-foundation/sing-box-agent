package v2rayapi

import (
	"reflect"
	"testing"
)

func TestStatExactWireBytes(t *testing.T) {
	// Pins field numbers/wire types of the v2ray stats proto: name=1 (string),
	// value=2 (int64 varint).
	got := (&Stat{Name: "ab", Value: 300}).Marshal()
	want := []byte{0x0a, 0x02, 'a', 'b', 0x10, 0xac, 0x02}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Stat.Marshal() = %x, want %x", got, want)
	}
}

func TestQueryStatsRequestExactWireBytes(t *testing.T) {
	// Pins field numbers: pattern=1, reset=2, patterns=3, regexp=4.
	got := (&QueryStatsRequest{Patterns: []string{"x"}}).Marshal()
	want := []byte{0x1a, 0x01, 'x'}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryStatsRequest.Marshal() = %x, want %x", got, want)
	}

	got = (&QueryStatsRequest{Pattern: "p", Reset: true, Patterns: []string{"a", "b"}, Regexp: true}).Marshal()
	want = []byte{0x0a, 0x01, 'p', 0x10, 0x01, 0x1a, 0x01, 'a', 0x1a, 0x01, 'b', 0x20, 0x01}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryStatsRequest.Marshal() = %x, want %x", got, want)
	}
}

func TestQueryStatsResponseExactWireBytes(t *testing.T) {
	// Pins field numbers: stat=1 (repeated message), Stat.name=1, Stat.value=2.
	got := (&QueryStatsResponse{Stat: []*Stat{{Name: "n", Value: 1}}}).Marshal()
	want := []byte{0x0a, 0x05, 0x0a, 0x01, 'n', 0x10, 0x01}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryStatsResponse.Marshal() = %x, want %x", got, want)
	}
}

func TestMessageRoundTrip(t *testing.T) {
	msgs := []interface {
		Marshal() []byte
		Unmarshal(data []byte) error
	}{
		&Stat{Name: "user>>>u1>>>traffic>>>uplink", Value: 1 << 40},
		&Stat{Name: "", Value: 0},
		&GetStatsRequest{Name: "counter", Reset: true},
		&GetStatsResponse{Stat: &Stat{Name: "counter", Value: -1}},
		&QueryStatsRequest{Pattern: "p", Reset: true, Patterns: []string{"a", "b"}, Regexp: true},
		&QueryStatsRequest{},
		&QueryStatsResponse{Stat: []*Stat{{Name: "n1", Value: 5}, {Name: "n2", Value: 1 << 60}}},
		&QueryStatsResponse{},
		&SysStatsResponse{NumGoroutine: 9, NumGC: 10, Alloc: 11, TotalAlloc: 12, Sys: 13, Mallocs: 14, Frees: 15, LiveObjects: 16, PauseTotalNs: 17, Uptime: 18},
	}

	for _, msg := range msgs {
		data := msg.Marshal()
		typ := reflect.TypeOf(msg).Elem()
		dst := reflect.New(typ).Interface().(interface{ Unmarshal(data []byte) error })
		if err := dst.Unmarshal(data); err != nil {
			t.Fatalf("%T.Unmarshal(%x): %v", msg, data, err)
		}
		if !reflect.DeepEqual(msg, dst) {
			t.Fatalf("%T round trip: got %+v, want %+v", msg, dst, msg)
		}
	}
}

func TestUnmarshalSkipsUnknownFields(t *testing.T) {
	// A forward-compatible server may add fields; decoding must ignore them.
	data := []byte{0x0a, 0x01, 'n', 0x10, 0x07, 0x19, 0, 0, 0, 0, 0, 0, 0, 0, 0x25, 0, 0, 0, 0, 0x38, 0x01}
	stat := &Stat{}
	if err := stat.Unmarshal(data); err != nil {
		t.Fatalf("Stat.Unmarshal: %v", err)
	}
	if stat.Name != "n" || stat.Value != 7 {
		t.Fatalf("got %+v, want {n 7}", stat)
	}
}

func TestUnmarshalTruncated(t *testing.T) {
	stat := &Stat{}
	if err := stat.Unmarshal([]byte{0x0a, 0x05, 'a'}); err == nil {
		t.Fatal("expected error for truncated length-delimited field")
	}
	if err := stat.Unmarshal([]byte{0x10}); err == nil {
		t.Fatal("expected error for truncated varint")
	}
}

func TestCodecMarshalUnsupportedType(t *testing.T) {
	if _, err := (Codec{}).Marshal(struct{}{}); err == nil {
		t.Fatal("expected error for unsupported message type")
	}
}
