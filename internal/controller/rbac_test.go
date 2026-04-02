// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// rbacRule represents a parsed kubebuilder:rbac marker.
type rbacRule struct {
	Groups    []string
	Resources []string
	Verbs     []string
}

// parseRBACMarkers parses all +kubebuilder:rbac markers from a Go source file.
func parseRBACMarkers(filename string) ([]rbacRule, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var rules []rbacRule
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "// +kubebuilder:rbac:") {
			continue
		}
		marker := strings.TrimPrefix(line, "// +kubebuilder:rbac:")
		rule := rbacRule{}
		for _, part := range strings.Split(marker, ",") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key := strings.TrimSpace(kv[0])
			value := strings.Trim(strings.TrimSpace(kv[1]), "\"")
			switch key {
			case "groups":
				rule.Groups = strings.Split(value, ";")
			case "resources":
				rule.Resources = strings.Split(value, ";")
			case "verbs":
				rule.Verbs = strings.Split(value, ";")
			}
		}
		rules = append(rules, rule)
	}
	return rules, scanner.Err()
}

// hasRule checks if the list of rules contains a rule matching the given group, resource, and all specified verbs.
func hasRule(rules []rbacRule, group, resource string, verbs ...string) bool {
	for _, rule := range rules {
		groupMatch := false
		for _, g := range rule.Groups {
			if g == group {
				groupMatch = true
				break
			}
		}
		if !groupMatch {
			continue
		}
		resourceMatch := false
		for _, r := range rule.Resources {
			if r == resource {
				resourceMatch = true
				break
			}
		}
		if !resourceMatch {
			continue
		}
		allVerbsPresent := true
		for _, v := range verbs {
			found := false
			for _, rv := range rule.Verbs {
				if rv == v {
					found = true
					break
				}
			}
			if !found {
				allVerbsPresent = false
				break
			}
		}
		if allVerbsPresent {
			return true
		}
	}
	return false
}

// hasNoWildcardGroups verifies no rule uses groups=* wildcard.
func hasNoWildcardGroups(rules []rbacRule) []string {
	var violations []string
	for _, rule := range rules {
		for _, g := range rule.Groups {
			if g == "*" {
				violations = append(violations, fmt.Sprintf("wildcard group '*' used for resources %v", rule.Resources))
			}
		}
	}
	return violations
}

// validResourceNames is the complete set of resource names (including sub-resources)
// that ETOS controllers are expected to reference. Any resource name not in this set
// is likely a typo. This catches all misspellings, not just previously known ones.
var validResourceNames = map[string]bool{
	// ETOS CRD resources
	"clusters": true, "clusters/status": true, "clusters/finalizers": true,
	"environmentrequests": true, "environmentrequests/status": true, "environmentrequests/finalizers": true,
	"environments": true, "environments/status": true, "environments/finalizers": true,
	"executionspaces": true, "executionspaces/status": true, "executionspaces/finalizers": true,
	"iuts": true, "iuts/status": true, "iuts/finalizers": true,
	"logarea": true, "logarea/status": true, "logarea/finalizers": true,
	"providers": true, "providers/status": true, "providers/finalizers": true,
	"testruns": true, "testruns/status": true, "testruns/finalizers": true,
	// Standard Kubernetes resources
	"deployments": true, "statefulsets": true,
	"secrets": true, "services": true, "serviceaccounts": true, "configmaps": true,
	"pods": true, "pods/log": true,
	"roles": true, "rolebindings": true,
	"ingresses": true,
	"jobs": true, "jobs/status": true,
}

// hasOnlyValidResourceNames checks that every resource name in the rules is in the whitelist.
func hasOnlyValidResourceNames(rules []rbacRule) []string {
	var violations []string
	for _, rule := range rules {
		for _, r := range rule.Resources {
			if !validResourceNames[r] {
				violations = append(violations, fmt.Sprintf("unknown resource name '%s' (not in valid resource whitelist)", r))
			}
		}
	}
	return violations
}

