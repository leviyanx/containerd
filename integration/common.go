/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package integration

import (
	"fmt"
	"os"
	"testing"

	cri "github.com/containerd/containerd/integration/cri-api/pkg/apis"
	"github.com/pelletier/go-toml"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ImageList holds public image references
type ImageList struct {
	Alpine           string
	BusyBox          string
	Pause            string
	ResourceConsumer string
	VolumeCopyUp     string
	VolumeOwnership  string
}

var (
	imageService cri.ImageManagerService
	imageMap     map[int]string
	imageList    ImageList
	pauseImage   string // This is the same with default sandbox image
)

func initImages(imageListFile string) {
	imageList = ImageList{
		Alpine:           "ghcr.io/containerd/alpine:3.14.0",
		BusyBox:          "ghcr.io/containerd/busybox:1.28",
		Pause:            "registry.k8s.io/pause:3.6",
		ResourceConsumer: "registry.k8s.io/e2e-test-images/resource-consumer:1.10",
		VolumeCopyUp:     "ghcr.io/containerd/volume-copy-up:2.1",
		VolumeOwnership:  "ghcr.io/containerd/volume-ownership:2.1",
	}

	if imageListFile != "" {
		fileContent, err := os.ReadFile(imageListFile)
		if err != nil {
			panic(fmt.Errorf("error reading '%v' file contents: %v", imageList, err))
		}

		err = toml.Unmarshal(fileContent, &imageList)
		if err != nil {
			panic(fmt.Errorf("error unmarshalling '%v' TOML file: %v", imageList, err))
		}
	}

	logrus.Infof("Using the following image list: %+v", imageList)

	imageMap = initImageMap(imageList)
	pauseImage = GetImage(Pause)
}

const (
	// None is to be used for unset/default images
	None = iota
	// Alpine image
	Alpine
	// BusyBox image
	BusyBox
	// Pause image
	Pause
	// ResourceConsumer image
	ResourceConsumer
	// VolumeCopyUp image
	VolumeCopyUp
	// VolumeOwnership image
	VolumeOwnership
)

func initImageMap(imageList ImageList) map[int]string {
	images := map[int]string{}
	images[Alpine] = imageList.Alpine
	images[BusyBox] = imageList.BusyBox
	images[Pause] = imageList.Pause
	images[ResourceConsumer] = imageList.ResourceConsumer
	images[VolumeCopyUp] = imageList.VolumeCopyUp
	images[VolumeOwnership] = imageList.VolumeOwnership
	return images
}

// GetImage returns the fully qualified URI to an image (including version)
func GetImage(image int) string {
	return imageMap[image]
}

// EnsureImageExists pulls the given image, ensures that no error was encountered
// while pulling it.
func EnsureImageExists(t *testing.T, imageName string) string {
	img, err := imageService.ImageStatus(&runtime.ImageSpec{Image: imageName})
	require.NoError(t, err)
	if img != nil {
		t.Logf("Image %q already exists, not pulling.", imageName)
		return img.Id
	}

	t.Logf("Pull test image %q", imageName)
	imgID, err := imageService.PullImage(&runtime.ImageSpec{Image: imageName}, nil, nil)
	require.NoError(t, err)

	return imgID
}

// EnsureWasmModuleExists pulls the given wasm module, ensures that no error was encountered
// while pulling it.
func EnsureWasmModuleExists(t *testing.T, image runtime.ImageSpec) string {
	img, err := imageService.ImageStatus(&image)
	require.NoError(t, err)
	if img != nil {
		t.Logf("Wasm module %q already exists, not pulling.", image.GetImage())
		return img.Id
	}

	t.Logf("Pull test wasm module %q", image.GetImage())
	imgID, err := imageService.PullImage(&image, nil, nil)
	require.NoError(t, err)

	return imgID
}
