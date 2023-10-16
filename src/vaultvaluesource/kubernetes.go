package vaultvaluesource

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

const (
	serviceAccountFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	roleEnv            = "VAULT_ROLE"
	defaultRole        = "argocd"
	authPathEnv        = "VAULT_AUTH_PATH"
	defaultAuthPath    = "/auth/kubernetes/login/"
)

const (
	envCheck   = "VAULT_SESSION"
	argoPrefix = "ARGOCD_ENV_"
)

// BwSession returns true of BW_SESSION or ARGOCD_ENV_BW_SESSION are set
// If ARGOCD_ENV_BWSESSION is set the value is copied to BW_SESSION
func VaultSession() bool {
	val, got := os.LookupEnv(argoPrefix + envCheck)
	if !got {
		_, got = os.LookupEnv(envCheck)
		return got
	}
	os.Setenv(envCheck, val)
	return true
}

func getArgoEnv(name string, defaultVal string) string {
	result, got := os.LookupEnv(argoPrefix + name)
	if !got {
		result, got = os.LookupEnv(name)
		if !got {
			return defaultVal
		}
	}
	return result
}

// readJWT reads the JWT data for the Agent to submit to Vault. The default is
// to read the JWT from the default service account location, defined by the
// constant serviceAccountFile. In normal use k.jwtData is nil at invocation and
// the method falls back to reading the token path with os.Open, opening a file
// from either the default location or from the token_path path specified in
// configuration.
func readJWT() (string, error) {
	// load configured token path if set, default to serviceAccountFile
	tokenFilePath := serviceAccountFile

	f, err := os.Open(tokenFilePath)
	if err != nil {
		log.Printf("Kubernetes authentication - no secret found %v", err)
		return "", nil
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(contentBytes)), nil
}

func getVaultRole() string {
	return getArgoEnv(roleEnv, defaultRole)
}

func getVaultAuthPath() string {
	path := getArgoEnv(authPathEnv, defaultAuthPath)
	if path != defaultAuthPath {
		return fmt.Sprintf("/auth/%s/login/", path)
	}
	return path
}

func (m *VaultValueSource) tryKubernetesAuth() error {
	jwt, err := readJWT()
	if err != nil {
		return err
	}
	if jwt == "" {
		return nil
	}
	secret, err := m.Client.Logical().Write(getVaultAuthPath(), map[string]interface{}{
		"role": getVaultRole(),
		"jwt":  jwt,
	})
	if err != nil {
		return err
	}
	m.Client.SetToken(secret.Auth.ClientToken)
	return nil
}
