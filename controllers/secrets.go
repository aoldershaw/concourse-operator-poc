package controllers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	deployv1alpha1 "github.com/aoldershaw/concourse-operator-poc/api/v1alpha1"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const keyBits = 4096

// TODO: split into multiple secrets - worker shouldn't be able to access web secrets
type webWorkerSecrets struct {
	dbEncryptionKey   [32]byte        // web
	sessionSigningKey *rsa.PrivateKey // web
	tsaHostPrivateKey *rsa.PrivateKey // web
	tsaHostPublicKey  *rsa.PublicKey  // worker
	workerPrivateKey  *rsa.PrivateKey // worker
	workerPublicKey   *rsa.PublicKey  // web
}

func getSecretName(concourse deployv1alpha1.Concourse) string {
	return concourse.Name
}

func generateSecret(concourse deployv1alpha1.Concourse) (corev1.Secret, error) {
	var secrets webWorkerSecrets
	_, err := rand.Read(secrets.dbEncryptionKey[:])
	if err != nil {
		return corev1.Secret{}, fmt.Errorf("read.Read: %w", err)
	}
	secrets.sessionSigningKey, err = generatePrivateKey()
	if err != nil {
		return corev1.Secret{}, fmt.Errorf("generate-session-signing-key: %w", err)
	}
	secrets.tsaHostPublicKey, secrets.tsaHostPrivateKey, err = generatePublicPrivateKeyPair()
	if err != nil {
		return corev1.Secret{}, fmt.Errorf("generate-tsa-host-key: %w", err)
	}
	secrets.workerPublicKey, secrets.workerPrivateKey, err = generatePublicPrivateKeyPair()
	if err != nil {
		return corev1.Secret{}, fmt.Errorf("generate-worker-key: %w", err)
	}
	secretName := getSecretName(concourse)
	return corev1.Secret{
		TypeMeta: v1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Secret"},
		ObjectMeta: v1.ObjectMeta{
			Name:      secretName,
			Namespace: concourse.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"db-encryption-key":    secrets.dbEncryptionKey[:],
			"session-signing-key":  privateKeyToBytes(secrets.sessionSigningKey),
			"tsa-host-private-key": privateKeyToBytes(secrets.tsaHostPrivateKey),
			"tsa-host-public-key":  publicKeyToBytes(secrets.tsaHostPublicKey),
			"worker-private-key":   privateKeyToBytes(secrets.workerPrivateKey),
			"worker-public-key":    publicKeyToBytes(secrets.workerPublicKey),
		},
	}, nil
}

func generatePrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, keyBits)
}

func generatePublicPrivateKeyPair() (*rsa.PublicKey, *rsa.PrivateKey, error) {
	privateKey, err := generatePrivateKey()
	if err != nil {
		return nil, nil, err
	}
	return &privateKey.PublicKey, privateKey, nil
}

func privateKeyToBytes(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

func publicKeyToBytes(key *rsa.PublicKey) []byte {
	// TODO: handle error?
	sshPubKey, _ := ssh.NewPublicKey(key)
	return ssh.MarshalAuthorizedKey(sshPubKey)
}
