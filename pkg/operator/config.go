package operator

type Config struct {
	Port                 string
	ProbeAddr            string
	LogLevel             string
	Namespace            string
	NamespaceLabel       string
	LeaderElectionID     string
	EnableLeaderElection bool
}

func DefaultConfig() Config {
	return Config{}
}