// formatRules formats rules for readable error messages.
func formatRules(rules []rbacRule) string {
	var lines []string
	for _, rule := range rules {
		lines = append(lines, fmt.Sprintf("  groups=%s resources=%s verbs=%s",
			strings.Join(rule.Groups, ";"),
			strings.Join(rule.Resources, ";"),
			strings.Join(rule.Verbs, ";")))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

var _ = Describe("RBAC Rules", func() {
	controllerDir := "."

	// Map of controller files to their expected RBAC requirements.
	// Each entry lists the minimum required (group, resource, verbs) tuples.
	type expectedRule struct {
		group    string
		resource string
		verbs    []string
	}

	controllerExpectations := map[string][]expectedRule{
		"cluster_controller.go": {
			{"etos.eiffel-community.github.io", "clusters", []string{"get", "list", "watch"}},
			{"etos.eiffel-community.github.io", "clusters/status", []string{"get", "update", "patch"}},
			{"apps", "deployments", []string{"get", "create", "update", "delete"}},
			{"apps", "statefulsets", []string{"get", "create", "update", "delete"}},
			{"", "secrets", []string{"get", "create", "update", "delete"}},
			{"", "services", []string{"get", "create", "update", "delete"}},
			{"", "serviceaccounts", []string{"get", "create", "update", "delete"}},
			{"rbac.authorization.k8s.io", "roles", []string{"get", "create", "update", "delete"}},
			{"rbac.authorization.k8s.io", "rolebindings", []string{"get", "create", "update", "delete"}},
			{"networking.k8s.io", "ingresses", []string{"get", "create", "update", "delete"}},
		},
		"testrun_controller.go": {
			{"etos.eiffel-community.github.io", "testruns", []string{"get", "list", "watch", "create", "update", "delete"}},
			{"etos.eiffel-community.github.io", "testruns/status", []string{"get", "update", "patch"}},
			{"etos.eiffel-community.github.io", "testruns/finalizers", []string{"update"}},
			{"etos.eiffel-community.github.io", "clusters", []string{"get"}},
			{"etos.eiffel-community.github.io", "environments", []string{"get", "list", "watch"}},
			{"etos.eiffel-community.github.io", "environmentrequests", []string{"get", "list", "watch", "create", "delete"}},
			{"etos.eiffel-community.github.io", "environmentrequests/status", []string{"get"}},
			{"etos.eiffel-community.github.io", "providers", []string{"get"}},
			{"batch", "jobs", []string{"get", "list", "watch", "create", "delete"}},
			{"batch", "jobs/status", []string{"get"}},
		},
		"environmentrequest_controller.go": {
			{"etos.eiffel-community.github.io", "environmentrequests", []string{"get", "list", "watch", "create", "update", "delete"}},
			{"etos.eiffel-community.github.io", "environmentrequests/status", []string{"get", "update", "patch"}},
			{"etos.eiffel-community.github.io", "environmentrequests/finalizers", []string{"update"}},
			{"etos.eiffel-community.github.io", "clusters", []string{"get"}},
			{"etos.eiffel-community.github.io", "environments", []string{"get", "list", "watch", "delete"}},
			{"etos.eiffel-community.github.io", "providers", []string{"get"}},
			{"etos.eiffel-community.github.io", "providers/status", []string{"get"}},
			{"", "secrets", []string{"get"}},
			{"", "pods", []string{"get", "list"}},
			{"batch", "jobs", []string{"get", "list", "watch", "create", "delete"}},
			{"batch", "jobs/status", []string{"get"}},
		},
		"environment_controller.go": {
			{"etos.eiffel-community.github.io", "environments", []string{"get", "list", "watch", "create", "update", "delete"}},
			{"etos.eiffel-community.github.io", "environments/status", []string{"get", "update", "patch"}},
			{"etos.eiffel-community.github.io", "environments/finalizers", []string{"update"}},
			{"etos.eiffel-community.github.io", "clusters", []string{"get"}},
			{"etos.eiffel-community.github.io", "environmentrequests", []string{"get"}},
			{"etos.eiffel-community.github.io", "providers", []string{"get"}},
			{"batch", "jobs", []string{"get", "list", "watch", "create", "delete"}},
			{"batch", "jobs/status", []string{"get"}},
		},
		"provider_controller.go": {
			{"etos.eiffel-community.github.io", "providers", []string{"get", "list", "watch"}},
			{"etos.eiffel-community.github.io", "providers/status", []string{"get", "update", "patch"}},
			{"etos.eiffel-community.github.io", "providers/finalizers", []string{"update"}},
		},
		"executionspace_controller.go": {
			{"etos.eiffel-community.github.io", "executionspaces", []string{"get", "list", "watch", "create", "update", "delete"}},
			{"etos.eiffel-community.github.io", "executionspaces/status", []string{"get", "update", "patch"}},
			{"etos.eiffel-community.github.io", "executionspaces/finalizers", []string{"update"}},
			{"etos.eiffel-community.github.io", "providers", []string{"get"}},
			{"etos.eiffel-community.github.io", "environmentrequests", []string{"get"}},
			{"batch", "jobs", []string{"get", "list", "watch", "create", "delete"}},
			{"batch", "jobs/status", []string{"get"}},
		},
		"iut_controller.go": {
			{"etos.eiffel-community.github.io", "iuts", []string{"get", "list", "watch", "create", "update", "delete"}},
			{"etos.eiffel-community.github.io", "iuts/status", []string{"get", "update", "patch"}},
			{"etos.eiffel-community.github.io", "iuts/finalizers", []string{"update"}},
			{"etos.eiffel-community.github.io", "providers", []string{"get"}},
			{"etos.eiffel-community.github.io", "environmentrequests", []string{"get"}},
			{"batch", "jobs", []string{"get", "list", "watch", "create", "delete"}},
			{"batch", "jobs/status", []string{"get"}},
		},
		"logarea_controller.go": {
			{"etos.eiffel-community.github.io", "logarea", []string{"get", "list", "watch", "create", "update", "delete"}},
			{"etos.eiffel-community.github.io", "logarea/status", []string{"get", "update", "patch"}},
			{"etos.eiffel-community.github.io", "logarea/finalizers", []string{"update"}},
			{"etos.eiffel-community.github.io", "providers", []string{"get"}},
			{"etos.eiffel-community.github.io", "environmentrequests", []string{"get"}},
			{"batch", "jobs", []string{"get", "list", "watch", "create", "delete"}},
			{"batch", "jobs/status", []string{"get"}},
		},
	}

	for file, expectations := range controllerExpectations {
		file := file
		expectations := expectations

		Context(fmt.Sprintf("in %s", file), func() {
			var rules []rbacRule

			BeforeEach(func() {
				var err error
				rules, err = parseRBACMarkers(filepath.Join(controllerDir, file))
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not use wildcard (*) API groups", func() {
				violations := hasNoWildcardGroups(rules)
				Expect(violations).To(BeEmpty(),
					"wildcard API groups found in %s:\n%s\nAll rules:\n%s",
					file, strings.Join(violations, "\n"), formatRules(rules))
			})

			It("should only use valid resource names", func() {
				violations := hasOnlyValidResourceNames(rules)
				Expect(violations).To(BeEmpty(),
					"unknown resource names in %s:\n%s", file, strings.Join(violations, "\n"))
			})

			It("should have all required RBAC rules", func() {
				for _, exp := range expectations {
					Expect(hasRule(rules, exp.group, exp.resource, exp.verbs...)).To(BeTrue(),
						"missing RBAC rule in %s: group=%q resource=%q verbs=%v\nActual rules:\n%s",
						file, exp.group, exp.resource, exp.verbs, formatRules(rules))
				}
			})

			It("should use specific API groups for standard Kubernetes resources", func() {
				// Standard Kubernetes resources that must use specific API groups.
				specificGroupResources := map[string]string{
					"deployments":     "apps",
					"statefulsets":    "apps",
					"secrets":         "",
					"services":        "",
					"serviceaccounts": "",
					"configmaps":      "",
					"pods":            "",
					"roles":           "rbac.authorization.k8s.io",
					"rolebindings":    "rbac.authorization.k8s.io",
					"ingresses":       "networking.k8s.io",
					"jobs":            "batch",
					"jobs/status":     "batch",
				}
				for _, rule := range rules {
					for _, resource := range rule.Resources {
						expectedGroup, isStandard := specificGroupResources[resource]
						if !isStandard {
							continue
						}
						for _, group := range rule.Groups {
							Expect(group).NotTo(Equal("*"),
								"resource '%s' in %s should use group '%s', not '*'",
								resource, file, expectedGroup)
						}
					}
				}
			})
		})
	}
})
