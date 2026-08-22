package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var catalogPath, privateKeyPath, publicKeyPath, signaturePath string
	var generate bool
	flag.StringVar(&catalogPath, "catalog", "", "exact runtime catalog JSON to sign")
	flag.StringVar(&privateKeyPath, "private-key", "", "PKCS#8 Ed25519 private key PEM")
	flag.StringVar(&publicKeyPath, "public-key", "", "PKIX Ed25519 public key PEM output")
	flag.StringVar(&signaturePath, "signature", "", "detached base64 signature output")
	flag.BoolVar(&generate, "generate", false, "generate a new key pair; refuses overwrite")
	flag.Parse()
	if catalogPath == "" || privateKeyPath == "" || publicKeyPath == "" || signaturePath == "" {
		fatalf("catalog, private-key, public-key, and signature are required")
	}
	if generate {
		if err := generateKeyPair(privateKeyPath, publicKeyPath); err != nil {
			fatalf("generate signing identity: %v", err)
		}
	}
	privateKey, publicKey, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		fatalf("load signing identity: %v", err)
	}
	if err := writePublicKey(publicKeyPath, publicKey, false); err != nil {
		fatalf("write trusted public key: %v", err)
	}
	catalog, err := os.ReadFile(catalogPath)
	if err != nil {
		fatalf("read catalog: %v", err)
	}
	signature := signatureText(privateKey, catalog)
	if err := writeAtomic(signaturePath, []byte(signature), 0o600); err != nil {
		fatalf("write signature: %v", err)
	}
	fmt.Printf("signed catalog=%s bytes=%d signature=%s public_key=%s\n", catalogPath, len(catalog), signaturePath, publicKeyPath)
}

func generateKeyPair(privatePath, publicPath string) error {
	for label, path := range map[string]string{"private key": privatePath, "public key": publicPath} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("%s already exists: %s", label, path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return err
	}
	if err := writeAtomic(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		return err
	}
	return writePublicKey(publicPath, publicKey, true)
}

func signatureText(privateKey ed25519.PrivateKey, message []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message)) + "\n"
}

func loadPrivateKey(path string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" || len(rest) != 0 {
		return nil, nil, fmt.Errorf("invalid PKCS#8 private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("private key is not Ed25519")
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("derive Ed25519 public key")
	}
	return privateKey, publicKey, nil
}

func writePublicKey(path string, publicKey ed25519.PublicKey, exclusive bool) error {
	if exclusive {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("public key already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return err
	}
	return writeAtomic(path, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o600)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sne-sign-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sirsi-sne-catalog-sign: "+format+"\n", args...)
	os.Exit(1)
}
