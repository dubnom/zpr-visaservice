package enrollment

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const endpointHelperIPCMaxFrame = 8192

var errEndpointHelperIPCProtocol = errors.New("invalid endpoint helper IPC message")

type endpointHelperIPCRequest struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	Digest    string `json:"digest,omitempty"`
}

type endpointHelperIPCResponse struct {
	Version     int    `json:"version"`
	Operation   string `json:"operation"`
	Reference   string `json:"reference,omitempty"`
	PublicKey   string `json:"public_key,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Signature   string `json:"signature,omitempty"`
	Error       string `json:"error,omitempty"`
}

type endpointHelperIPCClientPolicy struct {
	ExpectedReference     string
	PublicKeyDER          []byte
	HelperCodeRequirement string
	HelperUID             int
	RoundTripTimeout      time.Duration
}

type endpointHelperIPCSigner struct {
	mu       sync.Mutex
	ctx      context.Context
	conn     net.Conn
	policy   endpointHelperIPCClientPolicy
	verifier endpointHelperPeerVerifier
	key      *rsa.PublicKey
	stop     func() bool
}

var _ crypto.Signer = (*endpointHelperIPCSigner)(nil)

func endpointHelperIPCReference(reference string) bool {
	if reference == "" || len(reference) > 1024 || !utf8.ValidString(reference) {
		return false
	}
	for _, r := range reference {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func endpointHelperIPCTimeout(timeout time.Duration) bool {
	return timeout > 0 && timeout <= 30*time.Second
}

func endpointHelperIPCDeadline(ctx context.Context, conn net.Conn, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	return conn.SetDeadline(deadline)
}

func endpointHelperIPCWrite(conn net.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > endpointHelperIPCMaxFrame {
		return errEndpointHelperIPCProtocol
	}
	frame := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(frame, uint32(len(data)))
	copy(frame[4:], data)
	for len(frame) > 0 {
		n, err := conn.Write(frame)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		frame = frame[n:]
	}
	return nil
}

// Decode the object explicitly: encoding/json alone accepts duplicate fields.
func endpointHelperIPCRead(conn net.Conn, value any) (map[string]json.RawMessage, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > endpointHelperIPCMaxFrame {
		return nil, errEndpointHelperIPCProtocol
	}
	data := make([]byte, int(size))
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, errEndpointHelperIPCProtocol
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errEndpointHelperIPCProtocol
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err = decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return nil, errEndpointHelperIPCProtocol
		}
		if _, exists := fields[name]; exists {
			return nil, errEndpointHelperIPCProtocol
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil || bytes.Equal(raw, []byte("null")) {
			return nil, errEndpointHelperIPCProtocol
		}
		fields[name] = raw
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errEndpointHelperIPCProtocol
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errEndpointHelperIPCProtocol
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return nil, errEndpointHelperIPCProtocol
	}
	return fields, nil
}

func endpointHelperIPCFields(fields map[string]json.RawMessage, names ...string) bool {
	if len(fields) != len(names) {
		return false
	}
	for _, name := range names {
		if _, exists := fields[name]; !exists {
			return false
		}
	}
	return true
}

func endpointHelperIPCBase64(text string, size int) ([]byte, error) {
	data, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil || len(data) != size || base64.StdEncoding.EncodeToString(data) != text {
		return nil, errEndpointHelperIPCProtocol
	}
	return data, nil
}

func endpointHelperIPCPublic(der []byte) (*rsa.PublicKey, error) {
	if len(der) == 0 || len(der) > 1024 {
		return nil, errEndpointHelperIPCProtocol
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, errEndpointHelperIPCProtocol
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok || key.N == nil || key.N.BitLen() != 3072 || key.E != 65537 {
		return nil, errEndpointHelperIPCProtocol
	}
	canonical, err := x509.MarshalPKIXPublicKey(key)
	if err != nil || !bytes.Equal(canonical, der) {
		return nil, errEndpointHelperIPCProtocol
	}
	return key, nil
}

// The owner supplies an already accepted connection; this never installs a
// listener. Public uses existing RuntimeKey create/resume semantics, including
// reservation-before-create and unchanged native gates. There are no retries.
// Every failure is returned to the owning service, even when a sanitized
// unavailable response is sent to the authenticated peer.
func serveEndpointHelperIPCConnection(ctx context.Context, conn net.Conn, helper *endpointHelperSource, timeout time.Duration) error {
	if ctx == nil || conn == nil {
		return ErrInvalid
	}
	defer conn.Close()
	if helper == nil || helper.verifier == nil || !endpointHelperIPCTimeout(timeout) {
		return ErrInvalid
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	for {
		if err := endpointHelperIPCDeadline(ctx, conn, timeout); err != nil {
			return err
		}
		if err := helper.verifier.VerifyEndpointHelperPeer(ctx, conn, helper.policy); err != nil {
			return fmt.Errorf("%w: %v", errEndpointHelperPeerDenied, err)
		}
		var request endpointHelperIPCRequest
		fields, err := endpointHelperIPCRead(conn, &request)
		if err != nil {
			return err
		}
		if request.Version != 1 {
			return errEndpointHelperIPCProtocol
		}
		var digest []byte
		switch request.Operation {
		case "public":
			if !endpointHelperIPCFields(fields, "version", "operation") {
				return errEndpointHelperIPCProtocol
			}
		case "sign":
			if !endpointHelperIPCFields(fields, "version", "operation", "digest") {
				return errEndpointHelperIPCProtocol
			}
			digest, err = endpointHelperIPCBase64(request.Digest, sha256.Size)
			if err != nil {
				return err
			}
		default:
			return errEndpointHelperIPCProtocol
		}
		response, err := endpointHelperIPCOperation(ctx, helper, conn, request.Operation, digest)
		if err != nil {
			if errors.Is(err, errEndpointHelperPeerDenied) {
				return err
			}
			writeErr := endpointHelperIPCWrite(conn, endpointHelperIPCResponse{
				Version: 1, Operation: request.Operation, Error: "unavailable",
			})
			return errors.Join(err, writeErr)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := endpointHelperIPCWrite(conn, response); err != nil {
			return err
		}
	}
}

func endpointHelperIPCOperation(ctx context.Context, helper *endpointHelperSource, conn net.Conn, operation string, digest []byte) (endpointHelperIPCResponse, error) {
	key, err := helper.RuntimeKey(ctx, conn)
	if err != nil {
		return endpointHelperIPCResponse{}, err
	}
	der, err := x509.MarshalPKIXPublicKey(key.PublicKey)
	if err != nil {
		return endpointHelperIPCResponse{}, err
	}
	if _, err := endpointHelperIPCPublic(der); err != nil {
		return endpointHelperIPCResponse{}, err
	}
	sum := sha256.Sum256(der)
	fingerprint := hex.EncodeToString(sum[:])
	if !endpointHelperIPCReference(key.Reference) || key.Reference != helper.policy.KeyReference || fingerprint != key.Fingerprint {
		return endpointHelperIPCResponse{}, errFixtureKeyBindingMismatch
	}
	response := endpointHelperIPCResponse{Version: 1, Operation: operation,
		Reference: key.Reference, PublicKey: base64.StdEncoding.EncodeToString(der), Fingerprint: fingerprint}
	if operation == "sign" {
		signature, err := key.signer.Sign(rand.Reader, digest, crypto.SHA256)
		if err != nil {
			return endpointHelperIPCResponse{}, err
		}
		if len(signature) != 384 {
			return endpointHelperIPCResponse{}, errFixtureInvalidSigner
		}
		if err := rsa.VerifyPKCS1v15(key.PublicKey, crypto.SHA256, digest, signature); err != nil {
			return endpointHelperIPCResponse{}, err
		}
		response.Signature = base64.StdEncoding.EncodeToString(signature)
	}
	return response, nil
}

// The verifier parameter is an internal fake-test seam. Production construction
// is restricted to the Darwin constructor with its kernel-backed verifier.
func newEndpointHelperIPCSigner(ctx context.Context, conn net.Conn, policy endpointHelperIPCClientPolicy, verifier endpointHelperPeerVerifier) (*endpointHelperIPCSigner, error) {
	if ctx == nil || conn == nil {
		return nil, ErrInvalid
	}
	fail := func(err error) (*endpointHelperIPCSigner, error) { _ = conn.Close(); return nil, err }
	if verifier == nil || !endpointHelperIPCReference(policy.ExpectedReference) ||
		!validCodeRequirement(policy.HelperCodeRequirement) || policy.HelperUID < 0 ||
		uint64(policy.HelperUID) > 1<<32-1 || !endpointHelperIPCTimeout(policy.RoundTripTimeout) {
		return fail(ErrInvalid)
	}
	policy.PublicKeyDER = bytes.Clone(policy.PublicKeyDER)
	key, err := endpointHelperIPCPublic(policy.PublicKeyDER)
	if err != nil {
		return fail(err)
	}
	signer := &endpointHelperIPCSigner{ctx: ctx, conn: conn, policy: policy, verifier: verifier, key: key}
	signer.stop = context.AfterFunc(ctx, func() { _ = conn.Close() })
	if _, err := signer.roundTrip("public", nil); err != nil {
		_ = signer.Close()
		return nil, err
	}
	return signer, nil
}

func (s *endpointHelperIPCSigner) Public() crypto.PublicKey {
	return &rsa.PublicKey{N: new(big.Int).Set(s.key.N), E: s.key.E}
}

func (s *endpointHelperIPCSigner) Close() error {
	if s.stop != nil {
		s.stop()
	}
	return s.conn.Close()
}

func (s *endpointHelperIPCSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if opts != crypto.SHA256 || len(digest) != sha256.Size {
		return nil, errEndpointHelperIPCProtocol
	}
	return s.roundTrip("sign", bytes.Clone(digest))
}

func (s *endpointHelperIPCSigner) roundTrip(operation string, digest []byte) (signature []byte, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	if err = endpointHelperIPCDeadline(s.ctx, s.conn, s.policy.RoundTripTimeout); err != nil {
		return nil, err
	}
	peerPolicy := fixtureEndpointKeyPolicy{CodeRequirement: s.policy.HelperCodeRequirement, UID: s.policy.HelperUID}
	if err = s.verifier.VerifyEndpointHelperPeer(s.ctx, s.conn, peerPolicy); err != nil {
		return nil, fmt.Errorf("%w: %v", errEndpointHelperPeerDenied, err)
	}
	request := endpointHelperIPCRequest{Version: 1, Operation: operation}
	if operation == "sign" {
		request.Digest = base64.StdEncoding.EncodeToString(digest)
	}
	if err = endpointHelperIPCWrite(s.conn, request); err != nil {
		return nil, err
	}
	var response endpointHelperIPCResponse
	fields, err := endpointHelperIPCRead(s.conn, &response)
	if err != nil {
		return nil, err
	}
	if response.Version != 1 || response.Operation != operation {
		return nil, errEndpointHelperIPCProtocol
	}
	if endpointHelperIPCFields(fields, "version", "operation", "error") && response.Error == "unavailable" {
		return nil, errors.New("endpoint helper unavailable")
	}
	names := []string{"version", "operation", "reference", "public_key", "fingerprint"}
	if operation == "sign" {
		names = append(names, "signature")
	}
	if !endpointHelperIPCFields(fields, names...) || response.Reference != s.policy.ExpectedReference ||
		response.PublicKey != base64.StdEncoding.EncodeToString(s.policy.PublicKeyDER) {
		return nil, errEndpointHelperIPCProtocol
	}
	sum := sha256.Sum256(s.policy.PublicKeyDER)
	if response.Fingerprint != hex.EncodeToString(sum[:]) {
		return nil, errEndpointHelperIPCProtocol
	}
	if operation == "public" {
		return nil, nil
	}
	signature, err = endpointHelperIPCBase64(response.Signature, 384)
	if err != nil {
		return nil, err
	}
	if err = rsa.VerifyPKCS1v15(s.key, crypto.SHA256, digest, signature); err != nil {
		return nil, err
	}
	return signature, nil
}
