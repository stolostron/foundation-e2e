package options

import (
	"flag"
	"fmt"

	"github.com/stolostron/foundation-e2e/performance/utils"

	"github.com/stolostron/foundation-e2e/performance/metrics"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

const (
	performanceTestLabel       = "perftest.open-cluster-management.io"
	defaultNamespace           = "open-cluster-management-hub"
	workCreationTimeRecordFile = "works-creation-time"
)

type PerfTestOptions struct {
	Namespace           string
	ClusterNamePrefix   string
	PlacementNamePrefix string
	PlacementNamespace  string
	HubKubeconfig       string

	OutputDir string

	ClusterCount   int
	WorkCount      int
	PlacementCount int

	HubKubeClient    kubernetes.Interface
	DynamicHubClient dynamic.Interface
	HubClusterClient clusterclient.Interface
	HubWorkClient    workclient.Interface
	MetricsRecorder  *metrics.MetricsRecorder
}

func NewPerfTestOptions() *PerfTestOptions {
	return &PerfTestOptions{
		ClusterNamePrefix:   "test",
		PlacementNamePrefix: "test-placement",
		PlacementNamespace:  "placement-ns",
		Namespace:           defaultNamespace,
		ClusterCount:        3,
		WorkCount:           5,
		PlacementCount:      3,

		OutputDir: "output/",
	}
}

func (o *PerfTestOptions) AddFlags() {
	flag.StringVar(&o.HubKubeconfig, "hub-kubeconfig", o.HubKubeconfig, "The kubeconfig of multicluster controlplane")
	flag.StringVar(&o.Namespace, "hub-namespace", o.Namespace, "The namespace of multicluster controlplane")
	flag.StringVar(&o.ClusterNamePrefix, "cluster-name-prefix", o.ClusterNamePrefix, "The name prefix of clusters")
	flag.StringVar(&o.PlacementNamePrefix, "placement-name-prefix", o.PlacementNamePrefix, "The name prefix of placements")
	flag.StringVar(&o.PlacementNamespace, "placement-namespace", o.PlacementNamespace, "The test for placement namespace")

	flag.StringVar(&o.OutputDir, "output-dir", o.OutputDir, "The directory of performance test output files")

	flag.IntVar(&o.ClusterCount, "cluster-count", o.ClusterCount, "The count of clusters")
	flag.IntVar(&o.WorkCount, "work-count", o.WorkCount, "The count of works in one cluster")
	flag.IntVar(&o.PlacementCount, "placement-count", o.PlacementCount, "The count of placement")
}

func (o *PerfTestOptions) BuildClients() error {
	var err error
	kubeConfigFile, err := utils.GetKubeConfigFile()
	if err != nil {
		return err
	}

	hubConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigFile)
	if err != nil {
		return err
	}

	o.HubKubeClient, err = kubernetes.NewForConfig(hubConfig)
	if err != nil {
		return err
	}

	o.DynamicHubClient, err = dynamic.NewForConfig(hubConfig)
	if err != nil {
		return err
	}

	o.HubClusterClient, err = clusterclient.NewForConfig(hubConfig)
	if err != nil {
		return fmt.Errorf("failed to build hub cluster client with %s, %v", o.HubKubeconfig, err)
	}

	o.HubWorkClient, err = workclient.NewForConfig(hubConfig)
	if err != nil {
		return fmt.Errorf("failed to build hub work client with %s, %v", o.HubKubeconfig, err)
	}

	o.MetricsRecorder, err = metrics.BuildMetricsGetter(kubeConfigFile, o.Namespace)
	if err != nil {
		klog.Errorf("failed to build metrics getter with %s, %v", kubeConfigFile, err)
	}

	return nil
}

func (o *PerfTestOptions) Validate() error {
	if o.ClusterCount <= 0 {
		return fmt.Errorf("flag `--cluster-count` must be greater than 0")
	}

	if o.ClusterNamePrefix == "" {
		return fmt.Errorf("flag `--cluster-name-prefix` is required")
	}

	return nil
}
