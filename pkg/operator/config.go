package operator

type Config struct {
	Port                 string
	ProbeAddr            string
	LogLevel             string
	NamespaceLabel       string
	EnableLeaderElection bool
}

func DefaultConfig() *Config {
	return &Config{}
}
