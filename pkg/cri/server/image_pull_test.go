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

package server

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"

	criconfig "github.com/containerd/containerd/pkg/cri/config"
)

func TestParseAuth(t *testing.T) {
	testUser := "username"
	testPasswd := "password"
	testAuthLen := base64.StdEncoding.EncodedLen(len(testUser + ":" + testPasswd))
	testAuth := make([]byte, testAuthLen)
	base64.StdEncoding.Encode(testAuth, []byte(testUser+":"+testPasswd))
	invalidAuth := make([]byte, testAuthLen)
	base64.StdEncoding.Encode(invalidAuth, []byte(testUser+"@"+testPasswd))
	for desc, test := range map[string]struct {
		auth           *runtime.AuthConfig
		host           string
		expectedUser   string
		expectedSecret string
		expectErr      bool
	}{
		"should not return error if auth config is nil": {},
		"should not return error if empty auth is provided for access to anonymous registry": {
			auth:      &runtime.AuthConfig{},
			expectErr: false,
		},
		"should support identity token": {
			auth:           &runtime.AuthConfig{IdentityToken: "abcd"},
			expectedSecret: "abcd",
		},
		"should support username and password": {
			auth: &runtime.AuthConfig{
				Username: testUser,
				Password: testPasswd,
			},
			expectedUser:   testUser,
			expectedSecret: testPasswd,
		},
		"should support auth": {
			auth:           &runtime.AuthConfig{Auth: string(testAuth)},
			expectedUser:   testUser,
			expectedSecret: testPasswd,
		},
		"should return error for invalid auth": {
			auth:      &runtime.AuthConfig{Auth: string(invalidAuth)},
			expectErr: true,
		},
		"should return empty auth if server address doesn't match": {
			auth: &runtime.AuthConfig{
				Username:      testUser,
				Password:      testPasswd,
				ServerAddress: "https://registry-1.io",
			},
			host:           "registry-2.io",
			expectedUser:   "",
			expectedSecret: "",
		},
		"should return auth if server address matches": {
			auth: &runtime.AuthConfig{
				Username:      testUser,
				Password:      testPasswd,
				ServerAddress: "https://registry-1.io",
			},
			host:           "registry-1.io",
			expectedUser:   testUser,
			expectedSecret: testPasswd,
		},
		"should return auth if server address is not specified": {
			auth: &runtime.AuthConfig{
				Username: testUser,
				Password: testPasswd,
			},
			host:           "registry-1.io",
			expectedUser:   testUser,
			expectedSecret: testPasswd,
		},
	} {
		t.Logf("TestCase %q", desc)
		u, s, err := ParseAuth(test.auth, test.host)
		assert.Equal(t, test.expectErr, err != nil)
		assert.Equal(t, test.expectedUser, u)
		assert.Equal(t, test.expectedSecret, s)
	}
}

func TestRegistryEndpoints(t *testing.T) {
	for desc, test := range map[string]struct {
		mirrors  map[string]criconfig.Mirror
		host     string
		expected []string
	}{
		"no mirror configured": {
			mirrors: map[string]criconfig.Mirror{
				"registry-1.io": {
					Endpoints: []string{
						"https://registry-1.io",
						"https://registry-2.io",
					},
				},
			},
			host: "registry-3.io",
			expected: []string{
				"https://registry-3.io",
			},
		},
		"mirror configured": {
			mirrors: map[string]criconfig.Mirror{
				"registry-3.io": {
					Endpoints: []string{
						"https://registry-1.io",
						"https://registry-2.io",
					},
				},
			},
			host: "registry-3.io",
			expected: []string{
				"https://registry-1.io",
				"https://registry-2.io",
				"https://registry-3.io",
			},
		},
		"wildcard mirror configured": {
			mirrors: map[string]criconfig.Mirror{
				"*": {
					Endpoints: []string{
						"https://registry-1.io",
						"https://registry-2.io",
					},
				},
			},
			host: "registry-3.io",
			expected: []string{
				"https://registry-1.io",
				"https://registry-2.io",
				"https://registry-3.io",
			},
		},
		"host should take precedence if both host and wildcard mirrors are configured": {
			mirrors: map[string]criconfig.Mirror{
				"*": {
					Endpoints: []string{
						"https://registry-1.io",
					},
				},
				"registry-3.io": {
					Endpoints: []string{
						"https://registry-2.io",
					},
				},
			},
			host: "registry-3.io",
			expected: []string{
				"https://registry-2.io",
				"https://registry-3.io",
			},
		},
		"default endpoint in list with http": {
			mirrors: map[string]criconfig.Mirror{
				"registry-3.io": {
					Endpoints: []string{
						"https://registry-1.io",
						"https://registry-2.io",
						"http://registry-3.io",
					},
				},
			},
			host: "registry-3.io",
			expected: []string{
				"https://registry-1.io",
				"https://registry-2.io",
				"http://registry-3.io",
			},
		},
		"default endpoint in list with https": {
			mirrors: map[string]criconfig.Mirror{
				"registry-3.io": {
					Endpoints: []string{
						"https://registry-1.io",
						"https://registry-2.io",
						"https://registry-3.io",
					},
				},
			},
			host: "registry-3.io",
			expected: []string{
				"https://registry-1.io",
				"https://registry-2.io",
				"https://registry-3.io",
			},
		},
		"default endpoint in list with path": {
			mirrors: map[string]criconfig.Mirror{
				"registry-3.io": {
					Endpoints: []string{
						"https://registry-1.io",
						"https://registry-2.io",
						"https://registry-3.io/path",
					},
				},
			},
			host: "registry-3.io",
			expected: []string{
				"https://registry-1.io",
				"https://registry-2.io",
				"https://registry-3.io/path",
			},
		},
		"miss scheme endpoint in list with path": {
			mirrors: map[string]criconfig.Mirror{
				"registry-3.io": {
					Endpoints: []string{
						"https://registry-3.io",
						"registry-1.io",
						"127.0.0.1:1234",
					},
				},
			},
			host: "registry-3.io",
			expected: []string{
				"https://registry-3.io",
				"https://registry-1.io",
				"http://127.0.0.1:1234",
			},
		},
	} {
		t.Logf("TestCase %q", desc)
		c := newTestCRIService()
		c.config.Registry.Mirrors = test.mirrors
		got, err := c.registryEndpoints(test.host)
		assert.NoError(t, err)
		assert.Equal(t, test.expected, got)
	}
}

