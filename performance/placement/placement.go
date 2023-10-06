package placement

import (
	"context"
	"fmt"
	"path"
	"time"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"

	"k8s.io/apimachinery/pkg/api/errors"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"

	"github.com/onsi/gomega"
	"github.com/stolostron/foundation-e2e/performance/options"
	"github.com/stolostron/foundation-e2e/performance/report"
	"github.com/stolostron/foundation-e2e/performance/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterapiv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	clusterapiv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterapiv1beta2 "open-cluster-management.io/api/cluster/v1beta2"

	placementUtil "open-cluster-management.io/placement/test/integration/util"
)

const (
	statusTimeout  = 300
	statusInterval = 1

	clusterSetName     = "global"
	createMetricAction = "create_placement"
)

type PlacementOps struct {
	ops                 *options.PerfTestOptions
	placementRecordFile string
	rep                 *report.Report
}

var PlacementTemplate = clusterapiv1beta1.Placement{
	Spec: clusterapiv1beta1.PlacementSpec{
		Tolerations: []clusterapiv1beta1.Toleration{
			{
				Key:      "cluster.open-cluster-management.io/unreachable",
				Operator: clusterapiv1beta1.TolerationOpEqual,
			},
		},
		Predicates: []clusterapiv1beta1.ClusterPredicate{
			{
				RequiredClusterSelector: clusterapiv1beta1.ClusterSelector{
					LabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"cloud": "aws",
						},
					},
				},
			},
		},
	},
}

var PlacementScoreTemplate = clusterapiv1alpha1.AddOnPlacementScore{
	ObjectMeta: metav1.ObjectMeta{
		Name: "default",
	},
	Status: clusterapiv1alpha1.AddOnPlacementScoreStatus{
		Conditions: []metav1.Condition{
			{
				Type:               "AddOnPlacementScoreUpdated",
				Status:             "True",
				Message:            "AddOnPlacementScore updated successfully",
				Reason:             "AddOnPlacementScoreUpdated",
				LastTransitionTime: metav1.Now(),
			},
		},
		Scores: []clusterapiv1alpha1.AddOnPlacementScoreItem{
			{
				Name:  "cpuAvailable",
				Value: 66,
			},
			{
				Name:  "memAvailable",
				Value: 55,
			},
		},
	},
}

func CreatePlacementOps(ops *options.PerfTestOptions, rep *report.Report) *PlacementOps {
	return &PlacementOps{
		ops:                 ops,
		rep:                 rep,
		placementRecordFile: path.Join(ops.OutputDir, fmt.Sprintf("placement")),
	}
}

func (p *PlacementOps) CreatingPlacements(ctx context.Context, num int, noc *int32, nod int) error {
	klog.Infof("Create %d placements", num)
	var createPlacementMetric report.ActionMetric

	createPlacementMetricFiles := p.placementRecordFile + "-create"
	if p.ops.MetricsRecorder != nil {
		if err := p.ops.MetricsRecorder.Record(ctx, createPlacementMetricFiles, 0, &createPlacementMetric); err != nil {
			return err
		}
		p.rep.AllMetrics[createMetricAction] = createPlacementMetric
	}
	for i := 0; i < num; i++ {
		placementName := fmt.Sprintf("%s-%d", p.ops.PlacementNamePrefix, i)
		err := p.CreatePlacement(ctx, placementName, i, noc, nod)
		if err != nil {
			return err
		}

		if p.ops.MetricsRecorder != nil {
			if i != 0 && i%10 == 0 {
				if err := p.ops.MetricsRecorder.Record(ctx, createPlacementMetricFiles, i, &createPlacementMetric); err != nil {
					return err
				}
			}
		}
		time.Sleep(utils.CreateInterval) //sleep 1 second in case API server is too busy
	}
	klog.Infof("Finished %d placements", num)
	return nil
}

