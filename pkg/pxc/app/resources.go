package app

import (
	"fmt"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func CreateResources(r *api.PodResources) (rr corev1.ResourceRequirements, err error) {
	if r == nil {
		return rr, nil
	}

	if r.Requests != nil {
		rlist, err := createResourceList(r.Requests)
		if err != nil {
			return rr, err
		}

		rr.Requests = rlist
	}

	if r.Limits != nil {
		rlist, err := createResourceList(r.Limits)
		if err != nil {
			return rr, err
		}

		rr.Limits = rlist
	}

	return rr, nil
}

func createResourceList(l *api.ResourcesList) (rlist corev1.ResourceList, err error) {
	rlist = make(corev1.ResourceList)

	if len(l.CPU) > 0 {
		rlist[corev1.ResourceCPU], err = resource.ParseQuantity(l.CPU)
		if err != nil {
			return nil, fmt.Errorf("malformed CPU resources: %v", err)
		}
	}
	if len(l.Memory) > 0 {
		rlist[corev1.ResourceMemory], err = resource.ParseQuantity(l.Memory)
		if err != nil {
			return nil, fmt.Errorf("malformed memory resources: %v", err)
		}
	}
	if len(l.EphemeralStorage) > 0 {
		rlist[corev1.ResourceEphemeralStorage], err = resource.ParseQuantity(l.EphemeralStorage)
		if err != nil {
			return nil, fmt.Errorf("malformed ephemeral-storage resources: %v", err)
		}
	}

	return rlist, nil
}
