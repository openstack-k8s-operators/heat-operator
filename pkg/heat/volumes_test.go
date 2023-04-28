package heat

import (
	"testing"
)

func TestGetVolumes(t *testing.T) {
	name := "test-name"

	volumes := GetVolumes(name)

	if len(volumes) != 3 {
		t.Errorf("expected 3 volumes, but got %d", len(volumes))
	}

	expectedScriptsVolumeName := name + "-scripts"
	if volumes[0].Name != "scripts" || volumes[0].VolumeSource.ConfigMap == nil || volumes[0].VolumeSource.ConfigMap.LocalObjectReference.Name != expectedScriptsVolumeName {
		t.Errorf("expected scripts volume with name '%s', but got %+v", expectedScriptsVolumeName, volumes[0])
	}

	expectedConfigDataVolumeName := name + "-config-data"
	if volumes[1].Name != "config-data" || volumes[1].VolumeSource.ConfigMap == nil || volumes[1].VolumeSource.ConfigMap.LocalObjectReference.Name != expectedConfigDataVolumeName {
		t.Errorf("expected config-data volume with name '%s', but got %+v", expectedConfigDataVolumeName, volumes[1])
	}

	if volumes[2].Name != "config-data-merged" || volumes[2].VolumeSource.EmptyDir == nil || volumes[2].VolumeSource.EmptyDir.Medium != "" {
		t.Errorf("expected config-data-merged volume with empty directory, but got %+v", volumes[2])
	}
}
