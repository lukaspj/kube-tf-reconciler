package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	tfreconcilev1alpha1 "github.com/LEGO/kube-tf-reconciler/api/v1alpha1"
	"github.com/LEGO/kube-tf-reconciler/pkg/sources"
	"github.com/LEGO/kube-tf-reconciler/pkg/sources/webhook"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	sourceWatcherInterval     time.Duration
	sourceWatcherNamespace    string
	sourceWatcherInCluster    bool
	sourceWatcherWebhookPort  int
	sourceWatcherGitHubSecret string
)

//nolint:exhaustruct,gochecknoglobals
var sourceWatcherCmd = &cobra.Command{
	Use:   "source-watcher",
	Short: "Watch upstream module sources and request workspace refreshes on revision changes",
	RunE:  runSourceWatcher,
}

func init() {
	rootCmd.AddCommand(sourceWatcherCmd)
	sourceWatcherCmd.Flags().DurationVar(&sourceWatcherInterval, "interval", 5*time.Minute, "Poll interval for upstream revision checks")
	sourceWatcherCmd.Flags().StringVarP(&sourceWatcherNamespace, "namespace", "n", "", "Namespace to watch (empty = all namespaces)")
	sourceWatcherCmd.Flags().BoolVar(&sourceWatcherInCluster, "in-cluster", false, "Run in-cluster using the pod service account instead of kubeconfig")
	sourceWatcherCmd.Flags().IntVar(&sourceWatcherWebhookPort, "webhook-port", 8080, "Port for the webhook HTTP server (0 = disabled)")
	sourceWatcherCmd.Flags().StringVar(&sourceWatcherGitHubSecret, "webhook-github-secret", "", "GitHub webhook secret for signature verification (default: KREC_SOURCE_WEBHOOK_GITHUB_SECRET env)")
}

func runSourceWatcher(cmd *cobra.Command, _ []string) error {
	swScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(swScheme))
	utilruntime.Must(tfreconcilev1alpha1.AddToScheme(swScheme))

	var cfg *rest.Config
	var err error
	if sourceWatcherInCluster {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("failed to load in-cluster config: %w", err)
		}
	} else {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{}).ClientConfig()
		if err != nil {
			return fmt.Errorf("failed to load kubeconfig: %w", err)
		}
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: swScheme})
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	secret := sourceWatcherGitHubSecret
	if secret == "" {
		secret = os.Getenv("KREC_SOURCE_WEBHOOK_GITHUB_SECRET")
	}
	if secret == "" {
		slog.Warn("no github webhook secret configured, unsigned webhook payloads are accepted")
	}

	ctx := cmd.Context()
	watcher := &sources.Watcher{
		Client:    k8sClient,
		Resolver:  sources.NewGitResolver(),
		Namespace: sourceWatcherNamespace,
	}

	if sourceWatcherWebhookPort > 0 {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /api/webhook", sources.WebhookHandler(watcher, webhook.NewGitHub(secret)))
		srv := &http.Server{Addr: fmt.Sprintf(":%d", sourceWatcherWebhookPort), Handler: mux}

		go func() {
			slog.InfoContext(ctx, "webhook server listening", "port", sourceWatcherWebhookPort)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.ErrorContext(ctx, "webhook server failed", "error", err)
			}
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				slog.ErrorContext(ctx, "webhook server shutdown failed", "error", err)
			}
		}()
	}

	ticker := time.NewTicker(sourceWatcherInterval)
	defer ticker.Stop()

	if err := watcher.Poll(ctx); err != nil {
		slog.ErrorContext(ctx, "polling module revisions failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := watcher.Poll(ctx); err != nil {
				slog.ErrorContext(ctx, "polling module revisions failed", "error", err)
			}
		}
	}
}
