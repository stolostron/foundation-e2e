// Copyright Contributors to the Open Cluster Management project
package metrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/stolostron/foundation-e2e/performance/report"
	"github.com/stolostron/foundation-e2e/performance/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsv1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

var supportedMetricsAPIVersions = []string{
	"v1beta1",
}
var metricPods = []string{
	"open-cluster-management-hub/cluster-manager-placement-controller",
	"open-cluster-management-hub/cluster-manager-registration-controller",
	"openshift-kube-apiserver/kube-apiserver-ip",
	"kube-system/kube-apiserver",
}

var skipContainer = sets.NewString(
	"kube-apiserver-cert-syncer",
	"kube-apiserver-check-endpoints",
	"kube-apiserver-insecure-readyz",
	"kube-apiserver-cert-regeneration-controller",
)

type MetricsRecorder struct {
	metricsClient metricsclientset.Interface
}

func BuildMetricsGetter(kubeConfig, namespace string) (*MetricsRecorder, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig with %s, %v", kubeConfig, err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to build discovery client with %s, %v", kubeConfig, err)
	}

	apiGroups, err := discoveryClient.ServerGroups()
	if err != nil {
		return nil, err
	}

	if !metricsAPIAvailable(apiGroups) {
		return nil, fmt.Errorf("metrics API not available on the %s", kubeConfig)
	}

	metricsClient, err := metricsclientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &MetricsRecorder{
		metricsClient: metricsClient,
	}, nil
}

func (g *MetricsRecorder) Record(ctx context.Context, filenamePrefix string, clusterCounts int, am *report.ActionMetric) error {
	for _, pod := range metricPods {
		namespace, name, err := cache.SplitMetaNamespaceKey(pod)
		if err != nil {
			return err
		}
		err = g.RecordPod(ctx, namespace, name, filenamePrefix, clusterCounts, am)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *MetricsRecorder) RecordPod(ctx context.Context, namespace, namePrefix, filenamePrefix string, clusterCounts int, am *report.ActionMetric) error {
	versionedMetrics, err := g.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list metrics in the namespace %s , %v", namespace, err)
	}
	metrics := &metricsapi.PodMetricsList{}
	err = metricsv1beta1api.Convert_v1beta1_PodMetricsList_To_metrics_PodMetricsList(versionedMetrics, metrics, nil)
	if err != nil {
		return fmt.Errorf("failed to convert metrics, %v", err)
	}

	for _, m := range metrics.Items {
		if !strings.HasPrefix(m.Name, namePrefix) {
			continue
		}
		for _, c := range m.Containers {
			if c.Name == "POD" {
				continue
			}
			if skipContainer.Has(c.Name) {
				continue
			}

			memory, ok := c.Usage.Memory().AsInt64()
			if !ok {
				utils.PrintMsg(fmt.Sprintf("container=%s, cpu=%s, memory=unknown", c.Name, c.Usage.Cpu()))
				continue
			}

			// millicore
			cpu := c.Usage.Cpu()
			var cpuInt int
			if strings.Contains(cpu.String(), "m") {
				cpuString := strings.ReplaceAll(cpu.String(), "m", "")
				cpuInt, err = strconv.Atoi(cpuString)
				if err != nil {
					klog.Errorf("Failed to convert cpu:%v", err)
					return err
				}
			}
			if strings.Contains(cpu.String(), "n") {
				cpuString := strings.ReplaceAll(cpu.String(), "n", "")
				cpuInt, err = strconv.Atoi(cpuString)
				if err != nil {
					klog.Errorf("Failed to convert cpu:%v", err)
					return err
				}
				cpuInt = cpuInt / 1000 / 1000
			}

			// megabytes
			memory = memory / 1024 / 1024
			utils.PrintMsg(fmt.Sprintf("container=%s, counts=%d, cpu=%d, memory=%dMi",
				c.Name, clusterCounts, cpuInt, memory))
			fullFileName := filenamePrefix + "-" + m.Name + "-" + c.Name + ".csv"
			if err := utils.AppendRecordToFile(fullFileName, fmt.Sprintf("%d,%d,%d",
				clusterCounts, cpuInt, memory)); err != nil {
				return fmt.Errorf("failed to dump metrics to file, %v", err)
			}

			var maxCpu = cpuInt
			var minCpu = cpuInt
			var maxMemory = memory
			var minMemory = memory
			if _, ok := am.PodMetric[m.Name]; ok {
				maxCpu = am.PodMetric[m.Name].MaxCpu
				minCpu = am.PodMetric[m.Name].MinCpu
				maxMemory = int64(am.PodMetric[m.Name].MaxMemory)
				minMemory = int64(am.PodMetric[m.Name].MinMemory)
				if maxCpu < cpuInt {
					maxCpu = cpuInt
				}
				if minCpu > cpuInt {
					minCpu = cpuInt
				}
				if maxMemory < memory {
					maxMemory = memory
				}
				if minMemory > memory {
					minMemory = memory
				}
			}
			if len(am.PodMetric) == 0 {
				am.PodMetric = make(map[string]report.Metric)
			}
			am.PodMetric[m.Name] = report.Metric{
				FileName:  fullFileName,
				MaxCpu:    maxCpu,
				MinCpu:    minCpu,
				MaxMemory: int(maxMemory),
				MinMemory: int(minMemory),
			}
		}
	}

	return nil
}

func metricsAPIAvailable(discoveredAPIGroups *metav1.APIGroupList) bool {
	for _, discoveredAPIGroup := range discoveredAPIGroups.Groups {
		if discoveredAPIGroup.Name != metricsapi.GroupName {
			continue
		}
		for _, version := range discoveredAPIGroup.Versions {
			for _, supportedVersion := range supportedMetricsAPIVersions {
				if version.Version == supportedVersion {
					return true
				}
			}
		}
	}
	return false
}
