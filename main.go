package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/crumbhole/argocd-vault-replacer/src/bwvaluesource"
	"github.com/crumbhole/argocd-vault-replacer/src/k8svaluesource"
	"github.com/crumbhole/argocd-vault-replacer/src/substitution"
	"github.com/crumbhole/argocd-vault-replacer/src/vaultvaluesource"
)

type scanner struct {
	source substitution.ValueSource
}

func (s *scanner) process(input []byte) error {
	subst := substitution.Substitutor{Source: s.source}
	modifiedcontents, err := subst.Substitute(input)
	if err != nil {
		return err
	}
	fmt.Printf("---\n%s\n", modifiedcontents)
	return nil
}

func (s *scanner) scanFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	fileRegexp := regexp.MustCompile(`\.ya?ml$`)
	if fileRegexp.MatchString(path) {
		filecontents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		err = s.process(filecontents)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *scanner) scanDir(path string) error {
	return filepath.Walk(path, s.scanFile)
}

func selectValueSource() substitution.ValueSource {
	// This would be better with a factory pattern
	if bwvaluesource.BwSession() {
		return bwvaluesource.BitwardenValueSource{}
	}
	if vaultvaluesource.VaultSession() {
		return vaultvaluesource.VaultValueSource{}
	}
	return k8svaluesource.KubernetesValueSource{}
}

func copyEnv() {
	for _, envEntry := range []string{`VAULT_ADDR`, `VAULT_TOKEN`} {
		val, got := os.LookupEnv(`ARGOCD_ENV_` + envEntry)
		if got {
			os.Setenv(envEntry, val)
		}
	}
}

func main() {
	copyEnv()
	stat, _ := os.Stdin.Stat()
	s := scanner{source: selectValueSource()}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		reader := bufio.NewReader(os.Stdin)
		filecontents, err := io.ReadAll(reader)
		if err != nil {
			log.Fatal(err)
		}
		err = s.process(filecontents)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		dir, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		err = s.scanDir(dir)
		if err != nil {
			log.Fatal(err)
		}
	}
}