func (p *PlacementOps) CreatingPlacementsScores(ctx context.Context) error {
	klog.Infof("Create placements scores")

	clusters, err := p.ops.HubClusterClient.ClusterV1().ManagedClusters().List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", utils.PerformanceTestLabel),
	})
	if err != nil {
		return err
	}

	for _, cluster := range clusters.Items {
		klog.Infof("Create placements scores in %v", cluster.Name)
		pc, err := p.ops.HubClusterClient.ClusterV1alpha1().AddOnPlacementScores(cluster.Name).Create(ctx, &PlacementScoreTemplate, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		pc.Status = PlacementScoreTemplate.Status
		_, err = p.ops.HubClusterClient.ClusterV1alpha1().AddOnPlacementScores(cluster.Name).UpdateStatus(ctx, pc, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	klog.Infof("Finished placements score")
	return nil
}

func (p *PlacementOps) CleanPlacements(ctx context.Context) error {
	pll, err := p.ops.HubClusterClient.ClusterV1beta1().Placements(p.ops.PlacementNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pl := range pll.Items {
		klog.Infof("delete placement:%v", pl.Name)
		err = p.ops.HubClusterClient.ClusterV1beta1().Placements(p.ops.PlacementNamespace).Delete(context.Background(), pl.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	klog.Infof("delete placement namespace :%v", p.ops.PlacementNamespace)

	err = utils.DeleteClusterNamespace(context.TODO(), p.ops.HubKubeClient, p.ops.PlacementNamespace)

	return err
}

func (p *PlacementOps) CheckAllPlacementUpdated(ctx context.Context, fileSuffix string, nod int) error {
	var pll *clusterapiv1beta1.PlacementList
	var err error
	var finishedPlacements = sets.NewString()
	startTime := time.Now()
	for {
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			pll, err = p.ops.HubClusterClient.ClusterV1beta1().Placements(p.ops.PlacementNamespace).List(context.Background(), metav1.ListOptions{})
			if err != nil {
				klog.Errorf("Failed to list placement:%v", err)
				return err
			}
			return nil
		})
		if err != nil {
			klog.Errorf("Failed to list placement:%v", err)
			return err
		}

		if finishedPlacements.Len() >= len(pll.Items) {
			klog.Infof("All placement updated")
			return nil
		}

		for index, pl := range pll.Items {
			if finishedPlacements.Has(pl.Name) {
				continue
			}

			if !isPlacementStatusMatched(&pl, nod) {
				continue
			}
			if !isNumberOfDecisionsMatched(&pl, p.ops.HubClusterClient, nod) {
				continue
			}

			finishedPlacements.Insert(pl.Name)

			usedTime := time.Since(startTime).Seconds()
			if len(fileSuffix) != 0 {
				filePath := p.placementRecordFile + "_" + fileSuffix + ".csv"
				if err := utils.AppendRecordToFile(filePath, fmt.Sprintf("%s,%d,%d",
					pl.Name, index, int(usedTime))); err != nil {
					return err
				}
				p.getSheduleTime(int(usedTime), fileSuffix)
			}

		}
		time.Sleep(1 * time.Second)
	}
}

func (p *PlacementOps) CreatePlacement(ctx context.Context, name string, index int, noc *int32, nod int) error {
	placement := PlacementTemplate.DeepCopy()
	placement.Name = name
	placement.Spec.NumberOfClusters = noc
	var pl *clusterapiv1beta1.Placement
	retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		_, err := p.ops.HubClusterClient.ClusterV1beta1().Placements(p.ops.PlacementNamespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				klog.Infof("Create placement: %v", name)
				pl, err = p.ops.HubClusterClient.ClusterV1beta1().Placements(p.ops.PlacementNamespace).Create(context.Background(), placement, metav1.CreateOptions{})
				return err
			}
		}
		return nil
	})

	startTime := time.Now()

	err := p.assertNumberOfDecisions(pl, index, nod)
	if err != nil {
		return err
	}

	err = p.assertPlacementStatus(pl, index, nod)
	if err != nil {
		return err
	}

	usedTime := time.Since(startTime).Seconds()
	filePath := p.placementRecordFile + "_" + "create.csv"
	if err := utils.AppendRecordToFile(filePath, fmt.Sprintf("%d,%d",
		index, int(usedTime))); err != nil {
		return err
	}
	p.getSheduleTime(int(usedTime), createMetricAction)

	return nil
}

func (p *PlacementOps) getSheduleTime(usedTime int, key string) {
	var maxTime = usedTime
	var minTime = usedTime
	if _, ok := p.rep.PlacementSchedule[key]; ok {
		maxTime = p.rep.PlacementSchedule[key].MaxTime
		minTime = p.rep.PlacementSchedule[key].MinTime

		if maxTime < int(usedTime) {
			maxTime = int(usedTime)
		}
		if minTime > int(usedTime) {
			minTime = int(usedTime)
		}
	}
	p.rep.PlacementSchedule[key] = report.ScheduelTime{
		MaxTime: maxTime,
		MinTime: minTime,
	}
	return
}

