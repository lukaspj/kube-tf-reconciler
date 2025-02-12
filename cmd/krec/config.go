package main

import (
	"kube-recon/pkg/fang"
	"kube-recon/pkg/operator"
)

func ConfigFromEnvironment() (operator.Config, error) {
	return ConfigFromEnvironmentWithPrefix("KREC")
}

func ConfigFromEnvironmentWithPrefix(envPrefix string) (operator.Config, error) {
	loader := fang.New[operator.Config]().
		WithAutomaticEnv(envPrefix).
		WithConfigFile(fang.ConfigFileOptions{
			Paths: []string{"$HOME", "."},
			Names: []string{"config"},
			Type:  fang.ConfigFileTypeYaml,
		})

	return loader.Load()
}
