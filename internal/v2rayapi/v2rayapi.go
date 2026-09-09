// Package v2rayapi implements the subset of the generic v2ray stats gRPC
// protocol (v2ray.core.app.stats.command.StatsService) used by the agent.
// Message types and wire encoding are hand-written against the upstream
// stats.proto so that no protobuf generator or GPL dependency is needed.
package v2rayapi

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/encoding"
)

type Stat struct {
	Name  string
	Value int64
}

func (s *Stat) GetName() string {
	if s == nil {
		return ""
	}
	return s.Name
}

func (s *Stat) GetValue() int64 {
	if s == nil {
		return 0
	}
	return s.Value
}

type GetStatsRequest struct {
	Name  string
	Reset bool
}

type GetStatsResponse struct {
	Stat *Stat
}

type QueryStatsRequest struct {
	Pattern  string
	Reset    bool
	Patterns []string
	Regexp   bool
}

func (r *QueryStatsRequest) GetPattern() string {
	if r == nil {
		return ""
	}
	return r.Pattern
}

func (r *QueryStatsRequest) GetPatterns() []string {
	if r == nil {
		return nil
	}
	return r.Patterns
}

type QueryStatsResponse struct {
	Stat []*Stat
}

func (r *QueryStatsResponse) GetStat() []*Stat {
	if r == nil {
		return nil
	}
	return r.Stat
}

type SysStatsRequest struct{}

type SysStatsResponse struct {
	NumGoroutine uint32
	NumGC        uint32
	Alloc        uint64
	TotalAlloc   uint64
	Sys          uint64
	Mallocs      uint64
	Frees        uint64
	LiveObjects  uint64
	PauseTotalNs uint64
	Uptime       uint32
}

var errTruncated = errors.New("v2rayapi: truncated protobuf message")

const (
	wireVarint     = 0
	wireFixed64    = 1
	wireBytes      = 2
	wireStartGroup = 3
	wireEndGroup   = 4
	wireFixed32    = 5
)

func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func appendTag(b []byte, field int, wire byte) []byte {
	return appendVarint(b, uint64(field)<<3|uint64(wire))
}

func appendStringField(b []byte, field int, s string) []byte {
	if s == "" {
		return b
	}
	b = appendTag(b, field, wireBytes)
	b = appendVarint(b, uint64(len(s)))
	return append(b, s...)
}

func appendStringElement(b []byte, field int, s string) []byte {
	b = appendTag(b, field, wireBytes)
	b = appendVarint(b, uint64(len(s)))
	return append(b, s...)
}

func appendMessageField(b []byte, field int, data []byte) []byte {
	b = appendTag(b, field, wireBytes)
	b = appendVarint(b, uint64(len(data)))
	return append(b, data...)
}

func appendBoolField(b []byte, field int, v bool) []byte {
	if !v {
		return b
	}
	b = appendTag(b, field, wireVarint)
	return appendVarint(b, 1)
}

func appendInt64Field(b []byte, field int, v int64) []byte {
	if v == 0 {
		return b
	}
	b = appendTag(b, field, wireVarint)
	return appendVarint(b, uint64(v))
}

func appendUint64Field(b []byte, field int, v uint64) []byte {
	if v == 0 {
		return b
	}
	b = appendTag(b, field, wireVarint)
	return appendVarint(b, v)
}

func (s *Stat) Marshal() []byte {
	var b []byte
	b = appendStringField(b, 1, s.Name)
	b = appendInt64Field(b, 2, s.Value)
	return b
}