func (p *PlacementOps) assertNumberOfDecisions(placement *clusterapiv1beta1.Placement, index int, desiredNOD int) error {
	klog.Infof("Get Placement %v decisions.", placement.Name)
	gomega.Eventually(func() bool {
		return isNumberOfDecisionsMatched(placement, p.ops.HubClusterClient, desiredNOD)
	}, statusTimeout, statusInterval).Should(gomega.BeTrue())
	return nil
}

func isNumberOfDecisionsMatched(placement *clusterapiv1beta1.Placement, hubClusterClient clusterclient.Interface, desiredNOD int) bool {
	pdl, err := hubClusterClient.ClusterV1beta1().PlacementDecisions(placement.Namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("error to list placement decision:%v", err)
		return false
	}
	if len(pdl.Items) == 0 {
		klog.Errorf("No placement decision found")
		return false
	}

	actualNOD := 0
	for _, pd := range pdl.Items {
		if controlled := metav1.IsControlledBy(&pd.ObjectMeta, placement); !controlled {
			continue
		}
		klog.Infof("Get placement decision: %v, len(pd.Status.Decisions): %v. actualNOD: %v", pd.Name, len(pd.Status.Decisions), actualNOD)
		actualNOD += len(pd.Status.Decisions)
	}

	klog.Infof("placement: %v, actualNOD:%v, desiredNOD:%v", placement.Name, actualNOD, desiredNOD)
	return actualNOD == desiredNOD
}

func (p *PlacementOps) assertPlacementStatus(placement *clusterapiv1beta1.Placement, index, numOfSelectedClusters int) error {
	gomega.Eventually(func() bool {
		klog.Infof("Get Placement %v Status.", placement.Name)
		placement, err := p.ops.HubClusterClient.ClusterV1beta1().Placements(p.ops.PlacementNamespace).Get(context.Background(), placement.Name, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Failed to list placements, err:%v", err)
			return false
		}
		return isPlacementStatusMatched(placement, numOfSelectedClusters)
	}, statusTimeout, statusInterval).Should(gomega.BeTrue())

	return nil
}

func isPlacementStatusMatched(placement *clusterapiv1beta1.Placement, numOfSelectedClusters int) bool {
	klog.Infof("Get Placement %v Status.", placement.Name)
	if !placementUtil.HasCondition(
		placement.Status.Conditions,
		clusterapiv1beta1.PlacementConditionSatisfied,
		"",
		metav1.ConditionTrue,
	) {
		klog.Errorf("Failed to get satisfied conditions, Condition: %v", placement.Status.Conditions)
		return false
	}

	klog.Infof("placement: %v, placement.Status.NumberOfSelectedClusters:%v, numOfSelectedClusters:%v", placement.Name, placement.Status.NumberOfSelectedClusters, int32(numOfSelectedClusters))

	return placement.Status.NumberOfSelectedClusters == int32(numOfSelectedClusters)
}

func (p *PlacementOps) UpdatingPlacements(placement *clusterapiv1beta1.Placement, fileSuffix string, nod int, waitStatus bool) error {
	var pls *clusterapiv1beta1.PlacementList
	var err error
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		klog.Infof("List placement")
		pls, err = p.ops.HubClusterClient.ClusterV1beta1().Placements(p.ops.PlacementNamespace).List(context.Background(), metav1.ListOptions{})
		klog.Errorf("Failed to List placement:%v", err)
		return err
	})
	if err != nil {
		klog.Errorf("Failed to List placement:%v", err)
		return err
	}
	// updatePlacementMetricFiles := p.placementRecordFile + "-update-" + fileSuffix
	// if p.ops.MetricsRecorder != nil {
	// 	if err := p.ops.MetricsRecorder.Record(context.Background(), updatePlacementMetricFiles, 0, p.rep.AllMetrics[metricAction]); err != nil {
	// 		return err
	// 	}
	// }
	for i, pl := range pls.Items {
		pl.Spec = placement.Spec
		err = p.UpdatingPlacement(&pl, i, fileSuffix, nod, waitStatus)
		if err != nil {
			klog.Errorf("Failed to UpdatingPlacement placement:%v", err)
			return err
		}
		// if p.ops.MetricsRecorder != nil {
		// 	if i != 0 && i%10 == 0 {
		// 		if err := p.ops.MetricsRecorder.Record(context.Background(), updatePlacementMetricFiles, i, p.rep.AllMetrics[metricAction]); err != nil {
		// 			return err
		// 		}
		// 	}
		// }
	}
	if !waitStatus {
		err = p.CheckAllPlacementUpdated(context.Background(), fileSuffix, nod)
		return err
	}

	return err
}

