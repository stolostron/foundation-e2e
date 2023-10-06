package apirequestcount

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	apiserverv1 "github.com/openshift/api/apiserver/v1"
	"github.com/stolostron/foundation-e2e/performance/options"
	"github.com/stolostron/foundation-e2e/performance/report"
	"github.com/stolostron/foundation-e2e/performance/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	//record the api count every "eventuallyInterval" seconds
	//record max "totalTime" second api count
	interval  = 60
	totalTime = 600
)

var apirequestcountsGvr = schema.GroupVersionResource{Group: "apiserver.openshift.io", Version: "v1", Resource: "apirequestcounts"}

var allServiveAccounts = sets.NewString(
	"system:serviceaccount:multicluster-engine:cluster-proxy",
	"system:serviceaccount:open-cluster-management-agent:klusterlet-work-sa",
	"system:serviceaccount:open-cluster-management-hub:cluster-manager-registration-controller-sa",
	"system:serviceaccount:open-cluster-management-agent:klusterlet-work-sa",
	"system:serviceaccount:open-cluster-management-agent:klusterlet",
	"system:serviceaccount:multicluster-engine:ocm-foundation-sa",
	"system:serviceaccount:open-cluster-management-hub:cluster-manager-placement-controller-sa",
)

var apiCountMap = sets.NewString(
	"clusterroles.v1.rbac.authorization.k8s.io",
	"clusterrolebindings.v1.rbac.authorization.k8s.io",
	"rolebindings.v1.rbac.authorization.k8s.io",
	"roles.v1.rbac.authorization.k8s.io",

	"managedclusteraddons.v1alpha1.addon.open-cluster-management.io",
	"managedclusters.v1.cluster.open-cluster-management.io",
	"managedclustersetbindings.v1beta2.cluster.open-cluster-management.io",
	"managedclustersets.v1beta2.cluster.open-cluster-management.io",

	"addonplacementscores.v1alpha1.cluster.open-cluster-management.io",
	"placementdecisions.v1beta1.cluster.open-cluster-management.io",
	"placements.v1beta1.cluster.open-cluster-management.io",

	"manifestworks.v1.work.open-cluster-management.io",
	"manifestworkreplicasets.v1alpha1.work.open-cluster-management.io",
)

type ApiCountOps struct {
	ops                      *options.PerfTestOptions
	rep                      *report.Report
	apiCountRecordFile       string
	apiCountDetailRecordFile string
}

func CreateApiCountOps(ops *options.PerfTestOptions, rep *report.Report) *ApiCountOps {
	return &ApiCountOps{
		ops:                      ops,
		rep:                      rep,
		apiCountRecordFile:       path.Join(ops.OutputDir, fmt.Sprintf("apiCount.csv")),
		apiCountDetailRecordFile: path.Join(ops.OutputDir, fmt.Sprintf("apiDetailCount.csv")),
	}
}

func (p *ApiCountOps) CheckApiCount(ctx context.Context, usedTime float64, initCount map[string]int) error {
	for apiCountName, _ := range apiCountMap {
		var curCount int
		var addCount int
		var refreshed bool

		curResourceApiRequest, err := p.ops.DynamicHubClient.Resource(apirequestcountsGvr).Get(ctx, apiCountName, metav1.GetOptions{})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		apicountJson, err := curResourceApiRequest.MarshalJSON()
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		var apirequestCount apiserverv1.APIRequestCount
		err = json.Unmarshal(apicountJson, &apirequestCount)
		if err != nil {
			return err
		}
		for _, nodeApiCount := range apirequestCount.Status.CurrentHour.ByNode {
			for _, userApiCount := range nodeApiCount.ByUser {
				if allServiveAccounts.Has(userApiCount.UserName) {
					curCount += int(userApiCount.RequestCount)
					klog.Infof("API Count Resource: %v, userApiCount.UserName: %v. userApiCount.RequestCount: %v", apiCountName, userApiCount.UserName, userApiCount.RequestCount)
					if err := utils.AppendRecordToFile(p.apiCountDetailRecordFile, fmt.Sprintf("UsedTime:%d, Resource: %s, UserName: %v Count: %d",
						int(usedTime), apiCountName, userApiCount.UserName, userApiCount.RequestCount)); err != nil {
						gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					}
				}
			}
		}
		// count set to 0 every hour, so need to get addcount here
		if _, ok := initCount[apiCountName]; !ok {
			initCount[apiCountName] = curCount
		}

		if curCount < initCount[apiCountName] {
			refreshed = true
		}

		if refreshed {
			addCount = curCount + addCount
		} else {
			addCount = curCount - initCount[apiCountName]
		}

		if _, ok := p.rep.TotalApiRequestCount[apiCountName]; !ok {
			p.rep.TotalApiRequestCount[apiCountName] = addCount
		} else {
			if p.rep.TotalApiRequestCount[apiCountName] < addCount {
				p.rep.TotalApiRequestCount[apiCountName] = addCount
			}
		}
		if err := utils.AppendRecordToFile(p.apiCountRecordFile, fmt.Sprintf("UsedTime:%d, Resource: %s, All Count: %d, AddCount: %d",
			int(usedTime), apiCountName, curCount, addCount)); err != nil {
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		}
	}
	return nil
}

// CheckIncreasedApiCount get totalTime api request count
func (p *ApiCountOps) CheckIncreasedApiCount(ctx context.Context) {
	ginkgo.By(fmt.Sprintf("Check api request count"))
	startTime := time.Now()
	initCount := make(map[string]int)
	for {
		usedTime := time.Since(startTime).Seconds()
		if usedTime >= totalTime {
			break
		}
		err := p.CheckApiCount(ctx, usedTime, initCount)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		time.Sleep(interval * time.Second)
	}
	ginkgo.By(fmt.Sprintf("Finish api request count"))
}
