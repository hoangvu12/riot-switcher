package secure

import (
	"encoding/base64"
	"unsafe"

	"golang.org/x/sys/windows"
)

func EncryptString(value string) (string, error) {
	out, err := cryptProtectData([]byte(value))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

func DecryptString(value string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	out, err := cryptUnprotectData(data)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func cryptProtectData(data []byte) ([]byte, error) {
	in := bytesToBlob(data)
	var out windows.DataBlob
	if err := windows.CryptProtectData(in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return blobToBytes(&out), nil
}

func cryptUnprotectData(data []byte) ([]byte, error) {
	in := bytesToBlob(data)
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return blobToBytes(&out), nil
}

func bytesToBlob(data []byte) *windows.DataBlob {
	if len(data) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

func blobToBytes(blob *windows.DataBlob) []byte {
	if blob == nil || blob.Data == nil || blob.Size == 0 {
		return nil
	}
	data := unsafe.Slice(blob.Data, blob.Size)
	copyData := make([]byte, len(data))
	copy(copyData, data)
	return copyData
}
