// Copyright Contributors to the Open Cluster Management project

package cluster

import (
	"context"
	"fmt"
	"path"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/onsi/gomega"
	"github.com/stolostron/foundation-e2e/performance/options"
	"github.com/stolostron/foundation-e2e/performance/report"
	"github.com/stolostron/foundation-e2e/performance/utils"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

var metricAction = "create_cluster"

type ClusterOps struct {
	ops *options.PerfTestOptions
	rep *report.Report
}

func CreateClusterOps(ops *options.PerfTestOptions, rep *report.Report) *ClusterOps {
	return &ClusterOps{
		ops: ops,
		rep: rep,
	}
}

func (o *ClusterOps) CreateClusters(ctx context.Context) error {
	var metricsFile string
	var createClusterMetric report.ActionMetric

	metricsFile = path.Join(o.ops.OutputDir, "create-cluster")

	currentClusters, err := o.getCurrentClusterCount(ctx)
	if err != nil {
		return err
	}

	klog.Infof("current clusters count %d, expected clusters count %d", currentClusters, o.ops.ClusterCount)

	if (o.ops.ClusterCount - currentClusters) <= 0 {
		return nil
	}
	if o.ops.MetricsRecorder != nil {
		createClusterMetric = report.ActionMetric{}

		if err := o.ops.MetricsRecorder.Record(ctx, metricsFile, currentClusters, &createClusterMetric); err != nil {
			return err
		}

		o.rep.AllMetrics[metricAction] = createClusterMetric
	}

	klog.Infof("%d clusters will be created ...", o.ops.ClusterCount-currentClusters)

	for i := currentClusters; i < o.ops.ClusterCount; i++ {
		clusterName := getClusterName(o.ops.ClusterNamePrefix, i)

		klog.Infof("Cluster %q is creating", clusterName)
		startTime := time.Now()
		if err := utils.CreateClusterNamespace(ctx, o.ops.HubKubeClient, clusterName); err != nil {
			return err
		}

		if err := o.createCluster(ctx, clusterName); err != nil {
			return err
		}

		usedTime := time.Since(startTime)
		klog.Infof("Cluster %q is ready, time used %ds",
			clusterName, usedTime/(time.Millisecond*time.Microsecond))

		if o.ops.MetricsRecorder != nil {
			if i != 0 && i%10 == 0 {
				if err := o.ops.MetricsRecorder.Record(ctx, metricsFile, i, &createClusterMetric); err != nil {
					return err
				}
				o.rep.AllMetrics[metricAction] = createClusterMetric
			}
		}

		time.Sleep(utils.CreateInterval)
	}
	if o.ops.MetricsRecorder != nil {
		if err := o.ops.MetricsRecorder.Record(ctx, metricsFile, o.ops.ClusterCount, &createClusterMetric); err != nil {
			return err
		}
	}

	return nil
}

func (o *ClusterOps) getCurrentClusterCount(ctx context.Context) (int, error) {
	clusters, err := o.ops.HubClusterClient.ClusterV1().ManagedClusters().List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", utils.PerformanceTestLabel),
	})
	if err != nil {
		return -1, err
	}

	return len(clusters.Items), nil
}

func (o *ClusterOps) createCluster(ctx context.Context, name string) error {
	_, err := o.ops.HubClusterClient.ClusterV1().ManagedClusters().Create(
		ctx,
		&clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					utils.PerformanceTestLabel: "true",
					"cloud":                    "aws",
				},
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient: true,
			},
		},
		metav1.CreateOptions{},
	)
	return err
}

func getClusterName(prefix string, index int) string {
	return fmt.Sprintf("%s-%d", prefix, index)
}

func isCSRInTerminalState(status *certificatesv1.CertificateSigningRequestStatus) bool {
	for _, c := range status.Conditions {
		if c.Type == certificatesv1.CertificateApproved {
			return true
		}
		if c.Type == certificatesv1.CertificateDenied {
			return true
		}
	}
	return false
}

func (o *ClusterOps) CleanUp() error {
	ctx := context.Background()
	clusters, err := o.ops.HubClusterClient.ClusterV1().ManagedClusters().List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", utils.PerformanceTestLabel),
	})
	if err != nil {
		return err
	}

	for _, cluster := range clusters.Items {
		if err := o.ops.HubClusterClient.ClusterV1().ManagedClusters().Delete(ctx, cluster.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}

		klog.Infof("Cluster %q is deleted", cluster.Name)

		if err := o.ops.HubKubeClient.CoreV1().Namespaces().Delete(ctx, cluster.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}
		klog.Infof("Cluster namespace %q is deleted", cluster.Name)
	}

	//Wait all clusters and ns deleted
	gomega.Eventually(func() bool {
		clusters, err := o.ops.HubClusterClient.ClusterV1().ManagedClusters().List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=true", utils.PerformanceTestLabel),
		})
		if err != nil {
			return false
		}

		ns, err := o.ops.HubKubeClient.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=true", utils.PerformanceTestLabel),
		})
		if err != nil {
			return false
		}

		//All clusters related ns should be deleted
		if len(clusters.Items) == 0 && len(ns.Items) <= 1 {
			return true
		}
		klog.Infof("wait all clusters and ns deleted. clusters :%v, ns:%v", len(clusters.Items), len(ns.Items))
		return false
	}, 300*time.Second, 1*time.Second).Should(gomega.BeTrue())

	return nil
}