func (s *Stat) Unmarshal(data []byte) error {
	d := &decoder{data: data}
	for len(d.data) > 0 {
		tag, err := d.varint()
		if err != nil {
			return err
		}
		field, wire := int(tag>>3), byte(tag&7)
		switch {
		case field == 1 && wire == wireBytes:
			v, err := d.bytes()
			if err != nil {
				return err
			}
			s.Name = string(v)
		case field == 2 && wire == wireVarint:
			v, err := d.varint()
			if err != nil {
				return err
			}
			s.Value = int64(v)
		default:
			if err := d.skip(wire); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *GetStatsRequest) Marshal() []byte {
	var b []byte
	b = appendStringField(b, 1, r.Name)
	b = appendBoolField(b, 2, r.Reset)
	return b
}

func (r *GetStatsRequest) Unmarshal(data []byte) error {
	d := &decoder{data: data}
	for len(d.data) > 0 {
		tag, err := d.varint()
		if err != nil {
			return err
		}
		field, wire := int(tag>>3), byte(tag&7)
		switch {
		case field == 1 && wire == wireBytes:
			v, err := d.bytes()
			if err != nil {
				return err
			}
			r.Name = string(v)
		case field == 2 && wire == wireVarint:
			v, err := d.varint()
			if err != nil {
				return err
			}
			r.Reset = v != 0
		default:
			if err := d.skip(wire); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *GetStatsResponse) Marshal() []byte {
	var b []byte
	if r.Stat != nil {
		b = appendMessageField(b, 1, r.Stat.Marshal())
	}
	return b
}

func (r *GetStatsResponse) Unmarshal(data []byte) error {
	d := &decoder{data: data}
	for len(d.data) > 0 {
		tag, err := d.varint()
		if err != nil {
			return err
		}
		field, wire := int(tag>>3), byte(tag&7)
		switch {
		case field == 1 && wire == wireBytes:
			v, err := d.bytes()
			if err != nil {
				return err
			}
			r.Stat = &Stat{}
			if err := r.Stat.Unmarshal(v); err != nil {
				return err
			}
		default:
			if err := d.skip(wire); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *QueryStatsRequest) Marshal() []byte {
	var b []byte
	b = appendStringField(b, 1, r.Pattern)
	b = appendBoolField(b, 2, r.Reset)
	for _, pattern := range r.Patterns {
		b = appendStringElement(b, 3, pattern)
	}
	b = appendBoolField(b, 4, r.Regexp)
	return b
}

func (r *QueryStatsRequest) Unmarshal(data []byte) error {
	d := &decoder{data: data}
	for len(d.data) > 0 {
		tag, err := d.varint()
		if err != nil {
			return err
		}
		field, wire := int(tag>>3), byte(tag&7)
		switch {
		case field == 1 && wire == wireBytes:
			v, err := d.bytes()
			if err != nil {
				return err
			}
			r.Pattern = string(v)
		case field == 2 && wire == wireVarint:
			v, err := d.varint()
			if err != nil {
				return err
			}
			r.Reset = v != 0
		case field == 3 && wire == wireBytes:
			v, err := d.bytes()
			if err != nil {
				return err
			}
			r.Patterns = append(r.Patterns, string(v))
		case field == 4 && wire == wireVarint:
			v, err := d.varint()
			if err != nil {
				return err
			}
			r.Regexp = v != 0
		default:
			if err := d.skip(wire); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *QueryStatsResponse) Marshal() []byte {
	var b []byte
	for _, stat := range r.Stat {
		b = appendMessageField(b, 1, stat.Marshal())
	}
	return b
}

func (r *QueryStatsResponse) Unmarshal(data []byte) error {
	d := &decoder{data: data}
	for len(d.data) > 0 {
		tag, err := d.varint()
		if err != nil {
			return err
		}
		field, wire := int(tag>>3), byte(tag&7)
		switch {
		case field == 1 && wire == wireBytes:
			v, err := d.bytes()
			if err != nil {
				return err
			}
			stat := &Stat{}
			if err := stat.Unmarshal(v); err != nil {
				return err
			}
			r.Stat = append(r.Stat, stat)
		default:
			if err := d.skip(wire); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *SysStatsRequest) Marshal() []byte {
	return nil
}

func (r *SysStatsRequest) Unmarshal(data []byte) error {
	d := &decoder{data: data}
	for len(d.data) > 0 {
		tag, err := d.varint()
		if err != nil {
			return err
		}
		if err := d.skip(byte(tag & 7)); err != nil {
			return err
		}
	}
	return nil
}

func (r *SysStatsResponse) Marshal() []byte {
	var b []byte
	b = appendUint64Field(b, 1, uint64(r.NumGoroutine))
	b = appendUint64Field(b, 2, uint64(r.NumGC))
	b = appendUint64Field(b, 3, r.Alloc)
	b = appendUint64Field(b, 4, r.TotalAlloc)
	b = appendUint64Field(b, 5, r.Sys)
	b = appendUint64Field(b, 6, r.Mallocs)
	b = appendUint64Field(b, 7, r.Frees)
	b = appendUint64Field(b, 8, r.LiveObjects)
	b = appendUint64Field(b, 9, r.PauseTotalNs)
	b = appendUint64Field(b, 10, uint64(r.Uptime))
	return b
}

func (r *SysStatsResponse) Unmarshal(data []byte) error {
	d := &decoder{data: data}
	for len(d.data) > 0 {
		tag, err := d.varint()
		if err != nil {
			return err
		}
		field, wire := int(tag>>3), byte(tag&7)
		if wire != wireVarint {
			if err := d.skip(wire); err != nil {
				return err
			}
			continue
		}
		v, err := d.varint()
		if err != nil {
			return err
		}
		switch field {
		case 1:
			r.NumGoroutine = uint32(v)
		case 2:
			r.NumGC = uint32(v)
		case 3:
			r.Alloc = v
		case 4:
			r.TotalAlloc = v
		case 5:
			r.Sys = v
		case 6:
			r.Mallocs = v
		case 7:
			r.Frees = v
		case 8:
			r.LiveObjects = v
		case 9:
			r.PauseTotalNs = v
		case 10:
			r.Uptime = uint32(v)
		}
	}
	return nil
}

type decoder struct {
	data []byte
}

func (d *decoder) varint() (uint64, error) {
	var v uint64
	var shift uint
	for {
		if len(d.data) == 0 {
			return 0, errTruncated
		}
		b := d.data[0]
		d.data = d.data[1:]
		v |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return v, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, errors.New("v2rayapi: varint overflows 64 bits")
		}
	}
}

func (d *decoder) bytes() ([]byte, error) {
	n, err := d.varint()
	if err != nil {
		return nil, err
	}
	if n > uint64(len(d.data)) {
		return nil, errTruncated
	}
	v := d.data[:n]
	d.data = d.data[n:]
	return v, nil
}

func (d *decoder) skip(wire byte) error {
	switch wire {
	case wireVarint:
		_, err := d.varint()
		return err
	case wireFixed64:
		if len(d.data) < 8 {
			return errTruncated
		}
		d.data = d.data[8:]
		return nil
	case wireBytes:
		_, err := d.bytes()
		return err
	case wireFixed32:
		if len(d.data) < 4 {
			return errTruncated
		}
		d.data = d.data[4:]
		return nil
	default:
		return fmt.Errorf("v2rayapi: unsupported wire type %d", wire)
	}
}

// Codec adapts the hand-written message types to the grpc wire protocol under
// the standard "proto" content subtype.
type Codec struct{}

func (Codec) Marshal(v any) ([]byte, error) {
	switch msg := v.(type) {
	case *Stat:
		return msg.Marshal(), nil
	case *GetStatsRequest:
		return msg.Marshal(), nil
	case *GetStatsResponse:
		return msg.Marshal(), nil
	case *QueryStatsRequest:
		return msg.Marshal(), nil
	case *QueryStatsResponse:
		return msg.Marshal(), nil
	case *SysStatsRequest:
		return msg.Marshal(), nil
	case *SysStatsResponse:
		return msg.Marshal(), nil
	default:
		return nil, fmt.Errorf("v2rayapi: unsupported message type %T", v)
	}
}

func (Codec) Unmarshal(data []byte, v any) error {
	switch msg := v.(type) {
	case *Stat:
		return msg.Unmarshal(data)
	case *GetStatsRequest:
		return msg.Unmarshal(data)
	case *GetStatsResponse:
		return msg.Unmarshal(data)
	case *QueryStatsRequest:
		return msg.Unmarshal(data)
	case *QueryStatsResponse:
		return msg.Unmarshal(data)
	case *SysStatsRequest:
		return msg.Unmarshal(data)
	case *SysStatsResponse:
		return msg.Unmarshal(data)
	default:
		return fmt.Errorf("v2rayapi: unsupported message type %T", v)
	}
}

func (Codec) Name() string {
	return "proto"
}

var _ encoding.Codec = Codec{}
