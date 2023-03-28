package envtest

import (
	"encoding/json"
	"path/filepath"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/afero"
	versions "sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
)

//	{
//	  "info": {
//	    "major": "1",
//	    "minor": "25",
//	    "gitVersion": "v1.25.5",
//	    "gitCommit": "804d6167111f6858541cef440ccc53887fbbc96a",
//	    "gitTreeState": "clean",
//	    "buildDate": "2023-02-15T11:49:50Z",
//	    "goVersion": "go1.19.4",
//	    "compiler": "gc",
//	    "platform": "linux/amd64"
//	  },
//	  "string": "v1.25.5"
//	}
type clusterInfo struct {
	Info struct {
		Major      string `json:"major"`
		Minor      string `json:"minor"`
		GitVersion string `json:"gitVersion"`
	} `json:"info"`
	VersionString string `json:"string"`
}

func selectorFromSemver(sv *semver.Version) versions.Selector {
	// return versions.Concrete{
	// 	Major: int(sv.Major()),
	// 	Minor: int(sv.Minor()),
	// 	Patch: int(sv.Patch()),
	// }
	// default storage bucket does not contain all versions
	//
	return versions.PatchSelector{
		Major: int(sv.Major()),
		Minor: int(sv.Minor()),
		Patch: versions.AnyPoint,
	}
}

// DetectK8sVersion attempts to load k8s server version from which was bundle
// collected.
func DetectK8sVersion(b bundle.Bundle) (versions.Selector, error) {
	data, err := afero.ReadFile(b, filepath.Join(b.Layout().ClusterInfo(), "cluster_version.json"))
	if err != nil {
		return nil, err
	}

	i := &clusterInfo{}
	if err := json.Unmarshal(data, &i); err != nil {
		return nil, err
	}

	if sv, err := semver.NewVersion(i.VersionString); err == nil {
		return selectorFromSemver(sv), nil
	}

	if sv, err := semver.NewVersion(i.Info.GitVersion); err == nil {
		return selectorFromSemver(sv), nil
	}

	major, _ := strconv.Atoi(i.Info.Major)
	minor, _ := strconv.Atoi(i.Info.Minor)
	return versions.PatchSelector{
		Major: major,
		Minor: minor,
		Patch: versions.AnyPoint,
	}, nil
}
