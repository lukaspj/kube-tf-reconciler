package main

import (
	"k8s.io/apimachinery/pkg/runtime"
	"log/slog"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	cfg, err := ConfigFromEnvironment()
	if err != nil {
		slog.Error("unable to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("config loaded", "config", cfg)

	slog.SetLogLoggerLevel(slog.LevelInfo)
	ctrl.SetLogger(logr.FromSlogHandler(slog.Default().Handler()))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  runtime.NewScheme(),
		HealthProbeBindAddress:  cfg.ProbeAddr,
		LeaderElectionNamespace: cfg.Namespace,
		LeaderElection:          cfg.EnableLeaderElection,
		LeaderElectionID:        cfg.LeaderElectionID,
	})
	if err != nil {
		slog.Error("unable to start manager", "error", err)
		os.Exit(1)
	}

	reconciler := &operator.KrecReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		NamespaceLabel:         cfg.NamespaceLabel,
		DefaultReconcileConfig: operator.DefaultConfig(),
		TokenManager:           diag.NewTokenManager(metrics.Registry),
	}

	if err = reconciler.SetupWithManager(mgr); err != nil {
		slog.Error("unable to create controller", "error", err)
		os.Exit(1)
	}

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		slog.Error("unable to set up health check", "error", err)
		os.Exit(1)
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		slog.Error("unable to set up ready check", "error", err)
		os.Exit(1)
	}

	slog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("problem running manager", "error", err)
		os.Exit(1)
	}
}
