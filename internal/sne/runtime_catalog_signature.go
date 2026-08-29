package sne

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

// VerifyRuntimeCatalogSignature authenticates the exact catalog bytes before
// any selection or package path is trusted. The private key never belongs in
// the catalog, package, or repository; Pantheon receives only a pinned public
// key and detached signature.
func VerifyRuntimeCatalogSignature(catalogPath, signaturePath, publicKeyPath string) error {
	return verifyDetachedEd25519("SNE runtime catalog", catalogPath, signaturePath, publicKeyPath)
}

func verifyDetachedEd25519(label, messagePath, signaturePath, publicKeyPath string) error {
	message, err := os.ReadFile(messagePath)
	if err != nil {
		return fmt.Errorf("read %s for signature verification: %w", label, err)
	}
	signatureText, err := os.ReadFile(signaturePath)
	if err != nil {
		return fmt.Errorf("read %s signature: %w", label, err)
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(signatureText)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("decode %s signature", label)
	}
	publicPEM, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("read trusted SNE catalog public key: %w", err)
	}
	block, rest := pem.Decode(publicPEM)
	if block == nil || block.Type != "PUBLIC KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return fmt.Errorf("decode trusted SNE catalog public key PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse trusted SNE catalog public key: %w", err)
	}
	publicKey, ok := parsed.(ed25519.PublicKey)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("trusted SNE catalog key is not Ed25519")
	}
	if !ed25519.Verify(publicKey, message, signature) {
		return fmt.Errorf("%s signature mismatch", label)
	}
	return nil
}

func LoadSignedRuntimePackageCatalog(catalogPath, signaturePath, publicKeyPath string) (RuntimePackageCatalog, error) {
	if err := VerifyRuntimeCatalogSignature(catalogPath, signaturePath, publicKeyPath); err != nil {
		return RuntimePackageCatalog{}, err
	}
	return LoadRuntimePackageCatalog(catalogPath)
}
