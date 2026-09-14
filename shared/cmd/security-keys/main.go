// Command security-keys generates development signing keys without printing them.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: security-keys OUTPUT_DIRECTORY")
		os.Exit(1)
	}
	if e := run(os.Args[1]); e != nil {
		fmt.Fprintln(os.Stderr, "key generation failed:", e)
		os.Exit(1)
	}
}
func run(dir string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	key, e := rsa.GenerateKey(rand.Reader, 3072)
	if e != nil {
		return e
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pub, e := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if e != nil {
		return e
	}
	encoded, e := json.MarshalIndent(map[string]string{"development-1": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))}, "", "  ")
	if e != nil {
		return e
	}
	for _, f := range []struct {
		name string
		data []byte
	}{{"private.pem", private}, {"public-keys.json", encoded}} {
		out, e := os.OpenFile(filepath.Join(dir, f.name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = out.Write(f.data)
		ce := out.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	return nil
}
