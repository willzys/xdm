package webapi

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"testing"
)

func TestClassifyXChatEvent(t *testing.T) {
	plaintextContents := thriftTestStruct(
		thriftTestField(thriftStruct, 1, thriftTestStruct(
			thriftTestField(thriftString, 1, thriftTestString("hello")),
		)),
	)
	for _, test := range []struct {
		name      string
		key       string
		encrypted bool
	}{
		{name: "plaintext"},
		{name: "encrypted", key: "key-v1", encrypted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			createFields := [][]byte{thriftTestField(thriftString, 100, thriftTestStringBytes(plaintextContents))}
			if test.key != "" {
				createFields = append(createFields, thriftTestField(thriftString, 101, thriftTestString(test.key)))
			}
			encoded := base64.StdEncoding.EncodeToString(thriftTestStruct(
				thriftTestField(thriftStruct, 7, thriftTestStruct(
					thriftTestField(thriftStruct, 1, thriftTestStruct(createFields...)),
				)),
			))
			message, encrypted, err := classifyXChatEvent(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if !message || encrypted != test.encrypted {
				t.Fatalf("classifyXChatEvent() = message %t, encrypted %t", message, encrypted)
			}
		})
	}
}

func thriftTestStruct(fields ...[]byte) []byte {
	return append(bytes.Join(fields, nil), thriftStop)
}

func thriftTestField(kind byte, id int16, value []byte) []byte {
	result := []byte{kind, byte(id >> 8), byte(id)}
	return append(result, value...)
}

func thriftTestString(value string) []byte {
	return thriftTestStringBytes([]byte(value))
}

func thriftTestStringBytes(value []byte) []byte {
	result := make([]byte, 4, 4+len(value))
	binary.BigEndian.PutUint32(result, uint32(len(value)))
	return append(result, value...)
}
