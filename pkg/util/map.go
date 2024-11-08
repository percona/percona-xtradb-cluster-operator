package util

import (
	appsv1 "k8s.io/api/apps/v1"
)

func MergeTemplateAnnotations(sfs *appsv1.StatefulSet, annotations map[string]string) {
	if len(annotations) == 0 {
		return
	}
	MergeMaps(sfs.Spec.Template.Annotations, annotations)
}

func MergeMaps(dest map[string]string, mapList ...map[string]string) {
	if dest == nil {
		dest = make(map[string]string)
	}
	for _, m := range mapList {
		for k, v := range m {
			dest[k] = v
		}
	}
}
