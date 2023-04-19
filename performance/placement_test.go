package performance

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/stolostron/foundation-e2e/performance/placement"
	"github.com/stolostron/foundation-e2e/performance/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/api/cluster/v1beta1"
)

var _ = ginkgo.Describe("Update all placement label selector, check all placement done time", func() {
	ginkgo.It("Update all placement label selector, check all placement done time", func() {
		updatePlacement := placement.PlacementTemplate.DeepCopy()
		updatePlacement.Spec.NumberOfClusters = utils.Noc(ops.ClusterCount - 1)
		updatePlacement.Spec.Predicates = []v1beta1.ClusterPredicate{}
		err := placementOps.UpdatingPlacements(updatePlacement, "update_label", ops.ClusterCount-1, true)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		oriPlacement := placement.PlacementTemplate.DeepCopy()
		err = placementOps.UpdatingPlacements(oriPlacement, "", ops.ClusterCount, false)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})
})

var _ = ginkgo.Describe("Add one clusters, check all placement done time", func() {
	ginkgo.It("Add one clusters, check all placement done time", func() {
		clusterName := "placement-c1"
		_, err := ops.HubClusterClient.ClusterV1().ManagedClusters().Create(
			context.Background(),
			&clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
					Labels: map[string]string{
						utils.PerformanceTestLabel: "true",
						"cloud":                    "aws",
						"placement":                "test",
					},
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
			},
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		err = placementOps.CheckAllPlacementUpdated(context.Background(), "add_cluster", ops.ClusterCount+1)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		err = ops.HubClusterClient.ClusterV1().ManagedClusters().Delete(context.Background(), clusterName, metav1.DeleteOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		utils.DeleteClusterNamespace(context.Background(), ops.HubKubeClient, clusterName)

		err = placementOps.CheckAllPlacementUpdated(context.Background(), "", ops.ClusterCount)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})
})

var _ = ginkgo.Describe("Enable the resource plugin, check all placement done time", func() {
	ginkgo.It("Enable the resource plugin, check all placement done time", func() {
		targetClustersCount := 2
		err := placementOps.CreatingPlacementsScores(context.Background())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		scorePlacement := placement.PlacementTemplate.DeepCopy()
		scorePlacement.Spec.NumberOfClusters = utils.Noc(targetClustersCount)
		scorePlacement.Spec.PrioritizerPolicy = v1beta1.PrioritizerPolicy{
			Mode: v1beta1.PrioritizerPolicyModeExact,
			Configurations: []v1beta1.PrioritizerConfig{
				{
					ScoreCoordinate: &v1beta1.ScoreCoordinate{
						Type: v1beta1.ScoreCoordinateTypeAddOn,
						AddOn: &v1beta1.AddOnScore{
							ResourceName: placement.PlacementScoreTemplate.Name,
							ScoreName:    "cpuAvailable",
						},
					},
					Weight: 2,
				},
			},
		}
		err = placementOps.UpdatingPlacements(scorePlacement, "update_score", targetClustersCount, true)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		oriPlacement := placement.PlacementTemplate.DeepCopy()
		err = placementOps.UpdatingPlacements(oriPlacement, "", ops.ClusterCount, false)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

	})
})
