#!/bin/bash

#########################################
#   POPULATE THESE WITH ENV VARS        #
#########################################
# HUB_KUBECONFIG is the kubeconfig file path of the hub cluster
#export HUB_KUBECONFIG=/path/to/file
# RESULTS_DIR is the output directory of the test reports
#RESULTS_DIR=/path/to/directory


echo "Initiating foundation tests..."
echo "Tests <$TEST_GROUP> start at "$(date)

if [[ $TEST_GROUP == "placement" ]]; then
    /placement-e2e --ginkgo.v --ginkgo.label-filter=sanity-check \
     --ginkgo.junit-report="${RESULTS_DIR}/foundation-placement-e2e.xml" \
     -hub-kubeconfig=${HUB_KUBECONFIG} -create-global-clusterset=false -tolerate-unreachable-taint
elif [[ $TEST_GROUP == "work" ]]; then
    echo "Not implemented yet for $TEST_GROUP"
fi

echo "Tests <$TEST_GROUP> end at "$(date)