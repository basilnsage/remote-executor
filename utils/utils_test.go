package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNewSSHConfig(t *testing.T) {
	// create temp private key file
	tempKey := fmt.Sprintf("%s/temp-key.pem", os.TempDir())
	pkey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pkeyBytes := x509.MarshalPKCS1PrivateKey(pkey)
	pkeyPEM := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   pkeyBytes,
	}
	_ = ioutil.WriteFile(tempKey, pem.EncodeToMemory(&pkeyPEM), 0600)

	conf, err := NewSSHConfig(false, "/dev/null", tempKey, "foobar")
	if err != nil {
		t.Fatalf("NewSSHConfig: %v", err)
	}
	if got, want := conf.User, "foobar"; got != want {
		t.Errorf("bad user: %v, want %v", got, want)
	}
	//pkeySigner, _ := ssh.ParsePrivateKey(pem.EncodeToMemory(&pkeyPEM))
	//if got, want := conf.Auth[0], ssh.PublicKeys(pkeySigner); got != want {
	//if diff := cmp.Diff(conf.Auth[0], ssh.PublicKeys(pkeySigner)); diff != "" {
	//	t.Errorf("diff: %v", diff)
	//}
	{
		var got error
		var want error
		if got, want = conf.HostKeyCallback("host", fakeAddr{}, nil), nil; got != want {
			t.Errorf("HostKeyCallback should return nil, got: %v", got)
		}
	}
}

func TestParseHostsList(t *testing.T) {
	// create temp host file
	hosts := `
1.1.1.1
1.1.1.1:22
1.2.3.4.5
foo.bar.baz
asdf
foo bar baz
`
	tempFile := fmt.Sprintf("%s/test-hosts.list", os.TempDir())
	if err := ioutil.WriteFile(fmt.Sprintf("%s/test-hosts.list", os.TempDir()), []byte(hosts), 0600); err != nil {
		t.Fatalf("ioutil.WriteFile: %v", err)
	}
	defer func() { _ = os.Remove(tempFile) }()
	re, _ := regexp.Compile(`^([^\s]*)\b`)
	{
		got, err := ParseHostsList(tempFile, re, Append22)
		if err != nil {
			t.Errorf("ParseHostsList: %v", err)
		}
		want := []string{"1.1.1.1:22", "1.1.1.1:22", "1.2.3.4.5:22", "foo.bar.baz:22", "asdf:22", "foo:22"}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("diff: %v", diff)
		}
	}
}

func TestAppend22(t *testing.T) {
	if got, want := Append22("foo"), "foo:22"; got != want {
		t.Errorf("got: %v, want %v", got, want)
	}
	if got, want := Append22("foo:22"), "foo:22"; got != want {
		t.Errorf("got: %v, want %v", got, want)
	}
	if got, want := Append22("foo:"), "foo:22"; got != want {
		t.Errorf("got: %v, want %v", got, want)
	}
	if got, want := Append22("http://foo:22"), "http://foo:22"; got != want {
		t.Errorf("got: %v, want %v", got, want)
	}
	if got, want := Append22(""), ""; got != want {
		t.Errorf("got: %v, want %v", got, want)
	}
}

type fakeAddr struct {
	network string
	host    string
}

func (f fakeAddr) Network() string {
	return f.network
}

func (f fakeAddr) String() string {
	return f.host
}
