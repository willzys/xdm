package webapi

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	thriftStop   = 0
	thriftBool   = 2
	thriftByte   = 3
	thriftDouble = 4
	thriftI16    = 6
	thriftI32    = 8
	thriftI64    = 10
	thriftString = 11
	thriftStruct = 12
	thriftMap    = 13
	thriftSet    = 14
	thriftList   = 15
)

type thriftValue struct {
	kind   byte
	bytes  []byte
	fields map[int16]thriftValue
}

type thriftReader struct {
	data   []byte
	offset int
}

func classifyXChatEvent(encoded string) (message, encrypted bool, err error) {
	data, err := decodeBase64(encoded)
	if err != nil {
		return false, false, err
	}
	root, err := (&thriftReader{data: data}).readStruct(0)
	if err != nil {
		return false, false, err
	}
	detail, ok := thriftField(root, 7, thriftStruct)
	if !ok {
		return false, false, nil
	}
	create, ok := thriftField(detail, 1, thriftStruct)
	if !ok {
		return false, false, nil
	}
	message = true
	if version, ok := thriftField(create, 101, thriftString); ok && len(version.bytes) > 0 {
		return true, true, nil
	}
	contents, ok := thriftField(create, 100, thriftString)
	if !ok {
		return true, false, errors.New("XChat message has no contents")
	}
	entry, err := (&thriftReader{data: contents.bytes}).readStruct(0)
	if err != nil {
		return true, false, fmt.Errorf("decoding plaintext XChat message: %w", err)
	}
	messageContents, ok := thriftField(entry, 1, thriftStruct)
	if !ok {
		return true, false, nil
	}
	if _, ok := thriftField(messageContents, 1, thriftString); !ok {
		return true, false, errors.New("plaintext XChat message has no text field")
	}
	return true, false, nil
}

func thriftField(value thriftValue, id int16, kind byte) (thriftValue, bool) {
	field, ok := value.fields[id]
	return field, ok && field.kind == kind
}

func decodeBase64(value string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid XChat event encoding")
}

func (r *thriftReader) readStruct(depth int) (thriftValue, error) {
	if depth > 32 {
		return thriftValue{}, errors.New("Thrift nesting limit exceeded")
	}
	value := thriftValue{kind: thriftStruct, fields: make(map[int16]thriftValue)}
	for {
		kind, err := r.readByte()
		if err != nil {
			return thriftValue{}, err
		}
		if kind == thriftStop {
			return value, nil
		}
		id, err := r.readI16()
		if err != nil {
			return thriftValue{}, err
		}
		field, err := r.readValue(kind, depth+1)
		if err != nil {
			return thriftValue{}, err
		}
		value.fields[id] = field
	}
}

func (r *thriftReader) readValue(kind byte, depth int) (thriftValue, error) {
	value := thriftValue{kind: kind}
	switch kind {
	case thriftBool, thriftByte:
		_, err := r.readByte()
		return value, err
	case thriftDouble, thriftI64:
		return value, r.skip(8)
	case thriftI16:
		return value, r.skip(2)
	case thriftI32:
		return value, r.skip(4)
	case thriftString:
		length, err := r.readI32()
		if err != nil {
			return thriftValue{}, err
		}
		if length < 0 || length > 32<<20 {
			return thriftValue{}, errors.New("invalid Thrift string length")
		}
		value.bytes, err = r.readBytes(int(length))
		return value, err
	case thriftStruct:
		return r.readStruct(depth)
	case thriftList, thriftSet:
		elementKind, err := r.readByte()
		if err != nil {
			return thriftValue{}, err
		}
		count, err := r.readI32()
		if err != nil || count < 0 || count > 100000 {
			return thriftValue{}, errors.New("invalid Thrift collection length")
		}
		for range count {
			if _, err := r.readValue(elementKind, depth+1); err != nil {
				return thriftValue{}, err
			}
		}
		return value, nil
	case thriftMap:
		keyKind, err := r.readByte()
		if err != nil {
			return thriftValue{}, err
		}
		valueKind, err := r.readByte()
		if err != nil {
			return thriftValue{}, err
		}
		count, err := r.readI32()
		if err != nil || count < 0 || count > 100000 {
			return thriftValue{}, errors.New("invalid Thrift map length")
		}
		for range count {
			if _, err := r.readValue(keyKind, depth+1); err != nil {
				return thriftValue{}, err
			}
			if _, err := r.readValue(valueKind, depth+1); err != nil {
				return thriftValue{}, err
			}
		}
		return value, nil
	default:
		return thriftValue{}, fmt.Errorf("unsupported Thrift type %d", kind)
	}
}

func (r *thriftReader) readByte() (byte, error) {
	if r.offset >= len(r.data) {
		return 0, errors.New("unexpected end of Thrift data")
	}
	value := r.data[r.offset]
	r.offset++
	return value, nil
}

func (r *thriftReader) readI16() (int16, error) {
	value, err := r.readBytes(2)
	if err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(value)), nil
}

func (r *thriftReader) readI32() (int32, error) {
	value, err := r.readBytes(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(value)), nil
}

func (r *thriftReader) readBytes(length int) ([]byte, error) {
	if length < 0 || r.offset+length > len(r.data) {
		return nil, errors.New("unexpected end of Thrift data")
	}
	value := r.data[r.offset : r.offset+length]
	r.offset += length
	return value, nil
}

func (r *thriftReader) skip(length int) error {
	_, err := r.readBytes(length)
	return err
}