func TestDefaultScheme(t *testing.T) {
	for desc, test := range map[string]struct {
		host     string
		expected string
	}{
		"should use http by default for localhost": {
			host:     "localhost",
			expected: "http",
		},
		"should use http by default for localhost with port": {
			host:     "localhost:8080",
			expected: "http",
		},
		"should use http by default for 127.0.0.1": {
			host:     "127.0.0.1",
			expected: "http",
		},
		"should use http by default for 127.0.0.1 with port": {
			host:     "127.0.0.1:8080",
			expected: "http",
		},
		"should use http by default for ::1": {
			host:     "::1",
			expected: "http",
		},
		"should use http by default for ::1 with port": {
			host:     "[::1]:8080",
			expected: "http",
		},
		"should use https by default for remote host": {
			host:     "remote",
			expected: "https",
		},
		"should use https by default for remote host with port": {
			host:     "remote:8080",
			expected: "https",
		},
		"should use https by default for remote ip": {
			host:     "8.8.8.8",
			expected: "https",
		},
		"should use https by default for remote ip with port": {
			host:     "8.8.8.8:8080",
			expected: "https",
		},
	} {
		t.Logf("TestCase %q", desc)
		got := defaultScheme(test.host)
		assert.Equal(t, test.expected, got)
	}
}

func TestEncryptedImagePullOpts(t *testing.T) {
	for desc, test := range map[string]struct {
		keyModel     string
		expectedOpts int
	}{
		"node key model should return one unpack opt": {
			keyModel:     criconfig.KeyModelNode,
			expectedOpts: 1,
		},
		"no key model selected should default to node key model": {
			keyModel:     "",
			expectedOpts: 0,
		},
	} {
		t.Logf("TestCase %q", desc)
		c := newTestCRIService()
		c.config.ImageDecryption.KeyModel = test.keyModel
		got := len(c.encryptedImagesPullOpts())
		assert.Equal(t, test.expectedOpts, got)
	}
}

func TestPullWasmModule(t *testing.T) {
	for desc, test := range map[string]struct {
		r            *runtime.PullImageRequest
		expectedOpts string
	}{
		"should pull wasm module and return image id": {
			r: &runtime.PullImageRequest{
				Image: &runtime.ImageSpec{
					Image: "wasi_example_main",
					Annotations: map[string]string{
						"wasm.module.url": "https://github.com/leviyanx/wasm-program-image/raw/main/wasi/wasi_example_main.wasm",
					},
				},
			},
			expectedOpts: "fp051QR9RWgnkrmRlNAldEU9pWD_RyLdVG0stfDImoQ=",
		},
	} {
		t.Logf("TestCase %q", desc)
		c := newTestCRIService()
		pullImageResponse, _ := c.PullImage(context.Background(), test.r)
		got := pullImageResponse.ImageRef
		assert.Equal(t, test.expectedOpts, got)
	}

}