func (p *PlacementOps) UpdatingPlacement(placement *clusterapiv1beta1.Placement, index int, fileSuffix string, nod int, waitStatus bool) error {
	var pl *clusterapiv1beta1.Placement
	var err error
	curPlacement := placement.DeepCopy()
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		klog.Infof("Update placement: %v. fileSuffix:%v", placement.Name, fileSuffix)
		curPlacement.Spec = placement.Spec
		pl, err = p.ops.HubClusterClient.ClusterV1beta1().Placements(p.ops.PlacementNamespace).Update(context.Background(), curPlacement, metav1.UpdateOptions{})
		if err != nil {
			var createErr error
			klog.Errorf("Failed to update placement:%v", err)
			curPlacement, createErr = p.ops.HubClusterClient.ClusterV1beta1().Placements(p.ops.PlacementNamespace).Get(context.Background(), placement.Name, metav1.GetOptions{})
			if createErr != nil {
				return createErr
			}
		}
		return err
	})
	if err != nil {
		klog.Errorf("Failed to update placement:%v", err)
		return err
	}

	if waitStatus {
		startTime := time.Now()
		klog.Infof("Wait placement %s update", pl.Name)
		err = p.assertNumberOfDecisions(pl, index, nod)
		if err != nil {
			klog.Errorf("Failed to assertNumberOfDecisions placement:%v", err)
			return err
		}

		//if placement.Spec.NumberOfClusters != nil {
		err = p.assertPlacementStatus(pl, index, nod)
		//}
		if err != nil {
			klog.Errorf("Failed to assertPlacementStatus placement:%v", err)
			return err
		}

		usedTime := time.Since(startTime).Seconds()
		klog.Infof("placement %s updated. use: %f", pl.Name, usedTime)

		if len(fileSuffix) != 0 {
			filePath := p.placementRecordFile + "_" + fileSuffix + ".csv"
			if err := utils.AppendRecordToFile(filePath, fmt.Sprintf("%s,%d,%d",
				pl.Name, index, int(usedTime))); err != nil {
				return err
			}
			p.getSheduleTime(int(usedTime), fileSuffix)

		}
	}

	return nil
}

func (p *PlacementOps) BindingClusterSet() error {
	klog.Infof("Create clusterset/clustersetbinding")
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		_, err := p.ops.HubClusterClient.ClusterV1beta2().ManagedClusterSets().Get(context.Background(), clusterSetName, metav1.GetOptions{})
		if err != nil {
			if apiErrors.IsNotFound(err) {
				clusterset := &clusterapiv1beta2.ManagedClusterSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: clusterSetName,
					},
				}
				klog.Infof("Create clusterset: %v", clusterset.Name)
				_, err := p.ops.HubClusterClient.ClusterV1beta2().ManagedClusterSets().Create(context.Background(), clusterset, metav1.CreateOptions{})
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		_, err = p.ops.HubClusterClient.ClusterV1beta2().ManagedClusterSetBindings(p.ops.PlacementNamespace).Get(context.Background(), clusterSetName, metav1.GetOptions{})
		if err != nil {
			if apiErrors.IsNotFound(err) {
				csb := &clusterapiv1beta2.ManagedClusterSetBinding{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: p.ops.PlacementNamespace,
						Name:      clusterSetName,
					},
					Spec: clusterapiv1beta2.ManagedClusterSetBindingSpec{
						ClusterSet: clusterSetName,
					},
				}
				klog.Infof("Create clustersetbinding: %v", csb.Name)
				_, err = p.ops.HubClusterClient.ClusterV1beta2().ManagedClusterSetBindings(p.ops.PlacementNamespace).Create(context.Background(), csb, metav1.CreateOptions{})
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
