/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main is the main package of the Cluster API In-Cluster IPAM Provider.
package main

import (
	"flag"
	"os"

	//+kubebuilder:scaffold:imports
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/pkg/version"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/cluster-api/util/flags"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/controllers"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/index"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/webhooks"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ipamv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(v1alpha2.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		enableLeaderElection bool
		probeAddr            string
		watchNamespace       string
		watchFilter          string

		managerOptions = flags.ManagerOptions{}

		webhookPort     int
		webhookCertDir  string
		webhookCertName string
		webhookKeyName  string
	)
	// CAPI operator assumes some flags are present so we need to comply. Without this the operator crashes
	// https://github.com/kubernetes-sigs/cluster-api-operator/pull/871
	flags.AddManagerOptions(pflag.CommandLine, &managerOptions)

	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&watchNamespace, "namespace", "",
		"Namespace that the controller watches to reconcile cluster-api objects. If unspecified, the controller watches for cluster-api objects across all namespaces.")
	flag.StringVar(&watchFilter, "watch-filter", "", "")
	flag.IntVar(&webhookPort, "webhook-port", webhook.DefaultPort, "Webhook Server port.")
	flag.StringVar(&webhookCertDir, "webhook-cert-dir", "", "Webhook cert dir.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "", "Webhook cert name.")
	flag.StringVar(&webhookKeyName, "webhook-key-name", "", "Webhook key name.")

	goFlagSet := flag.CommandLine
	pflag.CommandLine.AddGoFlagSet(goFlagSet)
	pflag.Parse()

	tlsOpts, metricsOpts, err := flags.GetManagerOptions(managerOptions)
	if err != nil {
		setupLog.Error(err, "unable to get manager options")
		os.Exit(1)
	}

	// klog.Background will automatically use the right logger.
	ctrl.SetLogger(klog.Background())

	ctx := ctrl.SetupSignalHandler()

	opts := ctrl.Options{
		Metrics:                *metricsOpts,
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7bb7acb4.ipam.cluster.x-k8s.io",
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port:     webhookPort,
				CertDir:  webhookCertDir,
				CertName: webhookCertName,
				KeyName:  webhookKeyName,
				TLSOpts:  tlsOpts,
			},
		),
	}

	if watchNamespace != "" {
		opts.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				watchNamespace: {},
			},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), opts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = index.SetupIndexes(ctx, mgr); err != nil {
		setupLog.Error(err, "failed to setup indexes")
		os.Exit(1)
	}

	if err = (&ipamutil.ClaimReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		WatchFilterValue: watchFilter,
		Adapter: &controllers.InClusterProviderAdapter{
			Client:           mgr.GetClient(),
			WatchFilterValue: watchFilter,
		},
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IPAddressClaim")
		os.Exit(1)
	}
	if err = (&controllers.InClusterIPPoolReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "InClusterIPPoolReconciler")
		os.Exit(1)
	}
	if err = (&controllers.GlobalInClusterIPPoolReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GlobalInClusterIPPoolReconciler")
		os.Exit(1)
	}

	if webhookPort != 0 {
		if err := (&webhooks.InClusterIPPool{Client: mgr.GetClient()}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "InClusterIPPool")
			os.Exit(1)
		}
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager", "version", version.Get().String())
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
