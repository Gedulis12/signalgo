package libsignalgo

/*
#cgo LDFLAGS: -lsignal_ffi -ldl
#include "./libsignal-ffi.h"
*/
import "C"
import (
	"crypto/rand"
	"unsafe"
)

type Randomness [C.SignalRANDOMNESS_LEN]byte

func GenerateRandomness() (Randomness, error) {
	var randomness Randomness
	_, err := rand.Read(randomness[:])
	return randomness, err
}

type GroupMasterKey [C.SignalGROUP_MASTER_KEY_LEN]byte
type GroupSecretParams [C.SignalGROUP_SECRET_PARAMS_LEN]byte
type GroupPublicParams [C.SignalGROUP_PUBLIC_PARAMS_LEN]byte
type GroupIdentifier [C.SignalGROUP_IDENTIFIER_LEN]byte

type UUIDCiphertext [C.SignalUUID_CIPHERTEXT_LEN]byte
type ProfileKeyCiphertext [C.SignalPROFILE_KEY_CIPHERTEXT_LEN]byte

func GenerateGroupSecretParams() (GroupSecretParams, error) {
	randomness, err := GenerateRandomness()
	if err != nil {
		return GroupSecretParams{}, err
	}
	return GenerateGroupSecretParamsWithRandomness(randomness)
}

func GenerateGroupSecretParamsWithRandomness(randomness Randomness) (GroupSecretParams, error) {
	var params [C.SignalGROUP_SECRET_PARAMS_LEN]C.uchar
	signalFfiError := C.signal_group_secret_params_generate_deterministic(&params, (*[C.SignalRANDOMNESS_LEN]C.uint8_t)(unsafe.Pointer(&randomness)))
	if signalFfiError != nil {
		return GroupSecretParams{}, wrapError(signalFfiError)
	}
	var groupSecretParams GroupSecretParams
	copy(groupSecretParams[:], C.GoBytes(unsafe.Pointer(&params), C.int(C.SignalGROUP_SECRET_PARAMS_LEN)))
	return groupSecretParams, nil
}

func DeriveGroupSecretParamsFromMasterKey(groupMasterKey GroupMasterKey) (GroupSecretParams, error) {
	var params [C.SignalGROUP_SECRET_PARAMS_LEN]C.uchar
	signalFfiError := C.signal_group_secret_params_derive_from_master_key(&params, (*[C.SignalGROUP_MASTER_KEY_LEN]C.uint8_t)(unsafe.Pointer(&groupMasterKey)))
	if signalFfiError != nil {
		return GroupSecretParams{}, wrapError(signalFfiError)
	}
	var groupSecretParams GroupSecretParams
	copy(groupSecretParams[:], C.GoBytes(unsafe.Pointer(&params), C.int(C.SignalGROUP_SECRET_PARAMS_LEN)))
	return groupSecretParams, nil
}

func (gsp *GroupSecretParams) GetPublicParams() (*GroupPublicParams, error) {
	var publicParams [C.SignalGROUP_PUBLIC_PARAMS_LEN]C.uchar
	signalFfiError := C.signal_group_secret_params_get_public_params(&publicParams, (*[C.SignalGROUP_SECRET_PARAMS_LEN]C.uint8_t)(unsafe.Pointer(gsp)))
	if signalFfiError != nil {
		return nil, wrapError(signalFfiError)
	}
	var groupPublicParams GroupPublicParams
	copy(groupPublicParams[:], C.GoBytes(unsafe.Pointer(&publicParams), C.int(C.SignalGROUP_PUBLIC_PARAMS_LEN)))
	return &groupPublicParams, nil
}

func GetGroupIdentifier(groupPublicParams GroupPublicParams) (*GroupIdentifier, error) {
	var groupIdentifier [C.SignalGROUP_IDENTIFIER_LEN]C.uchar
	signalFfiError := C.signal_group_public_params_get_group_identifier(&groupIdentifier, (*[C.SignalGROUP_PUBLIC_PARAMS_LEN]C.uint8_t)(unsafe.Pointer(&groupPublicParams)))
	if signalFfiError != nil {
		return nil, wrapError(signalFfiError)
	}
	var result GroupIdentifier
	copy(result[:], C.GoBytes(unsafe.Pointer(&groupIdentifier), C.int(C.SignalGROUP_IDENTIFIER_LEN)))
	return &result, nil
}

func (gsp *GroupSecretParams) DecryptBlobWithPadding(blob []byte) ([]byte, error) {
	var plaintext C.SignalOwnedBuffer = C.SignalOwnedBuffer{}
	borrowedBlob := BytesToBuffer(blob)
	signalFfiError := C.signal_group_secret_params_decrypt_blob_with_padding(
		&plaintext,
		(*[C.SignalGROUP_SECRET_PARAMS_LEN]C.uint8_t)(unsafe.Pointer(gsp)),
		borrowedBlob,
	)
	if signalFfiError != nil {
		return nil, wrapError(signalFfiError)
	}
	return CopySignalOwnedBufferToBytes(plaintext), nil
}

func (gsp *GroupSecretParams) DecryptUUID(ciphertextUUID UUIDCiphertext) (*UUID, error) {
	uuid := [C.SignalUUID_LEN]C.uchar{}
	signalFfiError := C.signal_group_secret_params_decrypt_uuid(
		&uuid,
		(*[C.SignalGROUP_SECRET_PARAMS_LEN]C.uint8_t)(unsafe.Pointer(gsp)),
		(*[C.SignalUUID_CIPHERTEXT_LEN]C.uint8_t)(unsafe.Pointer(&ciphertextUUID)),
	)
	if signalFfiError != nil {
		return nil, wrapError(signalFfiError)
	}
	var result UUID
	copy(result[:], C.GoBytes(unsafe.Pointer(&uuid), C.int(C.SignalUUID_LEN)))
	return &result, nil
}

func (gsp *GroupSecretParams) DecryptProfileKey(ciphertextProfileKey ProfileKeyCiphertext, uuid UUID) (*ProfileKey, error) {
	profileKey := [C.SignalPROFILE_KEY_LEN]C.uchar{}
	signalFfiError := C.signal_group_secret_params_decrypt_profile_key(
		&profileKey,
		(*[C.SignalGROUP_SECRET_PARAMS_LEN]C.uint8_t)(unsafe.Pointer(gsp)),
		(*[C.SignalPROFILE_KEY_CIPHERTEXT_LEN]C.uint8_t)(unsafe.Pointer(&ciphertextProfileKey)),
		(*[C.SignalUUID_LEN]C.uint8_t)(unsafe.Pointer(&uuid)),
	)
	if signalFfiError != nil {
		return nil, wrapError(signalFfiError)
	}
	var result ProfileKey
	copy(result[:], C.GoBytes(unsafe.Pointer(&profileKey), C.int(C.SignalPROFILE_KEY_LEN)))
	return &result, nil
}
