package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/stolostron/foundation-e2e/performance/apirequestcount"
	"github.com/stolostron/foundation-e2e/performance/cluster"
	"github.com/stolostron/foundation-e2e/performance/placement"
	"github.com/stolostron/foundation-e2e/performance/report"
	"github.com/stolostron/foundation-e2e/performance/utils"

	"github.com/stolostron/foundation-e2e/performance/options"
)

var (
	ops          *options.PerfTestOptions
	rep          *report.Report
	clustersOps  *cluster.ClusterOps
	placementOps *placement.PlacementOps
	apiCountOps  *apirequestcount.ApiCountOps
)

func init() {
	ops = options.NewPerfTestOptions()
	ops.AddFlags()
	rep = report.NewReport()
}

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2E suite")
}

// - KUBECONFIG is the location of the kubeconfig file to use
// - MANAGED_CLUSTER_NAME is the name of managed cluster that is deployed by registration-operator
var _ = ginkgo.BeforeSuite(func() {
	err := ops.Validate()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	err = ops.BuildClients()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	rep.ClusterCount = ops.ClusterCount
	rep.PlacementCount = ops.PlacementCount
	opsRecordFile := path.Join(ops.OutputDir, fmt.Sprintf("ops.csv"))
	err = utils.AppendRecordToFile(opsRecordFile, fmt.Sprintf("ClustersCount: %d, PlacementCount: %d, WorkCount:%d",
		ops.ClusterCount, ops.PlacementCount, ops.WorkCount))

	clustersOps = cluster.CreateClusterOps(ops, rep)
	placementOps = placement.CreatePlacementOps(ops, rep)

	//Cleanup test env
	clustersOps.CleanUp()
	placementOps.CleanPlacements(context.Background())

	//create clusters
	err = clustersOps.CreateClusters(context.TODO())
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	//create placement namespace
	err = utils.CreateClusterNamespace(context.TODO(), ops.HubKubeClient, ops.PlacementNamespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	//create placements
	err = placementOps.BindingClusterSet()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = placementOps.CreatingPlacements(context.TODO(), ops.PlacementCount, nil, ops.ClusterCount)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	//API count
	apiCountOps = apirequestcount.CreateApiCountOps(ops, rep)
})

var _ = ginkgo.AfterSuite(func() {
	//Get the increased apirequest count after all resources created.
	apiCountOps.CheckIncreasedApiCount(context.TODO())

	//write the summary file
	recordFile := path.Join(ops.OutputDir, "summary.json")
	reportJson, err := json.Marshal(rep)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = utils.AppendRecordToFile(recordFile, fmt.Sprintf("%v", string(reportJson)))
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	//Clean up clusters
	clustersOps.CleanUp()

	//clean up placements
	placementOps.CleanPlacements(context.Background())

})
