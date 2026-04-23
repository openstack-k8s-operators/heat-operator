/*
Copyright 2026.

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

// Package functional_test contains functional tests for the heat operator.
package functional_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/roles"
	keystone_helpers "github.com/openstack-k8s-operators/keystone-operator/api/test/helpers"
	api "github.com/openstack-k8s-operators/lib-common/modules/test/apis"
)

func heatKeystoneTokenResponse(keystoneURL string) string {
	return fmt.Sprintf(
		`{"token":{"catalog":[{"endpoints":[{"interface":"internal","region_id":"RegionOne","url":"%s","region":"RegionOne"},{"interface":"public","region_id":"RegionOne","url":"%s","region":"RegionOne"}],"type":"identity","name":"keystone"}]}}`,
		keystoneURL, keystoneURL)
}

func setupHeatKeystoneFixture(logger logr.Logger) *keystone_helpers.KeystoneAPIFixture {
	fixture := keystone_helpers.NewKeystoneAPIFixtureWithServer(logger)
	roleStore := map[string]roles.Role{
		"admin": {
			Name: "admin",
			ID:   uuid.NewString(),
		},
	}

	fixture.Setup(
		api.Handler{Pattern: "/", Func: fixture.HandleVersion},
		api.Handler{Pattern: "/v3/users", Func: fixture.HandleUsers},
		api.Handler{Pattern: "/v3/domains", Func: fixture.HandleDomains},
		api.Handler{Pattern: "/v3/roles", Func: func(w http.ResponseWriter, r *http.Request) {
			fixture.LogRequest(r)
			switch r.Method {
			case "GET":
				nameFilter := r.URL.Query().Get("name")
				var rs []roles.Role
				for name, role := range roleStore {
					if nameFilter == "" || name == nameFilter {
						rs = append(rs, role)
					}
				}
				resp := struct {
					Roles []roles.Role `json:"roles"`
				}{Roles: rs}
				bytes, err := json.Marshal(&resp)
				if err != nil {
					fixture.InternalError(err, "Error during marshalling response", w, r)
					return
				}
				w.Header().Add("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, string(bytes))
			case "POST":
				bytes, err := io.ReadAll(r.Body)
				if err != nil {
					fixture.InternalError(err, "Error reading request body", w, r)
					return
				}
				var s struct {
					Role roles.Role `json:"role"`
				}
				if err := json.Unmarshal(bytes, &s); err != nil {
					fixture.InternalError(err, "Error during unmarshalling request", w, r)
					return
				}
				if s.Role.ID == "" {
					s.Role.ID = uuid.NewString()
				}
				roleStore[s.Role.Name] = s.Role
				respBytes, err := json.Marshal(&s)
				if err != nil {
					fixture.InternalError(err, "Error during marshalling response", w, r)
					return
				}
				w.Header().Add("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprint(w, string(respBytes))
			default:
				fixture.UnexpectedRequest(w, r)
			}
		}},
		api.Handler{Pattern: "/v3/role_assignments", Func: func(w http.ResponseWriter, r *http.Request) {
			fixture.LogRequest(r)
			if r.Method != "GET" {
				fixture.UnexpectedRequest(w, r)
				return
			}
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"role_assignments":[]}`)
		}},
		api.Handler{Pattern: "/v3/domains/", Func: func(w http.ResponseWriter, r *http.Request) {
			fixture.LogRequest(r)
			if r.Method == "PUT" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			fixture.UnexpectedRequest(w, r)
		}},
		api.Handler{Pattern: "/v3/auth/tokens", Func: func(w http.ResponseWriter, r *http.Request) {
			fixture.LogRequest(r)
			if r.Method != "POST" {
				fixture.UnexpectedRequest(w, r)
				return
			}
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprint(w, heatKeystoneTokenResponse(fixture.Endpoint()))
		}},
	)
	return fixture
}
